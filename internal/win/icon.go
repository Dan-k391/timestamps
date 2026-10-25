//go:build windows

package win

import (
	"image"
	"unsafe"
)

var (
	pCreateIconIndirect = user32.NewProc("CreateIconIndirect")
	pCreateBitmap       = gdi32.NewProc("CreateBitmap")
)

// IconFromImage makes an icon (HICON) from an image with alpha; 0 on
// failure. Free it with DestroyIcon.
func IconFromImage(img *image.NRGBA) uintptr {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	bi := bitmapInfo{Header: bitmapInfoHeader{Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}} // top-down
	bi.Header.Size = uint32(unsafe.Sizeof(bi.Header))
	var bits unsafe.Pointer
	dc, _, _ := pGetDC.Call(0)
	color, _, _ := pCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	pReleaseDC.Call(0, dc)
	if color == 0 || bits == nil {
		return 0
	}
	defer pDeleteObject.Call(color)
	px := unsafe.Slice((*byte)(bits), w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := img.NRGBAAt(img.Rect.Min.X+x, img.Rect.Min.Y+y)
			i := (y*w + x) * 4
			px[i], px[i+1], px[i+2], px[i+3] = c.B, c.G, c.R, c.A
		}
	}
	mask, _, _ := pCreateBitmap.Call(uintptr(w), uintptr(h), 1, 1, 0) // unused with alpha, but required
	if mask == 0 {
		return 0
	}
	defer pDeleteObject.Call(mask)
	ii := iconInfo{FIcon: 1, HbmMask: mask, HbmColor: color}
	hicon, _, _ := pCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	return hicon
}

// DestroyIcon frees an icon made by IconFromImage.
func DestroyIcon(hicon uintptr) {
	if hicon != 0 {
		pDestroyIcon.Call(hicon)
	}
}
