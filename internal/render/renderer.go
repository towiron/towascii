// Package render draws an ocean.World as a grid of ASCII cells: a wake field
// that diffuses where things swim and lights cells cyan, fish as one dash per
// cell that glints on hard turns, and sharks and whales rasterised cell by cell
// with supersampled edges — grey tiles shaded by the light, dense glyphs and
// shimmering accent colours.
//
// Ported from stackedlol/ascii-sharks (MIT), see THIRD_PARTY_LICENSES.md.
package render

import (
	"cmp"
	"math"
	"slices"

	"ascii-wallpaper/internal/ocean"
)

type ItemKind uint8

const (
	KindTile ItemKind = iota
	KindDot
	KindGlyph
)

// Item is one draw operation on a cell. Items come out grouped by phase and
// sorted by kind and colour inside a phase, so the host can batch runs.
type Item struct {
	RGBA     RGBA
	Col, Row uint16
	Glyph    byte
	Kind     ItemKind
}

type Layout struct {
	CellW, CellH float64
	DotSize      float64
	Cols, Rows   int
}

func LayoutFor(width, height float64) Layout {
	cw := ocean.Clamp(math.Round(width/124), 8, 13)
	ch := math.Round(cw * 1.45)
	return Layout{
		CellW:   cw,
		CellH:   ch,
		DotSize: math.Max(2, math.Round(cw*0.19)) + 1,
		Cols:    int(math.Ceil(width / cw)),
		Rows:    int(math.Ceil(height / ch)),
	}
}

var fishAlphas = [...]float64{0.28, 0.42, 0.58, 0.74, 0.9}

const wakeLevels = 6

var (
	wakeRGB    = [3]float64{110, 230, 255}
	eyeRGB     = rgbHex("#9aa0ad")
	litInkRGB  = rgbHex("#0e0f13")
	darkInkRGB = rgbHex("#1b1d24")
)

func rgbHex(h string) [3]float64 {
	r, g, b := parseHex(h)
	return [3]float64{r, g, b}
}

var accents = func() [][3]float64 {
	var out [][3]float64
	for _, h := range []string{"#e0892c", "#e03a32", "#b44ae0", "#33c8cc", "#3cb54a", "#3150e8", "#d9c14a"} {
		out = append(out, rgbHex(h))
	}
	return out
}()

// Sparse glyphs on bright skin, dense glyphs in shadow, so the texture
// reinforces the shading instead of fighting it.
const (
	skinRamp    = ":;t%SX8#@"
	finGlyphs   = "%;tS:"
	ridgeGlyphs = "@8%"
	edgeHeavy   = ":;"
	edgeLight   = ".',`"
	markGlyphs  = "#%@"
)

const (
	flagBody uint8 = iota + 1
	flagEdge
	flagFin
	flagRidge
	flagEye
	flagMark
)

// Sub-cell sample offsets for anti-aliased silhouettes.
var subsamples = [...]float64{0.25, 0.3, 0.75, 0.3, 0.25, 0.72, 0.75, 0.72}

const (
	phaseDots = iota
	phaseHeat
	phaseFish
	phaseTiles
	phaseGlyphs
	phaseCount
)

var kindIndex = func() map[ocean.Kind]uint8 {
	m := map[ocean.Kind]uint8{}
	for i, k := range ocean.AllKinds {
		m[k] = uint8(i)
	}
	return m
}()

// segment caches one render-spine segment so samples skip the sqrt and division.
type segment struct {
	ax, ay, sx, sy  float64
	invLen2, invLen float64
	length, arc     float64
}

type Renderer struct {
	Layout
	theme theme

	energy, energyTmp, heat []float64

	sharkFlag  []uint8
	sharkShade []float64
	sharkKey   []uint32
	sharkDepth []float64
	sharkKind  []uint8

	// Nearest render-spine segment per cell for the shark being rasterised.
	nearD2 []float64
	nearK  []int8

	// fishStamp marks cells holding a fish this frame; dots skip them.
	fishStamp []uint32
	frameID   uint32

	spine       [ocean.SharkNodes * 2]float64
	arc         [ocean.SharkNodes]float64
	segs        [ocean.SharkNodes - 1]segment
	arcTotal    float64
	shape       *Shape
	kind        ocean.Kind
	sharkID     int
	finTipLeft  float64
	finTipRight float64

	// Scratch outputs of sampleShark, to avoid allocating per sample.
	hitFlag  uint8
	hitShade float64
	hitS     float64
	hitV     float64
	hitDist  float64
	hitK     int
	// hitMargin is how far inside the body outline the sample sits, or -1.
	hitMargin float64

	phases [phaseCount][]Item
	items  []Item
}

func NewRenderer(white bool) *Renderer {
	r := &Renderer{}
	r.theme.set(white)
	return r
}

func (r *Renderer) SetWhite(white bool) {
	r.theme.set(white)
}

func (r *Renderer) Resize(width, height float64) Layout {
	l := LayoutFor(width, height)
	if l.Cols != r.Cols || l.Rows != r.Rows {
		n := l.Cols * l.Rows
		r.energy = make([]float64, n)
		r.energyTmp = make([]float64, n)
		r.heat = make([]float64, n)
		r.sharkFlag = make([]uint8, n)
		r.sharkShade = make([]float64, n)
		r.sharkKey = make([]uint32, n)
		r.sharkDepth = make([]float64, n)
		r.sharkKind = make([]uint8, n)
		r.nearD2 = make([]float64, n)
		r.nearK = make([]int8, n)
		r.fishStamp = make([]uint32, n)
	}
	r.Layout = l
	return l
}

// Render advances the visual fields by dt and returns the frame's draw list.
// The slice is reused by the next call.
func (r *Renderer) Render(w *ocean.World, dt float64) []Item {
	if r.Cols == 0 {
		return nil
	}
	t := w.Time
	r.consumeEvents(w)
	r.rasterizeSharks(w.Sharks, t)
	r.depositWake(w)
	r.diffuse(dt)

	for p := range r.phases {
		r.phases[p] = r.phases[p][:0]
	}
	r.markFish(w)
	r.drawDots()
	r.drawHeat()
	r.drawSharks(t)

	r.items = r.items[:0]
	for p := range r.phases {
		slices.SortFunc(r.phases[p], func(a, b Item) int {
			return cmp.Or(cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.RGBA, b.RGBA))
		})
		r.items = append(r.items, r.phases[p]...)
	}
	return r.items
}

func (r *Renderer) emit(phase int, i int, kind ItemKind, glyph byte, c RGBA) {
	r.phases[phase] = append(r.phases[phase], Item{
		RGBA:  c,
		Col:   uint16(i % r.Cols),
		Row:   uint16(i / r.Cols),
		Glyph: glyph,
		Kind:  kind,
	})
}

func (r *Renderer) cellIndex(x, y float64) int {
	c := int(math.Floor(x / r.CellW))
	row := int(math.Floor(y / r.CellH))
	if c < 0 || row < 0 || c >= r.Cols || row >= r.Rows {
		return -1
	}
	return row*r.Cols + c
}

func (r *Renderer) stamp(field []float64, x, y float64, radius int, amount float64) {
	c0 := int(math.Floor(x / r.CellW))
	r0 := int(math.Floor(y / r.CellH))
	for row := r0 - radius; row <= r0+radius; row++ {
		for c := c0 - radius; c <= c0+radius; c++ {
			if c < 0 || row < 0 || c >= r.Cols || row >= r.Rows {
				continue
			}
			d := math.Hypot(float64(c-c0), float64(row-r0)*1.4)
			if d > float64(radius)+0.5 {
				continue
			}
			i := row*r.Cols + c
			field[i] = math.Min(1.6, field[i]+amount*(1-d/float64(radius+1)))
		}
	}
}

func (r *Renderer) consumeEvents(w *ocean.World) {
	for _, e := range w.Events {
		r.stamp(r.heat, e.X, e.Y, 2, 1)
		r.stamp(r.energy, e.X, e.Y, 3, 0.9)
	}
	w.Events = w.Events[:0]
}

func (r *Renderer) depositWake(w *ocean.World) {
	for i := range w.Fish {
		f := &w.Fish[i]
		if !f.Alive {
			continue
		}
		if c := r.cellIndex(f.X, f.Y); c >= 0 {
			r.energy[c] = math.Min(1.6, r.energy[c]+0.012+f.Alarm*0.03)
		}
	}
	for si := range w.Sharks {
		sh := &w.Sharks[si]
		// The tail sweeps the most water.
		for k := ocean.SharkNodes - 6; k < ocean.SharkNodes; k++ {
			if c := r.cellIndex(sh.Nodes[k*2], sh.Nodes[k*2+1]); c >= 0 {
				r.energy[c] = math.Min(1.6, r.energy[c]+0.05+sh.Speed*0.0012)
			}
		}
	}
}

// diffuse blurs and decays the fields so wakes spread out like ripples and fade.
func (r *Renderer) diffuse(dt float64) {
	cols, rows := r.Cols, r.Rows
	e, tmp := r.energy, r.energyTmp
	decay := math.Exp(-dt * 1.5)
	for row := range rows {
		for c := range cols {
			i := row*cols + c
			l, rt, u, d := e[i], e[i], e[i], e[i]
			if c > 0 {
				l = e[i-1]
			}
			if c < cols-1 {
				rt = e[i+1]
			}
			if row > 0 {
				u = e[i-cols]
			}
			if row < rows-1 {
				d = e[i+cols]
			}
			tmp[i] = (e[i]*4 + l + rt + u + d) * 0.125 * decay
		}
	}
	r.energy, r.energyTmp = tmp, e
	heatDecay := math.Exp(-dt * 2.2)
	for i := range r.heat {
		r.heat[i] *= heatDecay
	}
}

// sampleShark classifies one point against the current shark (spine and arc
// already built). Body coordinates: s along the spine (0 snout → 1 tail), v
// across it. It writes the hit* fields and reports whether the point is on
// the body.
//
// Only segments k0..k1-1 are searched: the cell centre scans the whole spine
// and its subsamples reuse the neighbourhood of the centre's nearest segment.
func (r *Renderer) sampleShark(px, py, L float64, k0, k1 int) bool {
	best := math.Inf(1)
	var bestS, bestNx, bestNy float64
	bestSign := 1.0
	bestK := k0
	for k := k0; k < k1; k++ {
		sg := &r.segs[k]
		u := ocean.Clamp(((px-sg.ax)*sg.sx+(py-sg.ay)*sg.sy)*sg.invLen2, 0, 1)
		qx, qy := sg.ax+sg.sx*u, sg.ay+sg.sy*u
		if d2 := (px-qx)*(px-qx) + (py-qy)*(py-qy); d2 < best {
			best = d2
			bestK = k
			bestS = (sg.arc + u*sg.length) / r.arcTotal
			bestSign = 1
			if sg.sx*(py-sg.ay)-sg.sy*(px-sg.ax) < 0 {
				bestSign = -1
			}
			bestNx = -sg.sy * sg.invLen * bestSign
			bestNy = sg.sx * sg.invLen * bestSign
		}
	}
	r.hitK = bestK

	a := math.Sqrt(best)
	s := bestS
	u := s * L
	shape := r.shape
	w := shape.width(s) * L
	r.hitDist = a
	r.hitS = s
	r.hitV = a * bestSign
	r.hitMargin = -1

	if head := shape.Head; head != nil && u < L*(head.Sweep+head.Depth+head.Notch) {
		// Cephalofoil: a wide wing across the snout, its tips swept back
		// toward the eyes, and on a great hammerhead a shallow notch in the
		// middle of the front edge.
		span := L * head.Span
		rr := a / span
		front := L*head.Sweep*rr*rr + L*head.Notch*math.Max(0, 1-rr/0.14)
		// Full depth almost to the tips, so the wing reads as a solid bar.
		depth := L * head.Depth * (1 - 0.2*rr*rr*rr*rr)
		if rr <= 1 && u >= front && u <= front+depth {
			if rr > 0.84 {
				r.hitFlag, r.hitShade = flagEye, 0
			} else {
				r.hitFlag = flagBody
				r.hitShade = 0.62 - 0.2*rr
				if u < front+L*0.012 {
					r.hitShade += 0.1
				}
			}
			return true
		}
	}

	if a <= w {
		r.hitMargin = w - a
		across := a / w
		// Rounded back lit from the top-left; flanks fall off into shadow.
		facing := (bestNx*-0.6 + bestNy*-0.8) * across
		shade := (0.58 - across*across*0.42 + facing*0.26) * shape.BodyShade
		flag := flagBody
		switch {
		case s > shape.Dorsal[0] && s < shape.Dorsal[1] && across < 0.3:
			// First dorsal fin, catching the light.
			flag, shade = flagRidge, 1
		case s > shape.SecondDorsal[0] && s < shape.SecondDorsal[1] && across < 0.4:
			shade = math.Max(shade, 0.82)
		case shape.Eye != nil && s > shape.Eye[0] && s < shape.Eye[1] && across > 0.6:
			flag, shade = flagEye, 0
		case r.kind == ocean.Orca && s > 0.12 && s < 0.22 && across > 0.4 && across < 0.9:
			// The orca's white eye patch.
			shade = 0.95
		case r.kind == ocean.Orca && s > 0.5 && s < 0.6 && across < 0.5:
			// Grey saddle behind the dorsal.
			shade = math.Max(shade, 0.55)
		case s > shape.Gills[0] && s < shape.Gills[1] && across > 0.58:
			// Five gill slits.
			if int(math.Floor((s-shape.Gills[0])/(shape.Gills[1]-shape.Gills[0])*5))%2 == 0 {
				shade *= 0.45
			}
		case s < 0.05:
			// Pale snout tip.
			shade = math.Max(shade, 0.7)
		case r.kind == ocean.Zebra && s > 0.08 && s < 0.55 && (across < 0.1 || math.Abs(across-0.6) < 0.08):
			// Zebra sharks carry pale ridges down the back.
			shade = math.Max(shade, 0.8)
		case r.marked(s, a*bestSign, L, across):
			flag = flagMark
		}
		r.hitFlag, r.hitShade = flag, shade
		return true
	}

	tip := r.finTipLeft
	if bestSign > 0 {
		tip = r.finTipRight
	}
	p0, p1, p2 := shape.Pectoral[0], shape.Pectoral[1], shape.Pectoral[2]
	if inTriangle(u, a, L*p0, 0, L*p1, 0, L*p2, tip) &&
		// Falcate (sickle) fins get a concave trailing edge.
		!(shape.Falcate && inTriangle(u, a, L*p1, 0, L*(p1+(p2-p1)*0.33), tip*0.55, L*p2, tip)) {
		r.hitFlag = flagFin
		if r.marked(s, a*bestSign, L, 0) {
			r.hitFlag = flagMark
		}
		reach := ocean.Clamp(a/tip, 0, 1)
		r.hitShade = 0.38 + 0.22*(1-reach)
		if shape.FinShade > 0 {
			r.hitShade = shape.FinShade - 0.15*reach
		}
		return true
	}
	v := shape.Pelvic
	if (v[3] > 0 && inTriangle(u, a, L*v[0], 0, L*v[1], 0, L*v[2], L*v[3])) ||
		(shape.Caudal > 0 && inTriangle(u, a, L*shape.CaudalRoot[0], 0, L*shape.CaudalRoot[1], 0, L, L*shape.Caudal)) {
		r.hitFlag, r.hitShade = flagFin, 0.36
		return true
	}
	return false
}

// marked reports whether a body point (s along, v across) falls on a dark
// marking: tiger bars, zebra-shark spots, humpback head knobs.
func (r *Renderer) marked(s, v, L, across float64) bool {
	switch r.kind {
	case ocean.Tiger:
		// Wavy vertical bars over the back half, fading out toward the head.
		if s < 0.2 || s > 0.9 || across > 0.85 {
			return false
		}
		wobble := 0.35 * math.Sin((v/L)*25+s*5)
		bar := math.Sin(s*math.Pi*2*9 + wobble)
		return bar > 0.65-0.5*ocean.Smoothstep(0.2, 0.42, s)+0.4*ocean.Smoothstep(0.8, 0.9, s)
	case ocean.Zebra:
		// Leopard spots on a jittered grid, so they ride along with the skin.
		if s < 0.07 {
			return false
		}
		return r.spot(s, v, L, 0.06, 0.045, 0.08)
	case ocean.Humpback:
		if s < 0.02 || s > 0.13 || across > 0.7 {
			return false
		}
		return r.spot(s, v, L, 0.03, 0.03, 0.06)
	}
	return false
}

func (r *Renderer) spot(s, v, L, stepU, stepV, radius2 float64) bool {
	gu := s / stepU
	gv := v / (L * stepV)
	cu, cv := math.Floor(gu), math.Floor(gv)
	jx := ocean.Hash3(int(cu), int(cv), r.sharkID)*0.5 + 0.25
	jy := ocean.Hash3(int(cu), int(cv), r.sharkID+11)*0.5 + 0.25
	return (gu-cu-jx)*(gu-cu-jx)+(gv-cv-jy)*(gv-cv-jy) < radius2
}

// rasterizeSharks builds a smooth, undulating render spine from each sim
// spine, then supersamples every cell in the bounding box against the shape.
// Fully covered cells become solid skin; partial cells become edge glyphs.
func (r *Renderer) rasterizeSharks(sharks []ocean.Shark, t float64) {
	const N = ocean.SharkNodes
	cw, ch, cols, rows := r.CellW, r.CellH, r.Cols, r.Rows
	clear(r.sharkFlag)

	for si := range sharks {
		sh := &sharks[si]
		L := sh.Length
		n := &sh.Nodes
		swim := ocean.Clamp(sh.Speed/60, 0.4, 1.4)
		r.shape = Shapes[sh.Kind]
		r.kind = sh.Kind
		r.sharkID = sh.ID
		ki := kindIndex[sh.Kind]

		// Travelling body wave, growing toward the tail.
		for k := range N {
			s := float64(k) / (N - 1)
			a, b := max(0, k-1), min(N-1, k+1)
			tx, ty := n[a*2]-n[b*2], n[a*2+1]-n[b*2+1]
			tl := math.Hypot(tx, ty)
			if tl == 0 {
				tl = 1
			}
			tx, ty = tx/tl, ty/tl
			amp := L * 0.05 * (0.1 + 0.9*math.Pow(s, 1.8)) * swim * r.shape.Wave
			off := amp * math.Sin(sh.Phase-s*5.2)
			r.spine[k*2] = n[k*2] - ty*off
			r.spine[k*2+1] = n[k*2+1] + tx*off
		}
		r.arc[0] = 0
		for k := 1; k < N; k++ {
			r.arc[k] = r.arc[k-1] + math.Hypot(r.spine[k*2]-r.spine[(k-1)*2], r.spine[k*2+1]-r.spine[(k-1)*2+1])
		}
		r.arcTotal = r.arc[N-1]
		if r.arcTotal == 0 {
			r.arcTotal = 1
		}
		for k := range N - 1 {
			ax, ay := r.spine[k*2], r.spine[k*2+1]
			sx, sy := r.spine[(k+1)*2]-ax, r.spine[(k+1)*2+1]-ay
			len2 := sx*sx + sy*sy
			if len2 == 0 {
				len2 = 1
			}
			ln := math.Sqrt(len2)
			r.segs[k] = segment{ax: ax, ay: ay, sx: sx, sy: sy, invLen2: 1 / len2, invLen: 1 / ln, length: ln, arc: r.arc[k]}
		}

		// Bank into turns: the inside fin foreshortens, the outside one spreads.
		bank := ocean.Clamp(sh.Turn*0.45, -0.35, 0.35)
		flex := 1 + 0.06*math.Sin(sh.Phase*0.5)
		reachFin := r.shape.Pectoral[3]
		r.finTipRight = L * reachFin * flex * (1 - bank)
		r.finTipLeft = L * reachFin * flex * (1 + bank)

		minX, minY := math.Inf(1), math.Inf(1)
		maxX, maxY := math.Inf(-1), math.Inf(-1)
		for k := range N {
			minX = math.Min(minX, r.spine[k*2])
			maxX = math.Max(maxX, r.spine[k*2])
			minY = math.Min(minY, r.spine[k*2+1])
			maxY = math.Max(maxY, r.spine[k*2+1])
		}
		reach := L * 0.36
		c0 := max(0, int(math.Floor((minX-reach)/cw)))
		c1 := min(cols-1, int(math.Floor((maxX+reach)/cw)))
		r0 := max(0, int(math.Floor((minY-reach)/ch)))
		r1 := min(rows-1, int(math.Floor((maxY+reach)/ch)))
		if c0 > c1 || r0 > r1 {
			continue
		}
		diag := math.Hypot(cw, ch)
		skipBeyond := reach + diag

		r.nearestSegments(c0, c1, r0, r1, skipBeyond)
		lim2 := skipBeyond * skipBeyond

		for row := r0; row <= r1; row++ {
			for c := c0; c <= c1; c++ {
				i := row*cols + c
				// Beyond every fin and head reach: nothing can be drawn here.
				if r.nearD2[i] > lim2 {
					continue
				}
				x, y := float64(c)*cw, float64(row)*ch
				px, py := x+cw/2, y+ch/2
				nk := int(r.nearK[i])
				centerHit := r.sampleShark(px, py, L, nk, nk+1)
				flag, shade, s, v := r.hitFlag, r.hitShade, r.hitS, r.hitV
				k0, k1 := max(0, r.hitK-2), min(N-1, r.hitK+3)

				hits := 0
				if centerHit {
					hits = 1
				}
				// Deep inside the body every subsample would hit too.
				if centerHit && r.hitMargin > diag {
					hits = 5
				}
				for k := 0; hits < 5 && k < len(subsamples); k += 2 {
					if r.sampleShark(x+subsamples[k]*cw, y+subsamples[k+1]*ch, L, k0, k1) {
						hits++
					}
				}
				if hits == 0 {
					continue
				}
				coverage := float64(hits) / 5

				// Body-anchored key so each glyph rides along with the skin.
				r.sharkKey[i] = uint32(ocean.Hash3(int(math.Round(s*52)), int(math.Round(v/(cw*0.9))), sh.ID) * 4294967295)
				r.sharkDepth[i] = sh.Depth
				r.sharkKind[i] = ki

				if centerHit && coverage >= 0.6 {
					r.sharkFlag[i] = flag
					flicker := (ocean.Hash3(c, row, int(math.Floor(t*6))) - 0.5) * 0.06
					r.sharkShade[i] = ocean.Clamp(shade+flicker, 0, 1)
				} else {
					r.sharkFlag[i] = flagEdge
					r.sharkShade[i] = coverage
				}
			}
		}
	}
}

// nearestSegments records, for every cell centre in the box, the closest
// render-spine segment within reach. Splatting each segment over its own
// padded bounding box touches far fewer cells than scanning the whole spine
// from every cell, and picks the same segment (ties go to the lower index).
func (r *Renderer) nearestSegments(c0, c1, r0, r1 int, reach float64) {
	cw, ch, cols := r.CellW, r.CellH, r.Cols
	for row := r0; row <= r1; row++ {
		for c := c0; c <= c1; c++ {
			r.nearD2[row*cols+c] = math.Inf(1)
		}
	}
	for k := range r.segs {
		sg := &r.segs[k]
		minX := math.Min(sg.ax, sg.ax+sg.sx) - reach
		maxX := math.Max(sg.ax, sg.ax+sg.sx) + reach
		minY := math.Min(sg.ay, sg.ay+sg.sy) - reach
		maxY := math.Max(sg.ay, sg.ay+sg.sy) + reach
		sc0 := max(c0, int(math.Ceil((minX-cw/2)/cw)))
		sc1 := min(c1, int(math.Floor((maxX-cw/2)/cw)))
		sr0 := max(r0, int(math.Ceil((minY-ch/2)/ch)))
		sr1 := min(r1, int(math.Floor((maxY-ch/2)/ch)))
		for row := sr0; row <= sr1; row++ {
			py := float64(row)*ch + ch/2
			for c := sc0; c <= sc1; c++ {
				px := float64(c)*cw + cw/2
				u := ocean.Clamp(((px-sg.ax)*sg.sx+(py-sg.ay)*sg.sy)*sg.invLen2, 0, 1)
				qx, qy := sg.ax+sg.sx*u, sg.ay+sg.sy*u
				d2 := (px-qx)*(px-qx) + (py-qy)*(py-qy)
				i := row*cols + c
				if d2 < r.nearD2[i] {
					r.nearD2[i] = d2
					r.nearK[i] = int8(k)
				}
			}
		}
	}
}

// quantize snaps a 0-255 level to a coarse step so colours repeat and the
// host can batch long runs of identical draws.
func quantize(level float64) float64 {
	return math.Round(level/6) * 6
}

func (r *Renderer) drawSharks(t float64) {
	for i, flag := range r.sharkFlag {
		if flag == 0 {
			continue
		}
		key := r.sharkKey[i]
		shade := r.sharkShade[i]
		dim := 0.7 + 0.3*r.sharkDepth[i]
		shape := Shapes[ocean.AllKinds[r.sharkKind[i]]]
		tr, tg, tb := shape.Tint[0], shape.Tint[1], shape.Tint[2]

		// Colours re-roll on a staggered clock so the hide shimmers.
		tick := int(math.Floor(t*2.4 + float64(key%997)/97))
		roll := ocean.Hash3(int(key), tick, 7)

		if flag == flagEdge {
			// Soft, anti-aliased silhouette: no tile, just a light glyph.
			set := edgeLight
			if shade >= 0.5 {
				set = edgeHeavy
			}
			var c RGBA
			if roll < 0.07 {
				c = r.accent(key)
			} else {
				g := quantize((70 + shade*80) * dim)
				c = r.theme.rgb(g*tr, g*tg, math.Min(255, g*tb+6))
			}
			r.emit(phaseGlyphs, i, KindGlyph, set[(key>>8)%uint32(len(set))], c)
			continue
		}

		var level float64
		var glyph byte
		switch flag {
		case flagRidge:
			level = 226 * (0.35 + 0.65*shape.BodyShade)
			glyph = ridgeGlyphs[(key>>8)%uint32(len(ridgeGlyphs))]
		case flagEye:
			level = 12
			glyph = 'o'
		case flagMark:
			level = 24 + shade*22
			glyph = markGlyphs[(key>>8)%uint32(len(markGlyphs))]
		case flagFin:
			level = 40 + shade*120
			glyph = finGlyphs[(key>>8)%uint32(len(finGlyphs))]
		default:
			level = 46 + shade*160
			jitter := int((key>>4)%3) - 1
			idx := int(math.Floor((1-shade)*8.99)) + jitter
			glyph = skinRamp[min(max(idx, 0), len(skinRamp)-1)]
		}
		level = quantize(level * dim)
		// Tinted per kind; the classic tint is a cool blue-grey, like a back
		// seen through deep water.
		r.emit(phaseTiles, i, KindTile, 0,
			r.theme.rgb(math.Min(255, level*tr), math.Min(255, level*tg), math.Min(255, level*tb+4)))

		var c RGBA
		switch {
		case flag == flagEye:
			c = r.theme.rgb(eyeRGB[0], eyeRGB[1], eyeRGB[2])
		case flag == flagMark:
			c = r.theme.rgb(70*tr, 70*tg, 70*tb)
		case roll < 0.09:
			c = r.accent(key)
		case roll < 0.13:
			c = r.theme.rgb(255, 255, 255)
		case shade > 0.55:
			c = r.theme.rgb(litInkRGB[0], litInkRGB[1], litInkRGB[2])
		default:
			c = r.theme.rgb(darkInkRGB[0], darkInkRGB[1], darkInkRGB[2])
		}
		r.emit(phaseGlyphs, i, KindGlyph, glyph, c)
	}
}

func (r *Renderer) accent(key uint32) RGBA {
	a := accents[key%uint32(len(accents))]
	return r.theme.rgb(a[0], a[1], a[2])
}

// markFish snaps each fish to a cell: far fish are dim, near fish brighter,
// and turning or panicking fish glint. Nearer fish claim a cell first when
// two land in the same one.
func (r *Renderer) markFish(w *ocean.World) {
	r.frameID++
	frame := r.frameID
	for pass := range 2 {
		for fi := range w.Fish {
			f := &w.Fish[fi]
			near := 1
			if f.Depth > 0.78 {
				near = 0
			}
			if !f.Alive || near != pass {
				continue
			}
			i := r.cellIndex(f.X, f.Y)
			if i < 0 || r.fishStamp[i] == frame || r.sharkFlag[i] != 0 {
				continue
			}
			r.fishStamp[i] = frame
			glint := ocean.Clamp(math.Abs(f.Turn)/3.5, 0, 1)
			v := 0.3 + (0.4*(f.Depth-0.55))/0.45 + 0.3*math.Max(glint, f.Alarm)
			b := min(max(int(math.Floor(v*float64(len(fishAlphas)))), 0), len(fishAlphas)-1)
			r.emit(phaseFish, i, KindGlyph, '-', r.theme.over(230, 238, 255, fishAlphas[b]))
		}
	}
}

func (r *Renderer) drawDots() {
	for i, e := range r.energy {
		if e <= 0.45 || r.sharkFlag[i] != 0 || r.fishStamp[i] == r.frameID {
			continue
		}
		level := min(max(int(math.Floor((e-0.45)*5)), 0), wakeLevels-1)
		a := float64(level+1) / wakeLevels
		r.emit(phaseDots, i, KindDot, 0, r.theme.over(wakeRGB[0], wakeRGB[1], wakeRGB[2], a))
	}
}

func (r *Renderer) drawHeat() {
	for i, h := range r.heat {
		if h < 0.08 {
			continue
		}
		a := math.Round(h*10) / 10
		var c RGBA
		glyph := byte('.')
		if h > 0.5 {
			c = r.theme.over(255, 120, 60, math.Min(1, a))
		} else {
			c = r.theme.over(220, 40, 40, math.Min(1, a*1.6))
		}
		if h > 0.6 {
			glyph = '*'
		}
		r.emit(phaseHeat, i, KindGlyph, glyph, c)
	}
}
