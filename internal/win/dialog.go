//go:build windows

package win

import (
	"runtime"
	"syscall"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
)

var (
	clsidFileOpenDialog = ole.NewGUID("{DC1C5A9C-E88A-4DDE-A5A1-60F82A20AEF7}")
	iidIFileOpenDialog  = ole.NewGUID("{D57C7288-D4AD-4768-BE02-9D969532D960}")
	iidIShellItem       = ole.NewGUID("{43826D1E-E718-42EE-BC55-A1E261C37BFE}")

	pSHCreateItemFromParsingName = shell32.NewProc("SHCreateItemFromParsingName")
	pCoTaskMemFree               = windows.NewLazySystemDLL("ole32.dll").NewProc("CoTaskMemFree")
)

// comCall invokes method idx of a raw COM interface pointer.
func comCall(obj uintptr, idx int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(idx)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{obj}, args...)...)
	return r
}

// PickFolderDialog shows the standard folder picker and returns the chosen
// folder, or "" when cancelled. It needs a COM apartment on the calling
// (locked) thread.
func PickFolderDialog(owner uintptr, title, initialDir string) string {
	unk, err := ole.CreateInstance(clsidFileOpenDialog, iidIFileOpenDialog)
	if err != nil {
		return ""
	}
	dlg := uintptr(unsafe.Pointer(unk))
	defer comCall(dlg, 2) // Release
	// IFileDialog: 9 SetOptions, 12 SetFolder, 17 SetTitle, 20 GetResult; 3 Show.
	const fosPickFolders, fosForceFileSystem, fosPathMustExist = 0x20, 0x40, 0x800
	comCall(dlg, 9, fosPickFolders|fosForceFileSystem|fosPathMustExist)
	// Out-parameters live on the heap: the calls below go through comCall.
	if title != "" {
		t := utf16Ptr(title)
		comCall(dlg, 17, uintptr(unsafe.Pointer(t)))
		runtime.KeepAlive(t)
	}
	if initialDir != "" {
		start := new(uintptr)
		p := utf16Ptr(initialDir)
		if r, _, _ := pSHCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(p)), 0, uintptr(unsafe.Pointer(iidIShellItem)), uintptr(unsafe.Pointer(start))); r == 0 && *start != 0 {
			comCall(dlg, 12, *start)
			comCall(*start, 2)
		}
	}
	if comCall(dlg, 3, owner) != 0 { // cancelled
		return ""
	}
	item := new(uintptr)
	if comCall(dlg, 20, uintptr(unsafe.Pointer(item))) != 0 || *item == 0 {
		return ""
	}
	defer comCall(*item, 2)
	// IShellItem: 5 GetDisplayName(SIGDN_FILESYSPATH).
	name := new(*uint16)
	if comCall(*item, 5, 0x80058000, uintptr(unsafe.Pointer(name))) != 0 || *name == nil {
		return ""
	}
	defer pCoTaskMemFree.Call(uintptr(unsafe.Pointer(*name)))
	return windows.UTF16PtrToString(*name)
}
