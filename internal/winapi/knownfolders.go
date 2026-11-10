package winapi

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	modShell32 = syscall.NewLazyDLL("shell32.dll")
	modOle32   = syscall.NewLazyDLL("ole32.dll")

	procSHGetKnownFolderPath = modShell32.NewProc("SHGetKnownFolderPath")
	procCoTaskMemFree        = modOle32.NewProc("CoTaskMemFree")
)

// GUID structure Win32
type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// FOLDERID_Desktop : {B4BFCC3A-4D24-42B8-8E12-48FA4E42B7E4}
var FOLDERID_Desktop = GUID{
	Data1: 0xB4BFCC3A,
	Data2: 0x4D24,
	Data3: 0x42B8,
	Data4: [8]byte{0x8E, 0x12, 0x48, 0xFA, 0x4E, 0x42, 0xB7, 0xE4},
}

const (
	KF_FLAG_DEFAULT     = 0x00000000
	KF_FLAG_DONT_VERIFY = 0x00004000
)

// GetUserDesktopPath résout le chemin réel du bureau utilisateur via SHGetKnownFolderPath
// (gère la redirection OneDrive sans supposer ~/Desktop).
func GetUserDesktopPath() (string, error) {
	var pathPtr uintptr

	// 1. Essai avec KF_FLAG_DONT_VERIFY pour éviter l'échec 0x80070002 si le chemin OneDrive est virtuel ou distant
	ret, _, _ := procSHGetKnownFolderPath.Call(
		uintptr(unsafe.Pointer(&FOLDERID_Desktop)),
		uintptr(KF_FLAG_DONT_VERIFY),
		0, // current user token
		uintptr(unsafe.Pointer(&pathPtr)),
	)

	if ret == 0 && pathPtr != 0 {
		defer procCoTaskMemFree.Call(pathPtr)
		var length int
		p := (*[1 << 16]uint16)(unsafe.Pointer(pathPtr))
		for length = 0; p[length] != 0; length++ {
		}
		desktopPath := syscall.UTF16ToString(p[:length])
		if desktopPath != "" {
			return desktopPath, nil
		}
	}

	// 2. Fallback via le Registre Windows (User Shell Folders)
	if regPath, err := getDesktopFromRegistry(); err == nil && regPath != "" {
		return regPath, nil
	}

	return "", fmt.Errorf("impossible de résoudre le chemin du Bureau utilisateur (HRESULT: 0x%08X)", ret)
}

func getDesktopFromRegistry() (string, error) {
	modAdvapi32 := syscall.NewLazyDLL("advapi32.dll")
	procRegOpenKeyExW := modAdvapi32.NewProc("RegOpenKeyExW")
	procRegQueryValueExW := modAdvapi32.NewProc("RegQueryValueExW")
	procRegCloseKey := modAdvapi32.NewProc("RegCloseKey")

	const (
		HKEY_CURRENT_USER = 0x80000001
		KEY_READ          = 0x20019
	)

	subKey, _ := syscall.UTF16PtrFromString(`Software\Microsoft\Windows\CurrentVersion\Explorer\User Shell Folders`)
	var hKey uintptr
	ret, _, _ := procRegOpenKeyExW.Call(
		uintptr(HKEY_CURRENT_USER),
		uintptr(unsafe.Pointer(subKey)),
		0,
		uintptr(KEY_READ),
		uintptr(unsafe.Pointer(&hKey)),
	)
	if ret != 0 {
		return "", fmt.Errorf("RegOpenKeyExW a échoué : %d", ret)
	}
	defer procRegCloseKey.Call(hKey)

	valName, _ := syscall.UTF16PtrFromString("Desktop")
	var valType uint32
	var bufSize uint32 = 1024
	buf := make([]uint16, 512)

	ret, _, _ = procRegQueryValueExW.Call(
		hKey,
		uintptr(unsafe.Pointer(valName)),
		0,
		uintptr(unsafe.Pointer(&valType)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&bufSize)),
	)
	if ret != 0 {
		return "", fmt.Errorf("RegQueryValueExW a échoué : %d", ret)
	}

	rawPath := syscall.UTF16ToString(buf)
	// Développer les variables d'environnement (ex: %USERPROFILE%\Desktop ou %OneDrive%\Desktop)
	procExpandEnvironmentStringsW := modKernel32.NewProc("ExpandEnvironmentStringsW")
	rawPtr, _ := syscall.UTF16PtrFromString(rawPath)
	expBuf := make([]uint16, 1024)
	retExp, _, _ := procExpandEnvironmentStringsW.Call(
		uintptr(unsafe.Pointer(rawPtr)),
		uintptr(unsafe.Pointer(&expBuf[0])),
		uintptr(len(expBuf)),
	)
	if retExp == 0 {
		return rawPath, nil
	}

	return syscall.UTF16ToString(expBuf), nil
}
