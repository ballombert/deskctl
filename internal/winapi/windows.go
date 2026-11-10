package winapi

import (
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	modKernel32 = syscall.NewLazyDLL("kernel32.dll")

	modUser32 = syscall.NewLazyDLL("user32.dll")

	procGetWindowTextW           = modUser32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW     = modUser32.NewProc("GetWindowTextLengthW")
	procGetWindowThreadProcessId = modUser32.NewProc("GetWindowThreadProcessId")

	procOpenProcess                = modKernel32.NewProc("OpenProcess")
	procCloseHandle                = modKernel32.NewProc("CloseHandle")
	procQueryFullProcessImageNameW = modKernel32.NewProc("QueryFullProcessImageNameW")
)

const PROCESS_QUERY_LIMITED_INFORMATION = 0x1000

const AllDesktopsID = "00000000-0000-0000-0000-000000000000"

// WindowInfo représente la structure JSON demandée pour l'inventaire des fenêtres.
type WindowInfo struct {
	ProcessId    int     `json:"ProcessId"`
	ProcessName  string  `json:"ProcessName"`
	WindowTitle  string  `json:"WindowTitle"`
	DesktopID    string  `json:"DesktopId"`
	DesktopName  string  `json:"DesktopName"`
	WindowID     uint64  `json:"WindowId"`
	WindowHandle uintptr `json:"-"`
}

// GetProcessName retourne le nom exécutable sans extension pour un PID donné.
func GetProcessName(pid uint32) (string, error) {
	if pid == 0 || pid == 4 {
		return "System", nil
	}

	hProcess, _, _ := procOpenProcess.Call(
		uintptr(PROCESS_QUERY_LIMITED_INFORMATION),
		0,
		uintptr(pid),
	)
	if hProcess == 0 {
		return "", syscall.GetLastError()
	}
	defer procCloseHandle.Call(hProcess)

	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	ret, _, _ := procQueryFullProcessImageNameW.Call(
		hProcess,
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if ret == 0 {
		return "", syscall.GetLastError()
	}

	fullPath := syscall.UTF16ToString(buf[:size])
	base := filepath.Base(fullPath)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext), nil
}

// EnumerateUserWindows returns every application view, including views on inactive virtual desktops.
func EnumerateUserWindows() ([]WindowInfo, error) {
	desktops, err := EnumerateVirtualDesktops()
	if err != nil {
		return nil, fmt.Errorf("lecture des noms de bureaux : %w", err)
	}
	desktopNames := make(map[string]string, len(desktops))
	for _, desktop := range desktops {
		desktopNames[desktop.ID] = desktop.Name
	}
	var windowsList []WindowInfo
	err = withInternalDesktopManager(func(manager *internalDesktopManager) error {
		views, err := manager.getApplicationViews()
		if err != nil {
			return err
		}
		defer releaseCOMObject(views)

		var count uint32
		if _, err := callCOMMethod(views, 3, uintptr(unsafe.Pointer(&count))); err != nil {
			return fmt.Errorf("lecture du nombre de fenetres : %w", err)
		}
		if count > 10000 {
			return fmt.Errorf("nombre de fenetres inattendu : %d", count)
		}
		publicPointer, err := createCOMObject(clsidVirtualDesktopManager, iidVirtualDesktopManager, 1)
		if err != nil {
			return fmt.Errorf("activation d'IVirtualDesktopManager : %w", err)
		}
		publicManager := &virtualDesktopManager{instance: publicPointer}
		defer publicManager.release()

		seenHandles := make(map[uintptr]bool, count)
		for index := uint32(0); index < count; index++ {
			view, err := getObjectArrayItem(views, uintptr(index), iidApplicationView)
			if err != nil {
				return fmt.Errorf("lecture de la vue %d : %w", index+1, err)
			}
			hwnd, windowErr := getApplicationViewWindow(view)
			releaseCOMObject(view)
			if windowErr != nil {
				return fmt.Errorf("lecture du HWND de la vue %d : %w", index+1, windowErr)
			}
			if hwnd == 0 || seenHandles[hwnd] {
				continue
			}
			var pid uint32
			procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
			if pid == 0 || pid == 4 {
				continue
			}
			processName, err := GetProcessName(pid)
			if err != nil || processName == "" {
				processName = "Inconnu"
			}
			desktopID, err := publicManager.getWindowDesktopID(hwnd)
			if err != nil {
				return fmt.Errorf("lecture du bureau de la fenetre %d : %w", hwnd, err)
			}
			desktopName := desktopNames[desktopID]
			if desktopName == "" {
				if desktopID == AllDesktopsID {
					desktopName = "Tous les bureaux"
				} else {
					desktopName = fmt.Sprintf("Bureau inconnu (%s)", desktopID[:8])
				}
			}
			seenHandles[hwnd] = true
			windowsList = append(windowsList, WindowInfo{
				ProcessId:    int(pid),
				ProcessName:  processName,
				WindowTitle:  windowTitle(hwnd),
				DesktopID:    desktopID,
				DesktopName:  desktopName,
				WindowID:     uint64(hwnd),
				WindowHandle: hwnd,
			})
		}
		return nil
	})
	return windowsList, err
}

func windowTitle(hwnd uintptr) string {
	length, _, _ := procGetWindowTextLengthW.Call(hwnd)
	if length == 0 {
		return ""
	}
	buffer := make([]uint16, length+1)
	procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buffer[0])), uintptr(length+1))
	return strings.TrimSpace(syscall.UTF16ToString(buffer))
}
