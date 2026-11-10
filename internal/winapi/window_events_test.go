package winapi

import (
	"context"
	"testing"
	"time"
)

func TestTranslateWinEventFiltersObjects(t *testing.T) {
	cases := []struct {
		event             uint32
		hwnd              uintptr
		idObject, idChild int32
		want              WindowMoveEvent
		ok                bool
	}{
		{eventSystemMoveSizeStart, 42, 0, 0, WindowMoveEvent{WindowID: "42", Start: true}, true},
		{eventSystemMoveSizeEnd, 42, 0, 0, WindowMoveEvent{WindowID: "42"}, true},
		{eventSystemMoveSizeEnd, 0, 0, 0, WindowMoveEvent{}, false},
		{eventSystemMoveSizeEnd, 42, -4, 0, WindowMoveEvent{}, false},
		{eventSystemMoveSizeEnd, 42, 0, 3, WindowMoveEvent{}, false},
		{0x8000, 42, 0, 0, WindowMoveEvent{}, false},
	}
	for _, test := range cases {
		got, ok := translateWinEvent(test.event, test.hwnd, test.idObject, test.idChild)
		if ok != test.ok || got != test.want {
			t.Errorf("translateWinEvent(%#x,%d,%d,%d) = %+v,%v", test.event, test.hwnd, test.idObject, test.idChild, got, ok)
		}
	}
}

func TestWatchWindowMovesStopsOnCancel(t *testing.T) {
	for round := 0; round < 3; round++ {
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() { result <- WatchWindowMoves(ctx, func(WindowMoveEvent) {}) }()
		time.Sleep(50 * time.Millisecond)
		cancel()
		select {
		case err := <-result:
			if err != nil {
				t.Fatalf("round %d: %v", round, err)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("round %d: WatchWindowMoves did not stop after cancel", round)
		}
	}
}

func TestWatchWindowMovesImmediateCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- WatchWindowMoves(ctx, func(WindowMoveEvent) {}) }()
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("WatchWindowMoves did not stop")
	}
}

func TestStartWindowMoveWatcherReadyThenDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done, err := StartWindowMoveWatcher(ctx, func(WindowMoveEvent) {})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err, ok := <-done:
		if !ok || err != nil {
			t.Fatalf("done = %v, %v", err, ok)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("watcher did not stop")
	}
	if _, ok := <-done; ok {
		t.Fatal("done must be closed")
	}
	if _, err := StartWindowMoveWatcher(context.Background(), nil); err == nil {
		t.Fatal("nil emit must fail")
	}
}
