package fluent

import (
	"syscall"
	"unsafe"
)

// WindowsBuild uses the version independent of the executable's manifest.
// Failure keeps the standard window and menu path.
func WindowsBuild() uint32 {
	var version struct {
		Size, Major, Minor, Build, Platform uint32
		ServicePack                         [128]uint16
	}
	version.Size = uint32(unsafe.Sizeof(version))
	proc := syscall.NewLazyDLL("ntdll.dll").NewProc("RtlGetVersion")
	if err := proc.Find(); err != nil {
		return 0
	}
	status, _, _ := proc.Call(uintptr(unsafe.Pointer(&version)))
	if status != 0 {
		return 0
	}
	return version.Build
}
