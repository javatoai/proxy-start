//go:build windows

package windows

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

const iconSize = 32

const (
	shgfiIcon       = 0x000000100
	shgfiLargeIcon  = 0x000000000
	dibRGBColors    = 0
	biRGB           = 0
	diNormal        = 0x0003
	transparentByte = 0
)

var (
	shell32            = syscall.NewLazyDLL("shell32.dll")
	user32             = syscall.NewLazyDLL("user32.dll")
	gdi32              = syscall.NewLazyDLL("gdi32.dll")
	extractIconExW     = shell32.NewProc("ExtractIconExW")
	shGetFileInfoW     = shell32.NewProc("SHGetFileInfoW")
	destroyIcon        = user32.NewProc("DestroyIcon")
	drawIconEx         = user32.NewProc("DrawIconEx")
	createCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	deleteDC           = gdi32.NewProc("DeleteDC")
	createDIBSection   = gdi32.NewProc("CreateDIBSection")
	selectObject       = gdi32.NewProc("SelectObject")
	deleteGDIObject    = gdi32.NewProc("DeleteObject")
)

// IconProvider extracts shell icons without launching the target executable.
// The cache is invalidated when the executable's modification time changes.
type IconProvider struct {
	mu    sync.Mutex
	cache map[string]iconCacheEntry
}

type iconCacheEntry struct {
	modified int64
	size     int64
	data     []byte
}

var _ application.AppIconProvider = (*IconProvider)(nil)

// NewIconProvider constructs a process-safe, in-memory icon provider.
func NewIconProvider() *IconProvider {
	return &IconProvider{cache: make(map[string]iconCacheEntry)}
}

func (p *IconProvider) Icon(ctx context.Context, identity domain.AppIdentity) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path := canonicalPath(identity.Executable)
	if path == "" {
		return nil, errors.New("应用缺少可执行路径，无法读取图标")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("无法读取应用图标: %w", err)
	}
	if info.IsDir() {
		return nil, errors.New("应用图标路径不能是目录")
	}
	key := strings.ToLower(filepath.Clean(path))
	modified := info.ModTime().UnixNano()
	p.mu.Lock()
	if entry, ok := p.cache[key]; ok && entry.modified == modified && entry.size == info.Size() {
		data := append([]byte(nil), entry.data...)
		p.mu.Unlock()
		return data, nil
	}
	p.mu.Unlock()

	data, err := extractExecutableIcon(path)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.cache[key] = iconCacheEntry{modified: modified, size: info.Size(), data: append([]byte(nil), data...)}
	p.mu.Unlock()
	return data, nil
}

type iconInfo struct {
	hIcon        uintptr
	iIcon        int32
	dwAttributes uint32
	displayName  [260]uint16
	typeName     [80]uint16
}

type bitmapInfoHeader struct {
	size        uint32
	width       int32
	height      int32
	planes      uint16
	bitCount    uint16
	compression uint32
	sizeImage   uint32
	xPixelsPerM int32
	yPixelsPerM int32
	used        uint32
	important   uint32
}

type bitmapInfo struct {
	header bitmapInfoHeader
	colors [1]rgbQuad
}

type rgbQuad struct {
	blue     byte
	green    byte
	red      byte
	reserved byte
}

func extractExecutableIcon(path string) ([]byte, error) {
	windowsPath, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("无效的图标路径: %w", err)
	}
	var largeIcon uintptr
	count, _, _ := extractIconExW.Call(uintptr(unsafe.Pointer(windowsPath)), 0, uintptr(unsafe.Pointer(&largeIcon)), 0, 1)
	if count == 0 || largeIcon == 0 {
		var info iconInfo
		flags := uintptr(shgfiIcon | shgfiLargeIcon)
		if result, _, _ := shGetFileInfoW.Call(uintptr(unsafe.Pointer(windowsPath)), 0, uintptr(unsafe.Pointer(&info)), uintptr(unsafe.Sizeof(info)), flags); result != 0 {
			largeIcon = info.hIcon
		}
	}
	if largeIcon == 0 {
		return nil, fmt.Errorf("应用没有可提取的图标: %s", path)
	}
	defer destroyIcon.Call(largeIcon)
	return renderIconPNG(largeIcon)
}

func renderIconPNG(icon uintptr) ([]byte, error) {
	var pixels unsafe.Pointer
	bitmapDescription := bitmapInfo{header: bitmapInfoHeader{
		size:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		width:       iconSize,
		height:      -iconSize, // top-down DIB: first row is the top row
		planes:      1,
		bitCount:    32,
		compression: biRGB,
	}}
	bitmap, _, bitmapErr := createDIBSection.Call(0, uintptr(unsafe.Pointer(&bitmapDescription)), dibRGBColors, uintptr(unsafe.Pointer(&pixels)), 0, 0)
	if bitmap == 0 || pixels == nil {
		return nil, fmt.Errorf("创建图标位图失败: %v", bitmapErr)
	}
	defer deleteGDIObject.Call(bitmap)
	dc, _, dcErr := createCompatibleDC.Call(0)
	if dc == 0 {
		return nil, fmt.Errorf("创建图标绘制上下文失败: %v", dcErr)
	}
	defer deleteDC.Call(dc)
	previous, _, _ := selectObject.Call(dc, bitmap)
	if previous == 0 {
		return nil, errors.New("选择图标位图失败")
	}
	defer selectObject.Call(dc, previous)
	// CreateDIBSection memory is zero-initialized; retaining the zero fill
	// provides a transparent canvas for icons that carry an alpha channel.
	if result, _, drawErr := drawIconEx.Call(dc, 0, 0, icon, iconSize, iconSize, 0, 0, diNormal); result == 0 {
		return nil, fmt.Errorf("绘制图标失败: %v", drawErr)
	}

	rgba := image.NewRGBA(image.Rect(0, 0, iconSize, iconSize))
	raw := unsafe.Slice((*byte)(pixels), iconSize*iconSize*4)
	allAlphaZero := true
	for i := 0; i < iconSize*iconSize; i++ {
		if raw[i*4+3] != transparentByte {
			allAlphaZero = false
			break
		}
	}
	for y := 0; y < iconSize; y++ {
		for x := 0; x < iconSize; x++ {
			i := (y*iconSize + x) * 4
			b, g, r, a := raw[i], raw[i+1], raw[i+2], raw[i+3]
			if allAlphaZero && (r != 0 || g != 0 || b != 0) {
				a = 0xff
			} else if a == 0 && (r != 0 || g != 0 || b != 0) {
				// Some legacy shell icons render opaque pixels with an unset
				// alpha byte. Preserve their visible colors instead of returning
				// translucent black tiles to the UI.
				a = 0xff
			}
			rgba.SetRGBA(x, y, colorRGBA(r, g, b, a))
		}
	}
	var output bytes.Buffer
	if err := png.Encode(&output, rgba); err != nil {
		return nil, fmt.Errorf("编码应用图标失败: %w", err)
	}
	runtime.KeepAlive(pixels)
	return output.Bytes(), nil
}

// colorRGBA keeps image/color out of the platform API surface while allowing
// direct construction of the standard library's RGBA pixel type.
func colorRGBA(r, g, b, a byte) color.RGBA {
	return color.RGBA{R: r, G: g, B: b, A: a}
}
