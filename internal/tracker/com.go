//go:build windows

package tracker

import (
	"strings"
	"syscall"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// vcall invokes method idx of a raw COM interface pointer.
func vcall(obj uintptr, idx int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(idx)*unsafe.Sizeof(uintptr(0))))
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, obj)
	all = append(all, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return r
}

func release(obj uintptr) {
	if obj != 0 {
		vcall(obj, 2)
	}
}

var (
	clsidCUIAutomation       = ole.NewGUID("{FF48DBA4-60EF-4201-AA87-54103EEF594E}")
	iidIUIAutomation         = ole.NewGUID("{30CBE57D-D9D0-452A-AB13-7AC5AC4825EE}")
	clsidShellLink           = ole.NewGUID("{00021401-0000-0000-C000-000000000046}")
	iidIShellLinkW           = ole.NewGUID("{000214F9-0000-0000-C000-000000000046}")
	iidIPersistFile          = ole.NewGUID("{0000010B-0000-0000-C000-000000000046}")
	clsidMMDeviceEnumerator  = ole.NewGUID("{BCDE0395-E52F-467C-8E3D-C4579291692E}")
	iidIMMDeviceEnumerator   = ole.NewGUID("{A95664D2-9614-4F35-A746-DE8DB63617E6}")
	iidIAudioMeterInformatio = ole.NewGUID("{C02216F6-8C67-4B5B-9D00-D008E73E0064}")
)

const (
	uiaControlTypeProperty  = 30003
	uiaAutomationIDProperty = 30011
	uiaValueValueProperty   = 30045
	uiaEditControlType      = 50004
	treeScopeDescendants    = 4
)

// uia reads browser address bars through UI Automation. It must only be
// used from the thread that created it.
type uia struct {
	auto      uintptr
	editCond  uintptr
	ffCond    uintptr
	cache     map[uintptr]uintptr // browser hwnd -> address bar element
	cacheTick map[uintptr]int
	tick      int
}

func newUIA() *uia {
	unk, err := ole.CreateInstance(clsidCUIAutomation, iidIUIAutomation)
	if err != nil {
		return nil
	}
	u := &uia{auto: uintptr(unsafe.Pointer(unk)), cache: map[uintptr]uintptr{}, cacheTick: map[uintptr]int{}}
	v := ole.NewVariant(ole.VT_I4, uiaEditControlType)
	vcall(u.auto, 23, uiaControlTypeProperty, uintptr(unsafe.Pointer(&v)), uintptr(unsafe.Pointer(&u.editCond)))
	bs := ole.SysAllocString("urlbar-input")
	v2 := ole.NewVariant(ole.VT_BSTR, int64(uintptr(unsafe.Pointer(bs))))
	vcall(u.auto, 23, uiaAutomationIDProperty, uintptr(unsafe.Pointer(&v2)), uintptr(unsafe.Pointer(&u.ffCond)))
	ole.SysFreeString(bs)
	return u
}

func (u *uia) value(elem uintptr) (string, bool) {
	var v ole.VARIANT
	ole.VariantInit(&v)
	if hr := vcall(elem, 10, uiaValueValueProperty, uintptr(unsafe.Pointer(&v))); hr != 0 {
		return "", false
	}
	defer ole.VariantClear(&v)
	if v.VT != ole.VT_BSTR {
		return "", true
	}
	return v.ToString(), true
}

// addressBar returns the text of a browser window's address bar.
func (u *uia) addressBar(hwnd uintptr, firefox bool) string {
	if u == nil {
		return ""
	}
	u.tick++
	if elem, ok := u.cache[hwnd]; ok {
		if s, ok := u.value(elem); ok {
			u.cacheTick[hwnd] = u.tick
			return s
		}
		release(elem)
		delete(u.cache, hwnd)
	}
	var root uintptr
	if hr := vcall(u.auto, 6, hwnd, uintptr(unsafe.Pointer(&root))); hr != 0 || root == 0 {
		return ""
	}
	defer release(root)
	cond := u.editCond
	if firefox && u.ffCond != 0 {
		cond = u.ffCond
	}
	var found uintptr
	if hr := vcall(root, 5, treeScopeDescendants, cond, uintptr(unsafe.Pointer(&found))); hr != 0 || found == 0 {
		return ""
	}
	u.cache[hwnd] = found
	u.cacheTick[hwnd] = u.tick
	// Drop elements of windows not seen for a while.
	if len(u.cache) > 16 {
		for h, el := range u.cache {
			if u.tick-u.cacheTick[h] > 600 {
				release(el)
				delete(u.cache, h)
				delete(u.cacheTick, h)
			}
		}
	}
	s, _ := u.value(found)
	return s
}

// dispatchPath walks a chain of IDispatch properties and returns the final
// value as a string, e.g. ("Word.Application", "ActiveDocument", "FullName").
func dispatchPath(progID string, props ...string) string {
	clsid, err := ole.ClassIDFrom(progID)
	if err != nil {
		return ""
	}
	unk, err := ole.GetActiveObject(clsid, ole.IID_IUnknown)
	if err != nil {
		return ""
	}
	defer unk.Release()
	disp, err := unk.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return ""
	}
	cur := disp
	defer func() { cur.Release() }()
	for i, p := range props {
		v, err := oleutil.GetProperty(cur, p)
		if err != nil {
			return ""
		}
		if i == len(props)-1 {
			s := v.ToString()
			v.Clear()
			return s
		}
		next := v.ToIDispatch()
		if next == nil {
			v.Clear()
			return ""
		}
		cur.Release()
		cur = next
	}
	return ""
}

// explorerFolder returns the folder shown by an Explorer window.
func explorerFolder(shell **ole.IDispatch, hwnd uintptr, title string) string {
	if *shell == nil {
		unk, err := oleutil.CreateObject("Shell.Application")
		if err != nil {
			return ""
		}
		d, err := unk.QueryInterface(ole.IID_IDispatch)
		unk.Release()
		if err != nil {
			return ""
		}
		*shell = d
	}
	wv, err := oleutil.CallMethod(*shell, "Windows")
	if err != nil {
		(*shell).Release()
		*shell = nil
		return ""
	}
	wins := wv.ToIDispatch()
	if wins == nil {
		return ""
	}
	defer wins.Release()
	cv, err := oleutil.GetProperty(wins, "Count")
	if err != nil {
		return ""
	}
	n := int(cv.Val)
	best := ""
	for i := 0; i < n; i++ {
		iv, err := oleutil.CallMethod(wins, "Item", i)
		if err != nil {
			continue
		}
		item := iv.ToIDispatch()
		if item == nil {
			continue
		}
		hv, err := oleutil.GetProperty(item, "HWND")
		if err == nil && uintptr(hv.Val) == hwnd {
			loc, _ := oleutil.GetProperty(item, "LocationURL")
			name, _ := oleutil.GetProperty(item, "LocationName")
			path := fileURLToPath(loc.ToString())
			if path != "" && (best == "" || strings.HasPrefix(title, name.ToString())) {
				best = path
			}
		}
		item.Release()
	}
	return best
}

// resolveLink returns the target of a .lnk shortcut.
func resolveLink(lnk string) string {
	unk, err := ole.CreateInstance(clsidShellLink, iidIShellLinkW)
	if err != nil {
		return ""
	}
	sl := uintptr(unsafe.Pointer(unk))
	defer release(sl)
	var pf uintptr
	if hr := vcall(sl, 0, uintptr(unsafe.Pointer(iidIPersistFile)), uintptr(unsafe.Pointer(&pf))); hr != 0 || pf == 0 {
		return ""
	}
	defer release(pf)
	p, _ := syscall.UTF16PtrFromString(lnk)
	if hr := vcall(pf, 5, uintptr(unsafe.Pointer(p)), 0); hr != 0 {
		return ""
	}
	buf := make([]uint16, 1024)
	if hr := vcall(sl, 3, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0, 0); hr != 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

// audioMeter samples the peak level of the default playback device.
type audioMeter struct {
	meter uintptr
	fails int
}

func (a *audioMeter) open() {
	unk, err := ole.CreateInstance(clsidMMDeviceEnumerator, iidIMMDeviceEnumerator)
	if err != nil {
		return
	}
	enum := uintptr(unsafe.Pointer(unk))
	defer release(enum)
	var dev uintptr
	if hr := vcall(enum, 4, 0, 1, uintptr(unsafe.Pointer(&dev))); hr != 0 || dev == 0 {
		return
	}
	defer release(dev)
	vcall(dev, 3, uintptr(unsafe.Pointer(iidIAudioMeterInformatio)), 0x17, 0, uintptr(unsafe.Pointer(&a.meter)))
}

// peak returns the current output peak (0..1), reopening the device when
// the default endpoint changes.
func (a *audioMeter) peak() float32 {
	if a.meter == 0 {
		a.open()
		if a.meter == 0 {
			return 0
		}
	}
	var v float32
	if hr := vcall(a.meter, 3, uintptr(unsafe.Pointer(&v))); hr != 0 {
		release(a.meter)
		a.meter = 0
		return 0
	}
	return v
}

func (a *audioMeter) reset() {
	release(a.meter)
	a.meter = 0
}
