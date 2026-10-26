package render

import (
	"math"

	"ascii-wallpaper/internal/ocean"
)

// Shape is the top-down body outline and fin layout of one kind. All values
// are fractions of body length; s runs from 0 (snout) to 1 (tail).
type Shape struct {
	HalfWidth func(s float64) float64
	// Pectoral fin: leading root, trailing root, tip position along the body, and reach.
	Pectoral [4]float64
	// Falcate fins get a notched (sickle) trailing edge.
	Falcate bool
	// Pelvic fins: leading root, trailing root, tip, reach. Zero reach means none.
	Pelvic [4]float64
	// Caudal is the forked tail fin's reach, or 0 when the tail is a ribbon.
	Caudal       float64
	CaudalRoot   [2]float64
	Dorsal       [2]float64
	SecondDorsal [2]float64
	// Head is the hammerheads' cephalofoil; nil for everyone else.
	Head *Head
	// Eye is nil when the eyes sit somewhere else (hammerhead: on the cephalofoil tips).
	Eye   *[2]float64
	Gills [2]float64
	// Tint multiplies the skin colour (r, g, b).
	Tint [3]float64
	// BodyShade scales how much light the skin reflects; orcas are near black.
	BodyShade float64
	// FinShade, when set, overrides fin brightness (a humpback's pale flippers).
	FinShade float64
	// Wave scales the side-to-side body wave; whales swim with vertical strokes.
	Wave float64

	widths [halfWidthSteps + 1]float64
}

const halfWidthSteps = 512

// width is HalfWidth sampled from a lookup table; the outline functions use
// math.Pow and would otherwise dominate rasterisation.
func (sh *Shape) width(s float64) float64 {
	f := ocean.Clamp(s, 0, 1) * halfWidthSteps
	i := int(f)
	if i >= halfWidthSteps {
		return sh.widths[halfWidthSteps]
	}
	t := f - float64(i)
	return sh.widths[i]*(1-t) + sh.widths[i+1]*t
}

// Head describes a cephalofoil: half-span, how far its tips sweep back, its
// depth, and the notch in the middle of the front edge.
type Head struct {
	Span, Sweep, Depth, Notch float64
}

func classicHalfWidth(s float64) float64 {
	switch {
	case s < 0.13:
		return 0.09 * math.Pow(math.Sin((s/0.13)*math.Pi/2), 0.95)
	case s < 0.3:
		return 0.09 + 0.016*ocean.Smoothstep(0.13, 0.3, s)
	case s < 0.84:
		return 0.019 + 0.087*math.Pow(1-(s-0.3)/0.54, 1.25)
	}
	k := (s - 0.84) / 0.16
	return 0.019*(1-k) + 0.012*math.Sin(k*math.Pi)
}

func eye(a, b float64) *[2]float64 {
	return &[2]float64{a, b}
}

var defaultCaudalRoot = [2]float64{0.86, 0.95}

var Shapes = map[ocean.Kind]*Shape{
	ocean.Classic: {
		HalfWidth:    classicHalfWidth,
		Pectoral:     [4]float64{0.15, 0.43, 0.55, 0.26},
		Falcate:      true,
		Pelvic:       [4]float64{0.57, 0.65, 0.69, 0.085},
		Caudal:       0.085,
		Dorsal:       [2]float64{0.33, 0.47},
		SecondDorsal: [2]float64{0.71, 0.76},
		Eye:          eye(0.1, 0.14),
		Gills:        [2]float64{0.19, 0.29},
		Tint:         [3]float64{0.92, 0.97, 1.05},
	},
	// Slim neck under a wide, swept-back cephalofoil, tall first dorsal, long
	// upper tail lobe. Bronze-olive grey.
	ocean.Hammerhead: {
		HalfWidth: func(s float64) float64 {
			if s < 0.06 {
				return 0.045
			}
			if s < 0.3 {
				return 0.045 + 0.061*ocean.Smoothstep(0.06, 0.3, s)
			}
			return classicHalfWidth(s) * 0.92
		},
		Head:         &Head{Span: 0.17, Sweep: 0.035, Depth: 0.09},
		Pectoral:     [4]float64{0.17, 0.4, 0.5, 0.22},
		Falcate:      true,
		Pelvic:       [4]float64{0.56, 0.63, 0.67, 0.075},
		Caudal:       0.11,
		Dorsal:       [2]float64{0.28, 0.44},
		SecondDorsal: [2]float64{0.7, 0.74},
		Gills:        [2]float64{0.16, 0.26},
		Tint:         [3]float64{0.96, 0.98, 0.9},
	},
	// Blunt, square snout on a heavy body; barred back. Warm grey-brown.
	ocean.Tiger: {
		HalfWidth: func(s float64) float64 {
			switch {
			case s < 0.13:
				return 0.1 * math.Pow(math.Sin((s/0.13)*math.Pi/2), 0.45)
			case s < 0.3:
				return 0.1 + 0.014*ocean.Smoothstep(0.13, 0.3, s)
			case s < 0.84:
				return 0.019 + 0.095*math.Pow(1-(s-0.3)/0.54, 1.2)
			}
			k := (s - 0.84) / 0.16
			return 0.019*(1-k) + 0.012*math.Sin(k*math.Pi)
		},
		Pectoral:     [4]float64{0.15, 0.42, 0.53, 0.25},
		Falcate:      true,
		Pelvic:       [4]float64{0.57, 0.65, 0.69, 0.085},
		Caudal:       0.095,
		Dorsal:       [2]float64{0.33, 0.46},
		SecondDorsal: [2]float64{0.71, 0.75},
		Eye:          eye(0.08, 0.12),
		Gills:        [2]float64{0.19, 0.28},
		Tint:         [3]float64{1.02, 0.95, 0.84},
	},
	// Short rounded head, body done by half-length, then a long ribbon of a
	// tail fin. Broad rounded pectorals, spotted sandy skin.
	ocean.Zebra: {
		HalfWidth: func(s float64) float64 {
			switch {
			case s < 0.1:
				return 0.075 * math.Pow(math.Sin((s/0.1)*math.Pi/2), 0.7)
			case s < 0.28:
				return 0.075 + 0.025*ocean.Smoothstep(0.1, 0.28, s)
			case s < 0.55:
				return 0.022 + 0.078*math.Pow(1-(s-0.28)/0.27, 1.1)
			}
			k := (s - 0.55) / 0.45
			return 0.022*(1-k*k) + 0.012*math.Sin(k*math.Pi)
		},
		Pectoral:     [4]float64{0.12, 0.3, 0.36, 0.19},
		Pelvic:       [4]float64{0.42, 0.48, 0.51, 0.065},
		Dorsal:       [2]float64{0.36, 0.44},
		SecondDorsal: [2]float64{0.49, 0.53},
		Eye:          eye(0.06, 0.09),
		Gills:        [2]float64{0.13, 0.2},
		Tint:         [3]float64{1.08, 0.97, 0.74},
	},
	// A broader, nearly straight-edged cephalofoil with a central notch, a
	// towering sickle first dorsal, and cooler slate skin.
	ocean.GreatHammer: {
		HalfWidth: func(s float64) float64 {
			if s < 0.05 {
				return 0.04
			}
			if s < 0.3 {
				return 0.04 + 0.07*ocean.Smoothstep(0.05, 0.3, s)
			}
			return classicHalfWidth(s) * 0.95
		},
		Head:         &Head{Span: 0.21, Sweep: 0.006, Depth: 0.085, Notch: 0.014},
		Pectoral:     [4]float64{0.16, 0.38, 0.49, 0.24},
		Falcate:      true,
		Pelvic:       [4]float64{0.55, 0.62, 0.67, 0.085},
		Caudal:       0.13,
		Dorsal:       [2]float64{0.24, 0.45},
		SecondDorsal: [2]float64{0.68, 0.74},
		Gills:        [2]float64{0.15, 0.25},
		Tint:         [3]float64{0.86, 0.93, 1.02},
	},
	// Long conical snout, a torpedo body that narrows hard to the tail stalk,
	// a tall crescent tail. Metallic indigo back.
	ocean.Mako: {
		HalfWidth: func(s float64) float64 {
			switch {
			case s < 0.17:
				return 0.088 * math.Pow(math.Sin((s/0.17)*math.Pi/2), 1.5)
			case s < 0.32:
				return 0.088 + 0.012*ocean.Smoothstep(0.17, 0.32, s)
			case s < 0.86:
				return 0.014 + 0.086*math.Pow(1-(s-0.32)/0.54, 1.45)
			}
			k := (s - 0.86) / 0.14
			return 0.014*(1-k) + 0.01*math.Sin(k*math.Pi)
		},
		Pectoral:     [4]float64{0.18, 0.4, 0.5, 0.21},
		Falcate:      true,
		Pelvic:       [4]float64{0.58, 0.64, 0.68, 0.06},
		Caudal:       0.13,
		Dorsal:       [2]float64{0.34, 0.45},
		SecondDorsal: [2]float64{0.76, 0.79},
		Eye:          eye(0.11, 0.15),
		Gills:        [2]float64{0.2, 0.3},
		Tint:         [3]float64{0.78, 0.9, 1.2},
	},
	// Compact body over the front half, then the upper tail lobe as a whip as
	// long as the shark itself. Big eyes, long sickle pectorals.
	ocean.Thresher: {
		HalfWidth: func(s float64) float64 {
			switch {
			case s < 0.08:
				return 0.07 * math.Pow(math.Sin((s/0.08)*math.Pi/2), 0.75)
			case s < 0.2:
				return 0.07 + 0.02*ocean.Smoothstep(0.08, 0.2, s)
			case s < 0.48:
				return 0.012 + 0.078*math.Pow(1-(s-0.2)/0.28, 1.3)
			}
			// The whip: thin, with a slight sickle swell before it tapers out.
			k := (s - 0.48) / 0.52
			return 0.012*(1-k) + 0.006*math.Sin(k*math.Pi*0.9) + 0.002
		},
		Pectoral:     [4]float64{0.12, 0.27, 0.36, 0.2},
		Falcate:      true,
		Pelvic:       [4]float64{0.37, 0.41, 0.44, 0.045},
		Dorsal:       [2]float64{0.24, 0.32},
		SecondDorsal: [2]float64{0.45, 0.47},
		Eye:          eye(0.05, 0.1),
		Gills:        [2]float64{0.11, 0.18},
		Tint:         [3]float64{1.0, 0.9, 1.08},
	},
	// Blunt, broad head, heavy flattened body with both dorsals set far back,
	// rounded paddle pectorals, long low tail. Tawny brown.
	ocean.Nurse: {
		HalfWidth: func(s float64) float64 {
			switch {
			case s < 0.08:
				return 0.085 * math.Pow(math.Sin((s/0.08)*math.Pi/2), 0.4)
			case s < 0.3:
				return 0.085 + 0.02*ocean.Smoothstep(0.08, 0.3, s)
			case s < 0.72:
				return 0.022 + 0.083*math.Pow(1-(s-0.3)/0.42, 1.1)
			}
			k := (s - 0.72) / 0.28
			return 0.022*(1-k*k) + 0.014*math.Sin(k*math.Pi)
		},
		Pectoral:     [4]float64{0.16, 0.34, 0.4, 0.17},
		Pelvic:       [4]float64{0.45, 0.52, 0.55, 0.075},
		Dorsal:       [2]float64{0.5, 0.58},
		SecondDorsal: [2]float64{0.63, 0.69},
		Eye:          eye(0.05, 0.075),
		Gills:        [2]float64{0.14, 0.22},
		Tint:         [3]float64{1.1, 0.96, 0.76},
	},
	// Broad rounded head, a long stout body, wing-like pale flippers a third
	// of the body long, and wide flukes. Dark blue-grey.
	ocean.Humpback: {
		HalfWidth: func(s float64) float64 {
			switch {
			case s < 0.12:
				return 0.1 * math.Pow(math.Sin((s/0.12)*math.Pi/2), 0.6)
			case s < 0.35:
				return 0.1 + 0.02*ocean.Smoothstep(0.12, 0.35, s)
			case s < 0.86:
				return 0.016 + 0.104*math.Pow(1-(s-0.35)/0.51, 1.4)
			}
			k := (s - 0.86) / 0.14
			return 0.016*(1-k) + 0.008*math.Sin(k*math.Pi)
		},
		Pectoral:   [4]float64{0.24, 0.31, 0.52, 0.34},
		Caudal:     0.17,
		CaudalRoot: [2]float64{0.88, 0.97},
		Dorsal:     [2]float64{0.62, 0.66},
		Eye:        eye(0.14, 0.17),
		Tint:       [3]float64{0.84, 0.92, 1.08},
		BodyShade:  0.8,
		FinShade:   0.85,
		Wave:       0.35,
	},
	// Blunt head, bulky body, tall dorsal, paddle flippers; near-black skin
	// with a white eye patch and a grey saddle behind the dorsal.
	ocean.Orca: {
		HalfWidth: func(s float64) float64 {
			switch {
			case s < 0.1:
				return 0.1 * math.Pow(math.Sin((s/0.1)*math.Pi/2), 0.55)
			case s < 0.32:
				return 0.1 + 0.018*ocean.Smoothstep(0.1, 0.32, s)
			case s < 0.85:
				return 0.018 + 0.1*math.Pow(1-(s-0.32)/0.53, 1.3)
			}
			k := (s - 0.85) / 0.15
			return 0.018*(1-k) + 0.01*math.Sin(k*math.Pi)
		},
		Pectoral:   [4]float64{0.2, 0.3, 0.37, 0.15},
		Caudal:     0.14,
		CaudalRoot: [2]float64{0.87, 0.96},
		Dorsal:     [2]float64{0.38, 0.5},
		Eye:        eye(0.1, 0.13),
		Tint:       [3]float64{0.92, 0.96, 1.05},
		BodyShade:  0.4,
		Wave:       0.45,
	},
}

func init() {
	for _, sh := range Shapes {
		if sh.CaudalRoot == [2]float64{} {
			sh.CaudalRoot = defaultCaudalRoot
		}
		if sh.BodyShade == 0 {
			sh.BodyShade = 1
		}
		if sh.Wave == 0 {
			sh.Wave = 1
		}
		for i := range sh.widths {
			sh.widths[i] = sh.HalfWidth(float64(i) / halfWidthSteps)
		}
	}
}

func inTriangle(px, py, ax, ay, bx, by, cx, cy float64) bool {
	d1 := (px-bx)*(ay-by) - (ax-bx)*(py-by)
	d2 := (px-cx)*(by-cy) - (bx-cx)*(py-cy)
	d3 := (px-ax)*(cy-ay) - (cx-ax)*(py-ay)
	neg := d1 < 0 || d2 < 0 || d3 < 0
	pos := d1 > 0 || d2 > 0 || d3 > 0
	return !(neg && pos)
}
