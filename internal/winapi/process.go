package winapi

import (
	"strings"
	"syscall"
	"unsafe"
)

var (
	procCreateToolhelp32Snapshot = modKernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW          = modKernel32.NewProc("Process32FirstW")
	procProcess32NextW           = modKernel32.NewProc("Process32NextW")
)

type processEntry32 struct {
	Size            uint32
	Usage           uint32
	ProcessID       uint32
	DefaultHeapID   uintptr
	ModuleID        uint32
	Threads         uint32
	ParentProcessID uint32
	PriClassBase    int32
	Flags           uint32
	ExeFile         [260]uint16
}

// ProcessRunning reports whether a process with this executable name (for example "FancyWM.exe") exists.
func ProcessRunning(executable string) bool {
	const snapProcess = 0x2
	snapshot, _, _ := procCreateToolhelp32Snapshot.Call(snapProcess, 0)
	if snapshot == 0 || snapshot == ^uintptr(0) {
		return false
	}
	defer syscall.CloseHandle(syscall.Handle(snapshot))
	entry := processEntry32{Size: uint32(unsafe.Sizeof(processEntry32{}))}
	ok, _, _ := procProcess32FirstW.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
	for ok != 0 {
		if strings.EqualFold(syscall.UTF16ToString(entry.ExeFile[:]), executable) {
			return true
		}
		ok, _, _ = procProcess32NextW.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
	}
	return false
}
