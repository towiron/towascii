package render

import (
	"fmt"
	"strings"
)

type dumpCell struct {
	glyph  byte
	fg, bg RGBA
	hasBG  bool
}

func flatten(items []Item, l Layout) []dumpCell {
	cells := make([]dumpCell, l.Cols*l.Rows)
	for i := range cells {
		cells[i].glyph = ' '
	}
	for _, it := range items {
		c := &cells[int(it.Row)*l.Cols+int(it.Col)]
		switch it.Kind {
		case KindTile:
			c.bg, c.hasBG = it.RGBA, true
		case KindDot:
			c.glyph, c.fg = '.', it.RGBA
		case KindGlyph:
			c.glyph, c.fg = it.Glyph, it.RGBA
		}
	}
	return cells
}

// Text renders a draw list as plain characters, one line per row.
func Text(items []Item, l Layout) string {
	cells := flatten(items, l)
	var sb strings.Builder
	for row := range l.Rows {
		line := make([]byte, l.Cols)
		for c := range l.Cols {
			line[c] = cells[row*l.Cols+c].glyph
		}
		sb.WriteString(strings.TrimRight(string(line), " "))
		sb.WriteByte('\n')
	}
	return sb.String()
}

// ANSI renders a draw list with 24-bit terminal colours on the given background.
func ANSI(items []Item, l Layout, bg RGBA) string {
	cells := flatten(items, l)
	br, bgc, bb := bg.Channels()
	var sb strings.Builder
	for row := range l.Rows {
		for c := range l.Cols {
			cell := cells[row*l.Cols+c]
			back := bg
			if cell.hasBG {
				back = cell.bg
			}
			r, g, b := back.Channels()
			fr, fg, fb := cell.fg.Channels()
			fmt.Fprintf(&sb, "\x1b[48;2;%d;%d;%dm\x1b[38;2;%d;%d;%dm%c", r, g, b, fr, fg, fb, cell.glyph)
		}
		fmt.Fprintf(&sb, "\x1b[48;2;%d;%d;%dm\x1b[0m\n", br, bgc, bb)
	}
	return sb.String()
}
