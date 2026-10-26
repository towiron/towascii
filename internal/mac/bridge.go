// Package mac hosts the ocean in desktop-level Cocoa windows, one per screen,
// and exposes a status bar menu. Cocoa requires Run to be called on the main
// OS thread, so the caller must lock it in init.
package mac

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework CoreText -framework QuartzCore
#include "bridge.h"
*/
import "C"

import (
	"log/slog"
	"unsafe"

	"github.com/towiron/towascii/internal/render"
)

type Action int

const (
	ActionBlack Action = C.AW_ACTION_BLACK
	ActionWhite Action = C.AW_ACTION_WHITE
	ActionPause Action = C.AW_ACTION_PAUSE
	ActionQuit  Action = C.AW_ACTION_QUIT
)

// Handler receives callbacks on the main thread.
type Handler interface {
	Resize(screen uint32, width, height float64) render.Layout
	// Frame advances the screen's scene by dt seconds; ok=false means nothing to draw.
	Frame(screen uint32, dt float64) (items []render.Item, ok bool)
	ScreenRemoved(screen uint32)
	Action(a Action)
}

var handler Handler

func Run(h Handler, white, paused bool) {
	handler = h
	C.awRun(cbool(white), cbool(paused))
}

func Apply(white, paused bool) {
	C.awApply(cbool(white), cbool(paused))
}

func Quit() {
	C.awQuit()
}

// QuitAsync asks the Cocoa main loop to terminate; safe from any goroutine.
func QuitAsync() {
	C.awQuitAsync()
}

// RestoreWallpaper must run on the main thread.
func RestoreWallpaper() {
	C.awRestoreWallpaper()
}

func cbool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

// A panic must not unwind through Cocoa frames, so every callback swallows it.
func recoverCallback(name string) {
	if r := recover(); r != nil {
		slog.Error("callback panicked", "callback", name, "panic", r)
	}
}

//export goResize
func goResize(screen C.uint32_t, width, height C.double, out *C.AWLayout) {
	defer recoverCallback("resize")
	l := handler.Resize(uint32(screen), float64(width), float64(height))
	out.cellW = C.double(l.CellW)
	out.cellH = C.double(l.CellH)
	out.dotSize = C.double(l.DotSize)
	out.cols = C.int(l.Cols)
	out.rows = C.int(l.Rows)
}

// The item buffer is owned by C; Go only writes into it for the duration of
// the call, which keeps the cgo pointer rules trivially satisfied.
//
//export goTick
func goTick(screen C.uint32_t, buf *C.AWItem, capacity C.int, dt C.double) (count C.int) {
	count = -1
	defer recoverCallback("tick")
	items, ok := handler.Frame(uint32(screen), float64(dt))
	if !ok {
		return -1
	}
	n := min(len(items), int(capacity))
	dst := unsafe.Slice(buf, int(capacity))
	for i, it := range items[:n] {
		dst[i] = C.AWItem{
			rgba:  C.uint32_t(it.RGBA),
			col:   C.uint16_t(it.Col),
			row:   C.uint16_t(it.Row),
			glyph: C.uint8_t(it.Glyph),
			kind:  C.uint8_t(it.Kind),
		}
	}
	return C.int(n)
}

//export goScreenRemoved
func goScreenRemoved(screen C.uint32_t) {
	defer recoverCallback("screen removed")
	handler.ScreenRemoved(uint32(screen))
}

//export goMenuAction
func goMenuAction(id C.int) {
	defer recoverCallback("menu")
	handler.Action(Action(id))
}
