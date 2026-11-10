package winapi

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

var (
	modDwmapi                         = syscall.NewLazyDLL("dwmapi.dll")
	modAdvapi32                       = syscall.NewLazyDLL("advapi32.dll")
	procGetWindowRect                 = modUser32.NewProc("GetWindowRect")
	procSetWindowPos                  = modUser32.NewProc("SetWindowPos")
	procShowWindow                    = modUser32.NewProc("ShowWindow")
	procIsIconic                      = modUser32.NewProc("IsIconic")
	procIsZoomed                      = modUser32.NewProc("IsZoomed")
	procMonitorFromWindow             = modUser32.NewProc("MonitorFromWindow")
	procKeybdEvent                    = modUser32.NewProc("keybd_event")
	procSetProcessDpiAwarenessContext = modUser32.NewProc("SetProcessDpiAwarenessContext")
	procDwmGetWindowAttribute         = modDwmapi.NewProc("DwmGetWindowAttribute")
	procOpenProcessToken              = modAdvapi32.NewProc("OpenProcessToken")
	procGetTokenInformation           = modAdvapi32.NewProc("GetTokenInformation")
	procGetCurrentProcess             = modKernel32.NewProc("GetCurrentProcess")
)

const (
	swRestore                = 9
	swpNoZOrder              = 0x0004
	swpNoActivate            = 0x0010
	swpNoOwnerZOrder         = 0x0200
	monitorDefaultToNearest  = 2
	dwmwaExtendedFrameBounds = 9
	tokenQuery               = 0x0008
	tokenElevationClass      = 20
	vkMenu                   = 0x12
	keyEventFKeyUp           = 0x0002
	dpiAwarenessPerMonitorV2 = ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
)

// Rect is a screen rectangle in physical pixels.
type Rect struct {
	X      int32 `json:"x"`
	Y      int32 `json:"y"`
	Width  int32 `json:"width"`
	Height int32 `json:"height"`
}

func rectFrom(r displayRect) Rect {
	return Rect{X: r.Left, Y: r.Top, Width: r.Right - r.Left, Height: r.Bottom - r.Top}
}

// EnablePerMonitorDPI makes coordinates physical pixels on every monitor.
func EnablePerMonitorDPI() {
	if procSetProcessDpiAwarenessContext.Find() == nil {
		procSetProcessDpiAwarenessContext.Call(dpiAwarenessPerMonitorV2)
	}
}

// VisibleWindowRect returns the visible frame (without invisible resize borders).
func VisibleWindowRect(hwnd uintptr) (Rect, error) {
	var frame displayRect
	if procDwmGetWindowAttribute.Find() == nil {
		hr, _, _ := procDwmGetWindowAttribute.Call(hwnd, dwmwaExtendedFrameBounds, uintptr(unsafe.Pointer(&frame)), unsafe.Sizeof(frame))
		if int32(uint32(hr)) >= 0 && frame.Right > frame.Left {
			return rectFrom(frame), nil
		}
	}
	var window displayRect
	if result, _, err := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&window))); result == 0 {
		return Rect{}, fmt.Errorf("GetWindowRect : %w", err)
	}
	return rectFrom(window), nil
}

// SetVisibleWindowRect places the visible frame of a window exactly on target.
func SetVisibleWindowRect(hwnd uintptr, target Rect) error {
	if hwnd == 0 || target.Width <= 0 || target.Height <= 0 {
		return fmt.Errorf("rectangle ou fenetre invalide")
	}
	if alive, _, _ := procIsWindow.Call(hwnd); alive == 0 {
		return fmt.Errorf("fenetre fermee")
	}
	iconic, _, _ := procIsIconic.Call(hwnd)
	zoomed, _, _ := procIsZoomed.Call(hwnd)
	if iconic != 0 || zoomed != 0 {
		procShowWindow.Call(hwnd, swRestore)
	}
	var outer displayRect
	if result, _, err := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&outer))); result == 0 {
		return fmt.Errorf("GetWindowRect : %w", err)
	}
	visible, err := VisibleWindowRect(hwnd)
	if err != nil {
		return err
	}
	left := visible.X - outer.Left
	top := visible.Y - outer.Top
	right := outer.Right - (visible.X + visible.Width)
	bottom := outer.Bottom - (visible.Y + visible.Height)
	result, _, callErr := procSetWindowPos.Call(hwnd, 0,
		uintptr(target.X-left), uintptr(target.Y-top),
		uintptr(target.Width+left+right), uintptr(target.Height+top+bottom),
		swpNoZOrder|swpNoActivate|swpNoOwnerZOrder)
	if result == 0 {
		if errno, ok := callErr.(syscall.Errno); ok && errno == syscall.ERROR_ACCESS_DENIED {
			return fmt.Errorf("Windows refuse de redimensionner une fenetre d'un processus administrateur : lancer deskctl en administrateur")
		}
		return fmt.Errorf("SetWindowPos : %w", callErr)
	}
	return nil
}

// DisplayIDForWindow returns the device name of the monitor that holds most of the window.
func DisplayIDForWindow(hwnd uintptr) (string, error) {
	monitor, _, _ := procMonitorFromWindow.Call(hwnd, monitorDefaultToNearest)
	if monitor == 0 {
		return "", fmt.Errorf("moniteur introuvable")
	}
	info := monitorInfoEx{Size: uint32(unsafe.Sizeof(monitorInfoEx{}))}
	if result, _, err := procGetMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info))); result == 0 {
		return "", fmt.Errorf("lecture du moniteur : %w", err)
	}
	return syscall.UTF16ToString(info.DeviceName[:]), nil
}

func tokenElevated(process uintptr) (bool, error) {
	var token syscall.Handle
	if result, _, err := procOpenProcessToken.Call(process, tokenQuery, uintptr(unsafe.Pointer(&token))); result == 0 {
		return false, err
	}
	defer syscall.CloseHandle(token)
	var elevation, size uint32
	if result, _, err := procGetTokenInformation.Call(uintptr(token), tokenElevationClass,
		uintptr(unsafe.Pointer(&elevation)), unsafe.Sizeof(elevation), uintptr(unsafe.Pointer(&size))); result == 0 {
		return false, err
	}
	return elevation != 0, nil
}

// IsProcessElevatedAboveUs reports whether pid runs elevated while the current process does not.
func IsProcessElevatedAboveUs(pid uint32) bool {
	self, _, _ := procGetCurrentProcess.Call()
	if elevated, err := tokenElevated(self); err != nil || elevated {
		return false
	}
	process, _, _ := procOpenProcess.Call(PROCESS_QUERY_LIMITED_INFORMATION, 0, uintptr(pid))
	if process == 0 {
		return syscall.GetLastError() == syscall.ERROR_ACCESS_DENIED
	}
	defer procCloseHandle.Call(process)
	elevated, err := tokenElevated(process)
	return err == nil && elevated
}

// ForceForeground activates hwnd, using the documented ALT key workaround when
// Windows' foreground lock refuses a plain SetForegroundWindow.
func ForceForeground(hwnd uintptr) error {
	if alive, _, _ := procIsWindow.Call(hwnd); alive == 0 {
		return fmt.Errorf("fenetre fermee")
	}
	if iconic, _, _ := procIsIconic.Call(hwnd); iconic != 0 {
		procShowWindow.Call(hwnd, swRestore)
	}
	for attempt := 0; attempt < 3; attempt++ {
		if current, _, _ := procGetForegroundWindow.Call(); current == hwnd {
			return nil
		}
		if attempt > 0 {
			procKeybdEvent.Call(vkMenu, 0, 0, 0)
			procKeybdEvent.Call(vkMenu, 0, keyEventFKeyUp, 0)
		}
		procSetForegroundWindow.Call(hwnd)
		time.Sleep(60 * time.Millisecond)
	}
	if current, _, _ := procGetForegroundWindow.Call(); current != hwnd {
		return fmt.Errorf("Windows a refuse d'activer la fenetre choisie")
	}
	return nil
}
