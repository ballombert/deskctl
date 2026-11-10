package winapi

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var (
	procSetWinEventHook    = modUser32.NewProc("SetWinEventHook")
	procUnhookWinEvent     = modUser32.NewProc("UnhookWinEvent")
	procGetMessageW        = modUser32.NewProc("GetMessageW")
	procPeekMessageW       = modUser32.NewProc("PeekMessageW")
	procTranslateMessage   = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW   = modUser32.NewProc("DispatchMessageW")
	procPostThreadMessageW = modUser32.NewProc("PostThreadMessageW")
	procPostQuitMessage    = modUser32.NewProc("PostQuitMessage")
	procGetCurrentThreadId = modKernel32.NewProc("GetCurrentThreadId")
)

const (
	eventSystemMoveSizeStart = 0x000A
	eventSystemMoveSizeEnd   = 0x000B
	winEventOutOfContext     = 0x0000
	winEventSkipOwnProcess   = 0x0002
	objIDWindow              = 0
	childIDSelf              = 0
	wmQuit                   = 0x0012
	wmUser                   = 0x0400
	pmNoRemove               = 0x0000
	windowMoveQueueSize      = 256
)

// ErrWindowMoveQueueOverflow is returned when move events arrive faster than emit consumes them.
var ErrWindowMoveQueueOverflow = errors.New("file des evenements de deplacement saturee")

// WindowMoveEvent reports the start or the end of an interactive move/resize of a top-level window.
type WindowMoveEvent struct {
	WindowID string
	Start    bool
}

type winMessage struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	PtX      int32
	PtY      int32
	LPrivate uint32
}

type moveWatcher struct {
	events   chan WindowMoveEvent
	overflow atomic.Bool
}

var (
	moveWatchersMu       sync.Mutex
	moveWatchers         = map[uint32]*moveWatcher{}
	winEventCallback     uintptr
	winEventCallbackOnce sync.Once
)

// translateWinEvent keeps only move/size events of the window object itself.
func translateWinEvent(event uint32, hwnd uintptr, idObject, idChild int32) (WindowMoveEvent, bool) {
	if hwnd == 0 || idObject != objIDWindow || idChild != childIDSelf {
		return WindowMoveEvent{}, false
	}
	switch event {
	case eventSystemMoveSizeStart:
		return WindowMoveEvent{WindowID: strconv.FormatUint(uint64(hwnd), 10), Start: true}, true
	case eventSystemMoveSizeEnd:
		return WindowMoveEvent{WindowID: strconv.FormatUint(uint64(hwnd), 10)}, true
	}
	return WindowMoveEvent{}, false
}

func currentThreadID() uint32 {
	id, _, _ := procGetCurrentThreadId.Call()
	return uint32(id)
}

// winEventProc runs on the pump thread; it never blocks and never places windows.
func winEventProc(hook, event, hwnd, idObject, idChild, thread, eventTime uintptr) uintptr {
	moveEvent, ok := translateWinEvent(uint32(event), hwnd, int32(uint32(idObject)), int32(uint32(idChild)))
	if !ok {
		return 0
	}
	moveWatchersMu.Lock()
	watcher := moveWatchers[currentThreadID()]
	moveWatchersMu.Unlock()
	if watcher == nil || watcher.overflow.Load() {
		return 0
	}
	select {
	case watcher.events <- moveEvent:
	default:
		watcher.overflow.Store(true)
		procPostQuitMessage.Call(0)
	}
	return 0
}

type moveThread struct {
	mu     sync.Mutex
	id     uint32
	exited bool
}

// quit posts WM_QUIT only while the pump thread is guaranteed alive, so a recycled thread ID is never targeted.
func (thread *moveThread) quit() {
	for {
		thread.mu.Lock()
		if thread.exited {
			thread.mu.Unlock()
			return
		}
		result, _, _ := procPostThreadMessageW.Call(uintptr(thread.id), wmQuit, 0, 0)
		thread.mu.Unlock()
		if result != 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// WatchWindowMoves blocks until ctx is cancelled and emits the start and end of user window drags.
// It returns nil after cancellation, or the setup/loop error. See StartWindowMoveWatcher.
func WatchWindowMoves(ctx context.Context, emit func(WindowMoveEvent)) error {
	done, err := StartWindowMoveWatcher(ctx, emit)
	if err != nil {
		return err
	}
	return <-done
}

// StartWindowMoveWatcher returns once the hook is installed (or setup failed). done then receives the
// final error (nil on cancellation) and is closed; the caller stops the watcher by cancelling ctx.
// emit is called sequentially from a watcher goroutine and must return quickly; when the bounded
// queue overflows, the watcher stops with ErrWindowMoveQueueOverflow instead of dropping events silently.
func StartWindowMoveWatcher(ctx context.Context, emit func(WindowMoveEvent)) (<-chan error, error) {
	if emit == nil {
		return nil, fmt.Errorf("callback d'evenement absent")
	}
	result := make(chan error, 1)
	if ctx.Err() != nil {
		result <- nil
		close(result)
		return result, nil
	}
	winEventCallbackOnce.Do(func() { winEventCallback = syscall.NewCallback(winEventProc) })
	watcher := &moveWatcher{events: make(chan WindowMoveEvent, windowMoveQueueSize)}
	thread := &moveThread{}
	ready := make(chan error, 1)
	done := make(chan error, 1)
	go pumpWindowMoves(watcher, thread, ready, done)
	if err := <-ready; err != nil {
		return nil, err
	}
	go func() {
		result <- dispatchWindowMoves(ctx, watcher.events, thread, done, emit)
		close(result)
	}()
	return result, nil
}

func dispatchWindowMoves(ctx context.Context, events <-chan WindowMoveEvent, thread *moveThread, done <-chan error, emit func(WindowMoveEvent)) error {
	for {
		select {
		case <-ctx.Done():
			thread.quit()
			return <-done
		case err := <-done:
			if err != nil {
				return err
			}
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("boucle des evenements de fenetres arretee")
		case event, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			if ctx.Err() == nil {
				emit(event)
			}
		}
	}
}

// pumpWindowMoves owns a dedicated OS thread; it is never unlocked so the thread dies with the goroutine.
func pumpWindowMoves(watcher *moveWatcher, thread *moveThread, ready, done chan<- error) {
	runtime.LockOSThread()
	var message winMessage
	// Forces creation of the thread message queue before PostThreadMessage can target it.
	procPeekMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, wmUser, wmUser, pmNoRemove)
	id := currentThreadID()
	thread.mu.Lock()
	thread.id = id
	thread.mu.Unlock()
	finish := func() {
		thread.mu.Lock()
		thread.exited = true
		thread.mu.Unlock()
		moveWatchersMu.Lock()
		delete(moveWatchers, id)
		moveWatchersMu.Unlock()
		close(watcher.events)
	}
	moveWatchersMu.Lock()
	moveWatchers[id] = watcher
	moveWatchersMu.Unlock()
	hook, _, hookErr := procSetWinEventHook.Call(eventSystemMoveSizeStart, eventSystemMoveSizeEnd, 0,
		winEventCallback, 0, 0, winEventOutOfContext|winEventSkipOwnProcess)
	if hook == 0 {
		finish()
		ready <- fmt.Errorf("SetWinEventHook : %w", hookErr)
		return
	}
	ready <- nil
	var loopErr error
	for {
		result, _, callErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		status := int32(uint32(result))
		if status == -1 {
			loopErr = fmt.Errorf("GetMessageW : %w", callErr)
			break
		}
		if status == 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&message)))
	}
	procUnhookWinEvent.Call(hook)
	if loopErr == nil && watcher.overflow.Load() {
		loopErr = ErrWindowMoveQueueOverflow
	}
	finish()
	done <- loopErr
}
