package render

import "math"

// RGBA is packed as 0xRRGGBBAA; alpha is always opaque because translucent
// colours are pre-blended over the black background before theming.
type RGBA uint32

func (c RGBA) Channels() (r, g, b uint8) {
	return uint8(c >> 24), uint8(c >> 16), uint8(c >> 8)
}

func pack(r, g, b float64) RGBA {
	q := func(v float64) uint32 { return uint32(math.Round(math.Max(0, math.Min(255, v)))) }
	return RGBA(q(r)<<24 | q(g)<<16 | q(b)<<8 | 0xFF)
}

// theme maps colours designed for a black background onto the active one.
// The white theme inverts lightness only, so the cyan wake stays cyan and
// grey tiles turn darker instead of shifting hue.
type theme struct {
	white bool
	cache map[RGBA]RGBA
}

func (t *theme) set(white bool) {
	if t.white != white || t.cache == nil {
		t.white = white
		t.cache = make(map[RGBA]RGBA)
	}
}

func (t *theme) rgb(r, g, b float64) RGBA {
	c := pack(r, g, b)
	if !t.white {
		return c
	}
	if v, ok := t.cache[c]; ok {
		return v
	}
	v := invertLightness(c)
	t.cache[c] = v
	return v
}

// over pre-blends a translucent colour onto black.
func (t *theme) over(r, g, b, a float64) RGBA {
	return t.rgb(r*a, g*a, b*a)
}

func invertLightness(c RGBA) RGBA {
	r8, g8, b8 := c.Channels()
	r, g, b := float64(r8)/255, float64(g8)/255, float64(b8)/255
	h, s, l := toHSL(r, g, b)
	r, g, b = fromHSL(h, s, 1-l)
	return pack(r*255, g*255, b*255)
}

func toHSL(r, g, b float64) (h, s, l float64) {
	mx := max(r, g, b)
	mn := min(r, g, b)
	l = (mx + mn) / 2
	if mx == mn {
		return 0, 0, l
	}
	d := mx - mn
	if l > 0.5 {
		s = d / (2 - mx - mn)
	} else {
		s = d / (mx + mn)
	}
	switch mx {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h / 6, s, l
}

func fromHSL(h, s, l float64) (r, g, b float64) {
	if s == 0 {
		return l, l, l
	}
	q := l * (1 + s)
	if l >= 0.5 {
		q = l + s - l*s
	}
	p := 2*l - q
	return hue(p, q, h+1.0/3), hue(p, q, h), hue(p, q, h-1.0/3)
}

func hue(p, q, t float64) float64 {
	if t < 0 {
		t++
	}
	if t > 1 {
		t--
	}
	switch {
	case t < 1.0/6:
		return p + (q-p)*6*t
	case t < 0.5:
		return q
	case t < 2.0/3:
		return p + (q-p)*(2.0/3-t)*6
	}
	return p
}

func parseHex(s string) (r, g, b float64) {
	var v uint32
	for _, ch := range s[1:] {
		v <<= 4
		switch {
		case ch >= '0' && ch <= '9':
			v |= uint32(ch - '0')
		case ch >= 'a' && ch <= 'f':
			v |= uint32(ch-'a') + 10
		}
	}
	return float64(v >> 16 & 0xFF), float64(v >> 8 & 0xFF), float64(v & 0xFF)
}
