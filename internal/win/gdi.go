//go:build windows

package win

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"unsafe"
)

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [4]uint32
}

type iconInfo struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	HbmMask  uintptr
	HbmColor uintptr
}

type bitmap struct {
	Type       int32
	Width      int32
	Height     int32
	WidthBytes int32
	Planes     uint16
	BitsPixel  uint16
	Bits       uintptr
}

func dibBits(hdc, hbm uintptr, w, h int) []byte {
	bi := bitmapInfo{Header: bitmapInfoHeader{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}}
	buf := make([]byte, w*h*4)
	r, _, _ := pGetDIBits.Call(hdc, hbm, 0, uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bi)), 0)
	if r == 0 {
		return nil
	}
	return buf
}

// IconPNG extracts an executable's icon at the given size as PNG bytes.
func IconPNG(path string, size int) []byte {
	p := utf16Ptr(path)
	var hicon uintptr
	var id uint32
	n, _, _ := pPrivateExtractIconsW.Call(uintptr(unsafe.Pointer(p)), 0, uintptr(size), uintptr(size),
		uintptr(unsafe.Pointer(&hicon)), uintptr(unsafe.Pointer(&id)), 1, 0)
	if n == 0 || n == 0xFFFFFFFF || hicon == 0 {
		return nil
	}
	defer pDestroyIcon.Call(hicon)
	return hiconPNG(hicon)
}

// WindowIconPNG returns the icon a window shows in its title bar and the
// taskbar. Apps built on frameworks (Electron, Java, ...) set it at run time
// while their executable only carries the framework's icon.
func WindowIconPNG(hwnd uintptr) []byte {
	for _, kind := range []uintptr{1, 2, 0} { // ICON_BIG, ICON_SMALL2, ICON_SMALL
		var res uintptr
		r, _, _ := pSendMessageTimeoutW.Call(hwnd, 0x007F, kind, 0, 0x0002, 150, uintptr(unsafe.Pointer(&res))) // WM_GETICON, SMTO_ABORTIFHUNG
		if r != 0 && res != 0 {
			if b := hiconPNG(res); b != nil {
				return b
			}
		}
	}
	for _, idx := range []int32{-14, -34} { // GCLP_HICON, GCLP_HICONSM
		r, _, _ := pGetClassLongPtrW.Call(hwnd, uintptr(idx))
		if r != 0 {
			if b := hiconPNG(r); b != nil {
				return b
			}
		}
	}
	return nil
}

func hiconPNG(hicon uintptr) []byte {
	var ii iconInfo
	if r, _, _ := pGetIconInfo.Call(hicon, uintptr(unsafe.Pointer(&ii))); r == 0 {
		return nil
	}
	if ii.HbmMask != 0 {
		defer pDeleteObject.Call(ii.HbmMask)
	}
	if ii.HbmColor == 0 {
		return nil
	}
	defer pDeleteObject.Call(ii.HbmColor)
	var bm bitmap
	pGetObjectW.Call(ii.HbmColor, unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm)))
	w, h := int(bm.Width), int(bm.Height)
	if w <= 0 || h <= 0 || w > 512 || h > 512 {
		return nil
	}
	hdc, _, _ := pCreateCompatibleDC.Call(0)
	defer pDeleteDC.Call(hdc)
	px := dibBits(hdc, ii.HbmColor, w, h)
	if px == nil {
		return nil
	}
	hasAlpha := false
	for i := 3; i < len(px); i += 4 {
		if px[i] != 0 {
			hasAlpha = true
			break
		}
	}
	var mask []byte
	if !hasAlpha && ii.HbmMask != 0 {
		mask = dibBits(hdc, ii.HbmMask, w, h)
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		b, g, r, a := px[i*4], px[i*4+1], px[i*4+2], px[i*4+3]
		if !hasAlpha {
			a = 255
			if mask != nil && mask[i*4] != 0 {
				a = 0
			}
		}
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = r, g, b, a
	}
	var out bytes.Buffer
	if png.Encode(&out, img) != nil {
		return nil
	}
	return out.Bytes()
}

type monitorInfo struct {
	Size    uint32
	Monitor RECT
	Work    RECT
	Flags   uint32
}

// MonitorRect returns the bounds of the monitor showing hwnd (primary if 0).
func MonitorRect(hwnd uintptr) RECT {
	hm, _, _ := pMonitorFromWindow.Call(hwnd, 1) // MONITOR_DEFAULTTOPRIMARY
	mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if hm == 0 {
		return RECT{0, 0, int32(SystemMetric(0)), int32(SystemMetric(1))}
	}
	pGetMonitorInfoW.Call(hm, uintptr(unsafe.Pointer(&mi)))
	return mi.Monitor
}

// CaptureRect grabs a region of the screen as an RGBA image. It blits into
// a DIB section so the pixels can be read directly.
func CaptureRect(r RECT) (*image.RGBA, error) {
	w, h := int(r.Right-r.Left), int(r.Bottom-r.Top)
	if w <= 0 || h <= 0 {
		return nil, errors.New("empty capture rect")
	}
	screen, _, _ := pGetDC.Call(0)
	if screen == 0 {
		return nil, errors.New("GetDC failed")
	}
	defer pReleaseDC.Call(0, screen)
	mem, _, _ := pCreateCompatibleDC.Call(screen)
	defer pDeleteDC.Call(mem)
	bi := bitmapInfo{Header: bitmapInfoHeader{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}}
	var bits uintptr
	hbm, _, _ := pCreateDIBSection.Call(screen, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hbm == 0 || bits == 0 {
		return nil, errors.New("CreateDIBSection failed")
	}
	defer pDeleteObject.Call(hbm)
	old, _, _ := pSelectObject.Call(mem, hbm)
	ok, _, _ := pBitBlt.Call(mem, 0, 0, uintptr(w), uintptr(h), screen, uintptr(r.Left), uintptr(r.Top), 0x00CC0020)
	pGdiFlush.Call()
	if ok == 0 {
		pSelectObject.Call(mem, old)
		return nil, errors.New("BitBlt failed")
	}
	src := unsafe.Slice((*byte)(unsafe.Pointer(bits)), w*h*4)
	px := make([]byte, len(src))
	for i := 0; i < len(src); i += 4 {
		px[i], px[i+1], px[i+2], px[i+3] = src[i+2], src[i+1], src[i], 255
	}
	pSelectObject.Call(mem, old)
	return &image.RGBA{Pix: px, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}, nil
}
