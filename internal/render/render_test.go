package render

import (
	"flag"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"

	"ascii-wallpaper/internal/ocean"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestLayoutFor(t *testing.T) {
	cases := []struct {
		w, h float64
		want Layout
	}{
		{1920, 1080, Layout{CellW: 13, CellH: 19, DotSize: 3, Cols: 148, Rows: 57}},
		{1440, 900, Layout{CellW: 12, CellH: 17, DotSize: 3, Cols: 120, Rows: 53}},
		{800, 600, Layout{CellW: 8, CellH: 12, DotSize: 3, Cols: 100, Rows: 50}},
	}
	for _, c := range cases {
		if got := LayoutFor(c.w, c.h); got != c.want {
			t.Errorf("LayoutFor(%v, %v) = %+v, want %+v", c.w, c.h, got, c.want)
		}
	}
}

// straightScene lays one creature out flat, snout to the right, with no fish,
// so its silhouette is deterministic.
func straightScene(kind ocean.Kind) (*ocean.World, *Renderer, Layout) {
	w := ocean.NewWorld([]ocean.Spec{{Kind: kind, Size: 1}}, rand.New(rand.NewPCG(1, 2)))
	r := NewRenderer(false)
	l := r.Resize(1280, 720)
	w.Resize(1280, 720, l.CellW, l.CellH)
	w.Fish = w.Fish[:0]
	sh := &w.Sharks[0]
	for k := range ocean.SharkNodes {
		sh.Nodes[k*2] = 900 - float64(k)*sh.Length/(ocean.SharkNodes-1)
		sh.Nodes[k*2+1] = 360
	}
	sh.Heading, sh.Speed, sh.Phase, sh.Turn = 0, 0, math.Pi/2, 0
	return w, r, l
}

func TestSilhouettesGolden(t *testing.T) {
	for _, kind := range ocean.AllKinds {
		t.Run(string(kind), func(t *testing.T) {
			w, r, l := straightScene(kind)
			got := Text(r.Render(w, 0), l)
			path := filepath.Join("testdata", string(kind)+".golden")
			if *update {
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run go test -update to create it)", err)
			}
			if got != string(want) {
				t.Errorf("silhouette changed; run go test -update and review the diff\n%s", got)
			}
		})
	}
}

func TestSampleShark(t *testing.T) {
	w, r, _ := straightScene(ocean.Classic)
	r.Render(w, 0)
	L := w.Sharks[0].Length
	mid := 900 - L*0.6
	if !r.sampleShark(mid, 360, L, 0, ocean.SharkNodes-1) || r.hitFlag != flagBody {
		t.Errorf("point on the spine mid-body: hit=%v flag=%d", r.hitFlag != 0, r.hitFlag)
	}
	if r.sampleShark(mid, 360+L*0.5, L, 0, ocean.SharkNodes-1) {
		t.Error("point half a body length to the side should miss")
	}
	if r.sampleShark(900+L*0.2, 360, L, 0, ocean.SharkNodes-1) {
		t.Error("point ahead of the snout should miss")
	}
}

func TestDrawListOrder(t *testing.T) {
	s := NewScene(rand.New(rand.NewPCG(7, 7)), false)
	l := s.Resize(1440, 900)
	var items []Item
	for range 90 {
		items = s.Frame(1.0 / 30)
	}
	if len(items) == 0 {
		t.Fatal("empty frame")
	}
	if len(items) > 3*l.Cols*l.Rows {
		t.Fatalf("%d items exceed the host buffer of %d", len(items), 3*l.Cols*l.Rows)
	}
	phaseOf := func(it Item) int {
		switch {
		case it.Kind == KindDot:
			return phaseDots
		case it.Kind == KindTile:
			return phaseTiles
		case it.Glyph == '-':
			return phaseFish
		}
		return -1
	}
	last := -1
	tiles := map[[2]uint16]bool{}
	for i, it := range items {
		if int(it.Col) >= l.Cols || int(it.Row) >= l.Rows {
			t.Fatalf("item %d outside grid: %+v", i, it)
		}
		if p := phaseOf(it); p >= 0 {
			if p < last {
				t.Fatalf("item %d (%+v) in phase %d after phase %d", i, it, p, last)
			}
			last = p
		}
		if it.Kind == KindTile {
			cell := [2]uint16{it.Col, it.Row}
			if tiles[cell] {
				t.Fatalf("two tiles in cell %v", cell)
			}
			tiles[cell] = true
		}
		if i > 0 && phaseOf(it) == phaseOf(items[i-1]) && phaseOf(it) >= 0 &&
			it.Kind == items[i-1].Kind && it.RGBA < items[i-1].RGBA {
			t.Fatalf("items %d and %d not sorted by colour", i-1, i)
		}
	}
}

func TestThemeInvertsLightness(t *testing.T) {
	var th theme
	th.set(true)
	if got := th.rgb(0, 0, 0); got != 0xFFFFFFFF {
		t.Errorf("black -> %08x, want white", got)
	}
	if got := th.rgb(255, 255, 255); got != 0x000000FF {
		t.Errorf("white -> %08x, want black", got)
	}
	r, g, b := th.rgb(110, 230, 255).Channels()
	h0, _, l0 := toHSL(110.0/255, 230.0/255, 1)
	h1, _, l1 := toHSL(float64(r)/255, float64(g)/255, float64(b)/255)
	if math.Abs(h0-h1) > 0.01 {
		t.Errorf("wake hue drifted: %v -> %v", h0, h1)
	}
	if math.Abs(l1-(1-l0)) > 0.01 {
		t.Errorf("wake lightness %v, want %v", l1, 1-l0)
	}
	th.set(false)
	if got := th.rgb(110, 230, 255); got != 0x6EE6FFFF {
		t.Errorf("black theme changed the colour: %08x", got)
	}
}
