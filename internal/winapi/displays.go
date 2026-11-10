package winapi

import (
	"fmt"
	"sort"
	"syscall"
	"unsafe"
)

var (
	procEnumDisplayMonitors = modUser32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW     = modUser32.NewProc("GetMonitorInfoW")
	procGetForegroundWindow = modUser32.NewProc("GetForegroundWindow")
	procSetForegroundWindow = modUser32.NewProc("SetForegroundWindow")
)

type DisplayInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	X       int32  `json:"x"`
	Y       int32  `json:"y"`
	Width   int32  `json:"width"`
	Height  int32  `json:"height"`
	Primary bool   `json:"primary"`
	Work    Rect   `json:"work"`
}

type displayRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type monitorInfoEx struct {
	Size       uint32
	Monitor    displayRect
	Work       displayRect
	Flags      uint32
	DeviceName [32]uint16
}

func EnumerateDisplays() ([]DisplayInfo, error) {
	displays := []DisplayInfo{}
	var callbackErr error
	callback := syscall.NewCallback(func(monitor, _, _, _ uintptr) uintptr {
		info := monitorInfoEx{Size: uint32(unsafe.Sizeof(monitorInfoEx{}))}
		result, _, err := procGetMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info)))
		if result == 0 {
			callbackErr = fmt.Errorf("lecture d’un moniteur : %w", err)
			return 0
		}
		displays = append(displays, displayInfoFromMonitor(info))
		return 1
	})

	result, _, err := procEnumDisplayMonitors.Call(0, 0, callback, 0)
	if result == 0 {
		if callbackErr != nil {
			return nil, callbackErr
		}
		return nil, fmt.Errorf("énumération des moniteurs : %w", err)
	}
	if callbackErr != nil {
		return nil, callbackErr
	}
	sort.Slice(displays, func(left, right int) bool {
		if displays[left].X != displays[right].X {
			return displays[left].X < displays[right].X
		}
		return displays[left].Y < displays[right].Y
	})
	return displays, nil
}

func displayInfoFromMonitor(info monitorInfoEx) DisplayInfo {
	deviceID := syscall.UTF16ToString(info.DeviceName[:])
	return DisplayInfo{
		ID:      deviceID,
		Name:    deviceID,
		X:       info.Monitor.Left,
		Y:       info.Monitor.Top,
		Width:   info.Monitor.Right - info.Monitor.Left,
		Height:  info.Monitor.Bottom - info.Monitor.Top,
		Primary: info.Flags&1 != 0,
		Work:    rectFrom(info.Work),
	}
}

func WithForegroundWindow(hwnd uintptr, action func() error) error {
	if hwnd == 0 || action == nil {
		return fmt.Errorf("fenetre ou action invalide")
	}
	alive, _, _ := procIsWindow.Call(hwnd)
	if alive == 0 {
		return fmt.Errorf("fenetre fermee")
	}
	previous, _, _ := procGetForegroundWindow.Call()
	if previous != hwnd {
		if err := ForceForeground(hwnd); err != nil {
			return err
		}
		if previous != 0 {
			defer ForceForeground(previous)
		}
	}
	current, _, _ := procGetForegroundWindow.Call()
	if current != hwnd {
		return fmt.Errorf("la fenêtre choisie n’est pas active")
	}
	return action()
}
