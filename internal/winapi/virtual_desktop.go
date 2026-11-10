package winapi

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

var (
	procCoInitializeEx            = modOle32.NewProc("CoInitializeEx")
	procCoUninitialize            = modOle32.NewProc("CoUninitialize")
	procCoCreateInstance          = modOle32.NewProc("CoCreateInstance")
	procWindowsGetStringRawBuffer = syscall.NewLazyDLL("combase.dll").NewProc("WindowsGetStringRawBuffer")
	procWindowsDeleteString       = syscall.NewLazyDLL("combase.dll").NewProc("WindowsDeleteString")
	procRtlMoveMemory             = modKernel32.NewProc("RtlMoveMemory")
)

const coinitApartmentThreaded = 0x2

var (
	clsidImmersiveShell = GUID{
		Data1: 0xC2F03A33, Data2: 0x21F5, Data3: 0x47FA,
		Data4: [8]byte{0xB4, 0xBB, 0x15, 0x63, 0x62, 0xA2, 0xF2, 0x39},
	}
	clsidVirtualDesktopManagerInternal = GUID{
		Data1: 0xC5E0CDCA, Data2: 0x7B6E, Data3: 0x41B2,
		Data4: [8]byte{0x9F, 0xC4, 0xD9, 0x39, 0x75, 0xCC, 0x46, 0x7B},
	}
	iidServiceProvider = GUID{
		Data1: 0x6D5140C1, Data2: 0x7436, Data3: 0x11CE,
		Data4: [8]byte{0x80, 0x34, 0x00, 0xAA, 0x00, 0x60, 0x09, 0xFA},
	}
	iidVirtualDesktop = GUID{
		Data1: 0x3F07F4BE, Data2: 0xB107, Data3: 0x441A,
		Data4: [8]byte{0xAF, 0x0F, 0x39, 0xD8, 0x25, 0x29, 0x07, 0x2C},
	}
	iidApplicationView = GUID{
		Data1: 0x372E1D3B, Data2: 0x38D3, Data3: 0x42E4,
		Data4: [8]byte{0xA1, 0x5B, 0x8A, 0xB2, 0xB1, 0x78, 0xF5, 0x13},
	}
	iidApplicationViewCollection = GUID{
		Data1: 0x1841C6D7, Data2: 0x4F9D, Data3: 0x42C0,
		Data4: [8]byte{0xAF, 0x41, 0x87, 0x47, 0x53, 0x8F, 0x10, 0xE5},
	}
	iidObjectArray = GUID{
		Data1: 0x92CA9DCD, Data2: 0x5622, Data3: 0x4BBA,
		Data4: [8]byte{0xA8, 0x05, 0x5E, 0x9F, 0x54, 0x1B, 0xD8, 0xC9},
	}
	iidVirtualDesktopManagerInternal = GUID{
		Data1: 0x53F5CA0B, Data2: 0x158F, Data3: 0x4124,
		Data4: [8]byte{0x90, 0x0C, 0x05, 0x71, 0x58, 0x06, 0x0B, 0x27},
	}
	clsidVirtualDesktopManager = GUID{
		Data1: 0xAA509086, Data2: 0x5CA9, Data3: 0x4C25,
		Data4: [8]byte{0x8F, 0x95, 0x58, 0x9D, 0x3C, 0x07, 0xB4, 0x8A},
	}
	iidVirtualDesktopManager = GUID{
		Data1: 0xA5CD92FF, Data2: 0x29BE, Data3: 0x454C,
		Data4: [8]byte{0x8D, 0x04, 0xD8, 0x28, 0x79, 0xFB, 0x3F, 0x1B},
	}
	clsidVirtualDesktopPinnedApps = GUID{
		Data1: 0xB5A399E7, Data2: 0x1C87, Data3: 0x46B8,
		Data4: [8]byte{0x88, 0xE9, 0xFC, 0x57, 0x47, 0xB1, 0x71, 0xBD},
	}
	iidVirtualDesktopPinnedApps = GUID{
		Data1: 0x4CE81583, Data2: 0x1E4C, Data3: 0x4632,
		Data4: [8]byte{0xA6, 0x21, 0x07, 0xA5, 0x35, 0x43, 0x14, 0x8F},
	}
)

type virtualDesktopManager struct {
	instance unsafe.Pointer
}

type VirtualDesktopInfo struct {
	Index uint32
	ID    string
	Name  string
}

func (desktop VirtualDesktopInfo) DisplayName() string {
	if name := strings.TrimSpace(desktop.Name); name != "" {
		return name
	}
	return fmt.Sprintf("Bureau %d", desktop.Index+1)
}

type internalDesktopManager struct {
	instance unsafe.Pointer
	provider unsafe.Pointer
}

func EnumerateVirtualDesktops() ([]VirtualDesktopInfo, error) {
	var desktops []VirtualDesktopInfo
	err := withInternalDesktopManager(func(manager *internalDesktopManager) error {
		array, err := manager.getDesktops()
		if err != nil {
			return err
		}
		defer releaseCOMObject(array)

		var count uint32
		if _, err := callCOMMethod(array, 3, uintptr(unsafe.Pointer(&count))); err != nil {
			return fmt.Errorf("lecture du nombre de bureaux : %w", err)
		}
		if count == 0 || count > 256 {
			return fmt.Errorf("nombre de bureaux inattendu : %d", count)
		}
		for index := uint32(0); index < count; index++ {
			desktop, err := getObjectArrayItem(array, uintptr(index), iidVirtualDesktop)
			if err != nil {
				return fmt.Errorf("lecture du bureau %d : %w", index+1, err)
			}
			id, name, readErr := readDesktopInfo(desktop)
			releaseCOMObject(desktop)
			if readErr != nil {
				return fmt.Errorf("lecture du bureau %d : %w", index+1, readErr)
			}
			desktops = append(desktops, VirtualDesktopInfo{Index: index, ID: id, Name: name})
		}
		return nil
	})
	return desktops, err
}

func withInternalDesktopManager(run func(*internalDesktopManager) error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hresult, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded)
	if int32(uint32(hresult)) < 0 {
		return fmt.Errorf("CoInitializeEx a echoue (HRESULT 0x%08X)", uint32(hresult))
	}
	defer procCoUninitialize.Call()

	provider, err := createCOMObject(clsidImmersiveShell, iidServiceProvider, 4)
	if err != nil {
		return fmt.Errorf("activation d'ImmersiveShell : %w", err)
	}
	defer releaseCOMObject(provider)

	var managerPointer unsafe.Pointer
	_, err = callCOMMethod(provider, 3,
		uintptr(unsafe.Pointer(&clsidVirtualDesktopManagerInternal)),
		uintptr(unsafe.Pointer(&iidVirtualDesktopManagerInternal)),
		uintptr(unsafe.Pointer(&managerPointer)))
	if err != nil {
		return fmt.Errorf("requete du gestionnaire interne de bureaux : %w", err)
	}
	if managerPointer == nil {
		return fmt.Errorf("le gestionnaire interne de bureaux est absent")
	}
	manager := &internalDesktopManager{instance: managerPointer, provider: provider}
	defer manager.release()
	return run(manager)
}

func createCOMObject(classID, interfaceID GUID, context uintptr) (unsafe.Pointer, error) {
	var instance unsafe.Pointer
	hresult, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&classID)), 0, context,
		uintptr(unsafe.Pointer(&interfaceID)), uintptr(unsafe.Pointer(&instance)))
	if int32(uint32(hresult)) < 0 {
		return nil, fmt.Errorf("CoCreateInstance a echoue (HRESULT 0x%08X)", uint32(hresult))
	}
	if instance == nil {
		return nil, fmt.Errorf("CoCreateInstance a retourne un objet nul")
	}
	return instance, nil
}

func (manager *internalDesktopManager) getDesktops() (unsafe.Pointer, error) {
	var desktops unsafe.Pointer
	_, err := callCOMMethod(manager.instance, 7, uintptr(unsafe.Pointer(&desktops)))
	if err != nil {
		return nil, fmt.Errorf("IVirtualDesktopManagerInternal.GetDesktops : %w", err)
	}
	if desktops == nil {
		return nil, fmt.Errorf("GetDesktops a retourne un tableau nul")
	}
	return desktops, nil
}

func (manager *internalDesktopManager) getApplicationViews() (unsafe.Pointer, error) {
	var collection unsafe.Pointer
	_, err := callCOMMethod(manager.provider, 3,
		uintptr(unsafe.Pointer(&iidApplicationViewCollection)),
		uintptr(unsafe.Pointer(&iidApplicationViewCollection)),
		uintptr(unsafe.Pointer(&collection)))
	if err != nil {
		return nil, fmt.Errorf("requete de IApplicationViewCollection : %w", err)
	}
	if collection == nil {
		return nil, fmt.Errorf("IApplicationViewCollection est absente")
	}
	defer releaseCOMObject(collection)

	var views unsafe.Pointer
	_, err = callCOMMethod(collection, 3, uintptr(unsafe.Pointer(&views)))
	if err != nil {
		return nil, fmt.Errorf("IApplicationViewCollection.GetViews : %w", err)
	}
	if views == nil {
		return nil, fmt.Errorf("GetViews a retourne un tableau nul")
	}
	return views, nil
}

func (manager *internalDesktopManager) getApplicationView(hwnd uintptr) (unsafe.Pointer, error) {
	var collection unsafe.Pointer
	_, err := callCOMMethod(manager.provider, 3,
		uintptr(unsafe.Pointer(&iidApplicationViewCollection)),
		uintptr(unsafe.Pointer(&iidApplicationViewCollection)),
		uintptr(unsafe.Pointer(&collection)))
	if err != nil {
		return nil, fmt.Errorf("requete de IApplicationViewCollection : %w", err)
	}
	if collection == nil {
		return nil, fmt.Errorf("IApplicationViewCollection est absente")
	}
	defer releaseCOMObject(collection)

	var view unsafe.Pointer
	if _, err := callCOMMethod(collection, 6, hwnd, uintptr(unsafe.Pointer(&view))); err != nil {
		return nil, fmt.Errorf("lecture de la vue de fenetre : %w", err)
	}
	if view == nil {
		return nil, fmt.Errorf("aucune vue applicative pour cette fenetre")
	}
	return view, nil
}

func getApplicationViewWindow(view unsafe.Pointer) (uintptr, error) {
	var hwnd uintptr
	_, err := callCOMMethod(view, 9, uintptr(unsafe.Pointer(&hwnd)))
	if err != nil {
		return 0, fmt.Errorf("IApplicationView.GetThumbnailWindow : %w", err)
	}
	return hwnd, nil
}

func (manager *internalDesktopManager) release() {
	releaseCOMObject(manager.instance)
}

func getObjectArrayItem(array unsafe.Pointer, index uintptr, interfaceID GUID) (unsafe.Pointer, error) {
	var item unsafe.Pointer
	_, err := callCOMMethod(array, 4, index, uintptr(unsafe.Pointer(&interfaceID)), uintptr(unsafe.Pointer(&item)))
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, fmt.Errorf("IObjectArray.GetAt a retourne un objet nul")
	}
	return item, nil
}

func readDesktopInfo(desktop unsafe.Pointer) (string, string, error) {
	var id GUID
	if _, err := callCOMMethod(desktop, 4, uintptr(unsafe.Pointer(&id))); err != nil {
		return "", "", fmt.Errorf("IVirtualDesktop.GetId : %w", err)
	}
	var name uintptr
	if _, err := callCOMMethod(desktop, 5, uintptr(unsafe.Pointer(&name))); err != nil {
		return "", "", fmt.Errorf("IVirtualDesktop.GetName : %w", err)
	}
	defer procWindowsDeleteString.Call(name)
	return FormatDesktopID(id), readHString(name), nil
}

func readHString(value uintptr) string {
	var length uint32
	buffer, _, _ := procWindowsGetStringRawBuffer.Call(value, uintptr(unsafe.Pointer(&length)))
	if buffer == 0 || length == 0 {
		return ""
	}
	text := make([]uint16, length)
	procRtlMoveMemory.Call(uintptr(unsafe.Pointer(unsafe.SliceData(text))), buffer, uintptr(length)*2)
	runtime.KeepAlive(text)
	return syscall.UTF16ToString(text)
}

func callCOMMethod(instance unsafe.Pointer, methodIndex uintptr, args ...uintptr) (uintptr, error) {
	vtable := *(*unsafe.Pointer)(instance)
	method := *(*uintptr)(unsafe.Add(vtable, methodIndex*unsafe.Sizeof(uintptr(0))))
	callArgs := append([]uintptr{uintptr(instance)}, args...)
	hresult, _, _ := syscall.SyscallN(method, callArgs...)
	runtime.KeepAlive(instance)
	if int32(uint32(hresult)) < 0 {
		return hresult, fmt.Errorf("appel COM a echoue (HRESULT 0x%08X)", uint32(hresult))
	}
	return hresult, nil
}

func releaseCOMObject(instance unsafe.Pointer) {
	if instance != nil {
		_, _ = callCOMMethod(instance, 2)
	}
}

func ParseDesktopID(value string) (GUID, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(strings.TrimSuffix(value, "}"), "{")
	parts := strings.Split(value, "-")
	if len(parts) != 5 || len(parts[0]) != 8 || len(parts[1]) != 4 || len(parts[2]) != 4 || len(parts[3]) != 4 || len(parts[4]) != 12 {
		return GUID{}, fmt.Errorf("GUID de bureau invalide %q", value)
	}
	data1, err := hex.DecodeString(parts[0])
	if err != nil {
		return GUID{}, fmt.Errorf("GUID de bureau invalide %q", value)
	}
	data2, err := hex.DecodeString(parts[1])
	if err != nil {
		return GUID{}, fmt.Errorf("GUID de bureau invalide %q", value)
	}
	data3, err := hex.DecodeString(parts[2])
	if err != nil {
		return GUID{}, fmt.Errorf("GUID de bureau invalide %q", value)
	}
	data4, err := hex.DecodeString(parts[3] + parts[4])
	if err != nil {
		return GUID{}, fmt.Errorf("GUID de bureau invalide %q", value)
	}
	var id GUID
	id.Data1 = binary.BigEndian.Uint32(data1)
	id.Data2 = binary.BigEndian.Uint16(data2)
	id.Data3 = binary.BigEndian.Uint16(data3)
	copy(id.Data4[:], data4)
	return id, nil
}

func FormatDesktopID(id GUID) string {
	return fmt.Sprintf("%08x-%04x-%04x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		id.Data1, id.Data2, id.Data3,
		id.Data4[0], id.Data4[1], id.Data4[2], id.Data4[3],
		id.Data4[4], id.Data4[5], id.Data4[6], id.Data4[7])
}

func withVirtualDesktopManager(run func(*virtualDesktopManager) error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hresult, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded)
	if int32(uint32(hresult)) < 0 {
		return fmt.Errorf("CoInitializeEx a echoue (HRESULT 0x%08X)", uint32(hresult))
	}
	defer procCoUninitialize.Call()

	var instance unsafe.Pointer
	hresult, _, _ = procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidVirtualDesktopManager)),
		0,
		1,
		uintptr(unsafe.Pointer(&iidVirtualDesktopManager)),
		uintptr(unsafe.Pointer(&instance)),
	)
	if int32(uint32(hresult)) < 0 {
		return fmt.Errorf("CoCreateInstance(IVirtualDesktopManager) a echoue (HRESULT 0x%08X)", uint32(hresult))
	}
	manager := &virtualDesktopManager{instance: instance}
	defer manager.release()
	return run(manager)
}

func (manager *virtualDesktopManager) call(methodIndex uintptr, args ...uintptr) (uintptr, error) {
	vtable := *(*unsafe.Pointer)(manager.instance)
	method := *(*uintptr)(unsafe.Add(vtable, methodIndex*unsafe.Sizeof(uintptr(0))))
	callArgs := append([]uintptr{uintptr(manager.instance)}, args...)
	hresult, _, _ := syscall.SyscallN(method, callArgs...)
	runtime.KeepAlive(manager)
	if int32(uint32(hresult)) < 0 {
		return hresult, fmt.Errorf("IVirtualDesktopManager a echoue (HRESULT 0x%08X)", uint32(hresult))
	}
	return hresult, nil
}

func (manager *virtualDesktopManager) release() {
	manager.call(2)
}

func (manager *virtualDesktopManager) getWindowDesktopID(hwnd uintptr) (string, error) {
	var id GUID
	if _, err := manager.call(4, hwnd, uintptr(unsafe.Pointer(&id))); err != nil {
		return "", err
	}
	return FormatDesktopID(id), nil
}

func (manager *virtualDesktopManager) moveWindowToDesktop(hwnd uintptr, desktopID string) error {
	id, err := ParseDesktopID(desktopID)
	if err != nil {
		return err
	}
	_, err = manager.call(5, hwnd, uintptr(unsafe.Pointer(&id)))
	runtime.KeepAlive(id)
	return err
}

// MoveWindowToDesktop uses the shell-internal MoveViewToDesktop first: the public
// IVirtualDesktopManager returns E_ACCESSDENIED for windows owned by other processes.
func MoveWindowToDesktop(hwnd uintptr, desktopID string) error {
	internalErr := moveViewToDesktop(hwnd, desktopID)
	if internalErr == nil {
		return nil
	}
	publicErr := withVirtualDesktopManager(func(manager *virtualDesktopManager) error {
		return manager.moveWindowToDesktop(hwnd, desktopID)
	})
	if publicErr == nil {
		return nil
	}
	return fmt.Errorf("deplacement refuse : interne=%v ; public=%v", internalErr, publicErr)
}

func moveViewToDesktop(hwnd uintptr, desktopID string) error {
	if hwnd == 0 {
		return fmt.Errorf("fenetre invalide")
	}
	id, err := ParseDesktopID(desktopID)
	if err != nil {
		return err
	}
	return withInternalDesktopManager(func(manager *internalDesktopManager) error {
		view, err := manager.getApplicationView(hwnd)
		if err != nil {
			return err
		}
		defer releaseCOMObject(view)
		var desktop unsafe.Pointer
		if _, err := callCOMMethod(manager.instance, 14, uintptr(unsafe.Pointer(&id)), uintptr(unsafe.Pointer(&desktop))); err != nil {
			return fmt.Errorf("IVirtualDesktopManagerInternal.FindDesktop : %w", err)
		}
		if desktop == nil {
			return fmt.Errorf("bureau introuvable")
		}
		defer releaseCOMObject(desktop)
		if _, err := callCOMMethod(manager.instance, 4, uintptr(view), uintptr(desktop)); err != nil {
			return fmt.Errorf("IVirtualDesktopManagerInternal.MoveViewToDesktop : %w", err)
		}
		runtime.KeepAlive(id)
		return nil
	})
}

// CanWindowMoveDesktops asks the shell whether the view may change desktop.
func CanWindowMoveDesktops(hwnd uintptr) (bool, error) {
	var allowed bool
	err := withInternalDesktopManager(func(manager *internalDesktopManager) error {
		view, err := manager.getApplicationView(hwnd)
		if err != nil {
			return err
		}
		defer releaseCOMObject(view)
		var result int32
		if _, err := callCOMMethod(manager.instance, 5, uintptr(view), uintptr(unsafe.Pointer(&result))); err != nil {
			return fmt.Errorf("IVirtualDesktopManagerInternal.CanViewMoveDesktops : %w", err)
		}
		allowed = result != 0
		return nil
	})
	return allowed, err
}

// CurrentDesktopID returns the GUID of the active virtual desktop.
func CurrentDesktopID() (string, error) {
	var id string
	err := withInternalDesktopManager(func(manager *internalDesktopManager) error {
		var desktop unsafe.Pointer
		if _, err := callCOMMethod(manager.instance, 6, uintptr(unsafe.Pointer(&desktop))); err != nil {
			return fmt.Errorf("IVirtualDesktopManagerInternal.GetCurrentDesktop : %w", err)
		}
		if desktop == nil {
			return fmt.Errorf("bureau courant absent")
		}
		defer releaseCOMObject(desktop)
		var readErr error
		id, _, readErr = readDesktopInfo(desktop)
		return readErr
	})
	return id, err
}

func IsWindowPinned(hwnd uintptr) (bool, error) {
	if hwnd == 0 {
		return false, fmt.Errorf("fenetre invalide")
	}
	var pinned bool
	err := withInternalDesktopManager(func(manager *internalDesktopManager) error {
		view, err := manager.getApplicationView(hwnd)
		if err != nil {
			return err
		}
		defer releaseCOMObject(view)

		var pinnedApps unsafe.Pointer
		if _, err := callCOMMethod(manager.provider, 3,
			uintptr(unsafe.Pointer(&clsidVirtualDesktopPinnedApps)),
			uintptr(unsafe.Pointer(&iidVirtualDesktopPinnedApps)),
			uintptr(unsafe.Pointer(&pinnedApps))); err != nil {
			return fmt.Errorf("requete de IVirtualDesktopPinnedApps : %w", err)
		}
		if pinnedApps == nil {
			return fmt.Errorf("IVirtualDesktopPinnedApps est absente")
		}
		defer releaseCOMObject(pinnedApps)

		var result int32
		if _, err := callCOMMethod(pinnedApps, 6, uintptr(view), uintptr(unsafe.Pointer(&result))); err != nil {
			return fmt.Errorf("verification de l'epinglage : %w", err)
		}
		pinned = result != 0
		return nil
	})
	return pinned, err
}

func SetWindowPinned(hwnd uintptr, pinned bool) error {
	if hwnd == 0 {
		return fmt.Errorf("fenetre invalide")
	}
	return withInternalDesktopManager(func(manager *internalDesktopManager) error {
		view, err := manager.getApplicationView(hwnd)
		if err != nil {
			return err
		}
		defer releaseCOMObject(view)

		var pinnedApps unsafe.Pointer
		if _, err := callCOMMethod(manager.provider, 3,
			uintptr(unsafe.Pointer(&clsidVirtualDesktopPinnedApps)),
			uintptr(unsafe.Pointer(&iidVirtualDesktopPinnedApps)),
			uintptr(unsafe.Pointer(&pinnedApps))); err != nil {
			return fmt.Errorf("requete de IVirtualDesktopPinnedApps : %w", err)
		}
		if pinnedApps == nil {
			return fmt.Errorf("IVirtualDesktopPinnedApps est absente")
		}
		defer releaseCOMObject(pinnedApps)

		method := uintptr(8)
		if pinned {
			method = 7
		}
		if _, err := callCOMMethod(pinnedApps, method, uintptr(view)); err != nil {
			return fmt.Errorf("modification de l'epinglage : %w", err)
		}
		return nil
	})
}
