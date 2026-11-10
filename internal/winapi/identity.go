package winapi

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	procIsWindow        = modUser32.NewProc("IsWindow")
	procGetClassNameW   = modUser32.NewProc("GetClassNameW")
	procGetProcessTimes = modKernel32.NewProc("GetProcessTimes")
)

type WindowIdentity struct {
	ProcessID   uint32
	Application string
	Title       string
	Class       string
	Started     string
	DesktopID   string
}

func ReadWindowIdentity(hwnd uintptr) (WindowIdentity, error) {
	var identity WindowIdentity
	alive, _, _ := procIsWindow.Call(hwnd)
	if alive == 0 {
		return identity, fmt.Errorf("fenetre fermee")
	}
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&identity.ProcessID)))
	if identity.ProcessID == 0 || identity.ProcessID == 4 {
		return identity, fmt.Errorf("processus de fenetre invalide")
	}
	name, err := GetProcessName(identity.ProcessID)
	if err != nil {
		return identity, fmt.Errorf("identite du processus : %w", err)
	}
	identity.Application = name
	process, _, _ := procOpenProcess.Call(PROCESS_QUERY_LIMITED_INFORMATION, 0, uintptr(identity.ProcessID))
	if process == 0 {
		return identity, fmt.Errorf("ouverture du processus : %w", syscall.GetLastError())
	}
	defer procCloseHandle.Call(process)
	var created, exited, kernel, user syscall.Filetime
	result, _, _ := procGetProcessTimes.Call(process,
		uintptr(unsafe.Pointer(&created)), uintptr(unsafe.Pointer(&exited)),
		uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
	if result == 0 {
		return identity, fmt.Errorf("date du processus : %w", syscall.GetLastError())
	}
	identity.Started = fmt.Sprintf("%08x%08x", created.HighDateTime, created.LowDateTime)
	class := make([]uint16, 256)
	length, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
	if length == 0 {
		return identity, fmt.Errorf("classe de fenetre inaccessible")
	}
	identity.Class = syscall.UTF16ToString(class)
	identity.Title = windowTitle(hwnd)
	err = withVirtualDesktopManager(func(manager *virtualDesktopManager) error {
		var readErr error
		identity.DesktopID, readErr = manager.getWindowDesktopID(hwnd)
		return readErr
	})
	if err != nil {
		return identity, err
	}
	var currentPID uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&currentPID)))
	alive, _, _ = procIsWindow.Call(hwnd)
	if alive == 0 || currentPID != identity.ProcessID {
		return identity, fmt.Errorf("fenetre modifiee pendant la lecture")
	}
	return identity, nil
}
