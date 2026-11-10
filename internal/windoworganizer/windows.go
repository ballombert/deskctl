package windoworganizer

import (
	"fmt"
	"os"
	"strconv"

	"deskctl/internal/winapi"
)

type WindowsBackend struct{}

func (WindowsBackend) Inventory() (Inventory, error) {
	inventory := Inventory{Desktops: []Desktop{}, Displays: []winapi.DisplayInfo{}, Windows: []Window{}}
	desktops, err := winapi.EnumerateVirtualDesktops()
	if err != nil {
		return inventory, fmt.Errorf("bureaux Windows incompatibles ou indisponibles : %w", err)
	}
	for _, desktop := range desktops {
		inventory.Desktops = append(inventory.Desktops, Desktop{ID: desktop.ID, Name: desktop.DisplayName()})
	}
	if displays, displayErr := winapi.EnumerateDisplays(); displayErr == nil {
		inventory.Displays = displays
	}
	inventory.CurrentDesktopID, _ = winapi.CurrentDesktopID()
	windows, err := winapi.EnumerateUserWindows()
	if err != nil {
		return inventory, err
	}
	for _, window := range windows {
		if window.ProcessId == os.Getpid() {
			continue
		}
		item := Window{ID: strconv.FormatUint(window.WindowID, 10), ProcessID: uint32(window.ProcessId),
			Application: window.ProcessName, Title: window.WindowTitle, DesktopID: window.DesktopID}
		identity, readErr := winapi.ReadWindowIdentity(window.WindowHandle)
		if readErr != nil {
			item.Blocked, item.BlockCode = readErr.Error(), "identity"
			if winapi.IsProcessElevatedAboveUs(item.ProcessID) {
				item.Elevated = true
				item.Blocked = "Application administrateur : lancer deskctl en administrateur (" + readErr.Error() + ")"
			}
		} else {
			item = fromIdentity(item.ID, identity)
			item.Elevated = winapi.IsProcessElevatedAboveUs(item.ProcessID)
			item.Pinned, readErr = winapi.IsWindowPinned(window.WindowHandle)
			if readErr != nil {
				item.Blocked, item.BlockCode = fmt.Sprintf("Lecture de l’epinglage impossible : %v", readErr), "pin"
			} else if !item.Pinned && !hasDesktop(inventory.Desktops, item.DesktopID) {
				item.Blocked, item.BlockCode = "Bureau indisponible : aucun deplacement", "no-desktop"
			} else if movable, moveErr := winapi.CanWindowMoveDesktops(window.WindowHandle); !item.Pinned && moveErr == nil && !movable {
				item.Blocked, item.BlockCode = "Windows refuse de changer cette fenetre de bureau (fenetre systeme ou modale)", "immovable"
			}
		}
		item.Bounds, _ = winapi.VisibleWindowRect(window.WindowHandle)
		item.DisplayID, _ = winapi.DisplayIDForWindow(window.WindowHandle)
		inventory.Windows = append(inventory.Windows, item)
	}
	return inventory, nil
}

func fromIdentity(id string, identity winapi.WindowIdentity) Window {
	return Window{ID: id, ProcessID: identity.ProcessID, Application: identity.Application,
		Title: identity.Title, Class: identity.Class, Started: identity.Started, DesktopID: identity.DesktopID}
}

func (WindowsBackend) Move(expected Window, target string) error {
	handle, err := strconv.ParseUint(expected.ID, 10, strconv.IntSize)
	if err != nil || handle == 0 || expected.ProcessID == uint32(os.Getpid()) {
		return fmt.Errorf("identite de fenetre refusee")
	}
	identity, err := winapi.ReadWindowIdentity(uintptr(handle))
	if err != nil {
		return err
	}
	actual := fromIdentity(expected.ID, identity)
	actual.Pinned, err = winapi.IsWindowPinned(uintptr(handle))
	if err != nil {
		return err
	}
	if !sameWindow(expected, actual) {
		return fmt.Errorf("fenetre modifiee avant le mouvement")
	}
	if target == winapi.AllDesktopsID {
		return winapi.SetWindowPinned(uintptr(handle), true)
	}
	if expected.Pinned {
		if err := winapi.SetWindowPinned(uintptr(handle), false); err != nil {
			return err
		}
	}
	return winapi.MoveWindowToDesktop(uintptr(handle), target)
}
