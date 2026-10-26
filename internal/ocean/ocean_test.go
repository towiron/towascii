package ocean

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestHash3MatchesReference(t *testing.T) {
	// Values computed with the original JavaScript hash3.
	cases := []struct {
		x, y, z int
		want    float64
	}{
		{0, 0, 0, 0},
		{1, 2, 3, 0.6394013063982129},
		{-5, 17, 4000000000, 0.6148328136187047},
		{148, 57, 7, 0.15608854941092432},
		{123456789, -987654321, 11, 0.4570880539249629},
	}
	for _, c := range cases {
		if got := Hash3(c.x, c.y, c.z); got != c.want {
			t.Errorf("Hash3(%d, %d, %d) = %v, want %v", c.x, c.y, c.z, got, c.want)
		}
	}
}

func TestAngleDiff(t *testing.T) {
	cases := []struct{ a, b, want float64 }{
		{0, 1, 1},
		{1, 0, -1},
		{0.1, Tau - 0.1, -0.2},
		{Tau - 0.1, 0.1, 0.2},
		{0, 3 * Tau, 0},
	}
	for _, c := range cases {
		if got := AngleDiff(c.a, c.b); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("AngleDiff(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func newTestWorld(seed uint64, specs []Spec) *World {
	w := NewWorld(specs, rand.New(rand.NewPCG(seed, 1)))
	w.Resize(1920, 1080, 13, 19)
	return w
}

var testSpecs = []Spec{{Kind: Classic, Size: 1}, {Kind: Hammerhead, Size: 0.8}, {Kind: Humpback, Size: 0.9}}

func TestSeedPopulation(t *testing.T) {
	w := newTestWorld(1, testSpecs)
	if len(w.Fish) != 340 {
		t.Errorf("fish = %d, want 340 (area-based, clamped)", len(w.Fish))
	}
	if len(w.Sharks) != len(testSpecs) {
		t.Fatalf("sharks = %d, want %d", len(w.Sharks), len(testSpecs))
	}
	for i := 1; i < len(w.Sharks); i++ {
		if w.Sharks[i-1].Length > w.Sharks[i].Length {
			t.Errorf("sharks not sorted by length: %v > %v", w.Sharks[i-1].Length, w.Sharks[i].Length)
		}
	}
}

func TestStaysOnScreenAndFeeds(t *testing.T) {
	w := newTestWorld(2, testSpecs)
	var hunger float64
	for _, sh := range w.Sharks {
		if sh.Kind.IsWhale() {
			hunger = sh.Hunger
		}
	}
	eaten := 0
	for range 60 * 120 {
		w.Step(1.0 / 60)
		eaten += len(w.Events)
		w.Events = w.Events[:0]
	}
	if eaten == 0 {
		t.Error("no fish eaten in two minutes")
	}
	alive := 0
	for _, f := range w.Fish {
		if !f.Alive {
			continue
		}
		alive++
		if f.X < -60 || f.X > w.Width+60 || f.Y < -60 || f.Y > w.Height+60 {
			t.Fatalf("fish escaped: (%v, %v)", f.X, f.Y)
		}
	}
	if alive < len(w.Fish)*3/4 {
		t.Errorf("only %d of %d fish alive; respawn should keep the population steady", alive, len(w.Fish))
	}
	for _, sh := range w.Sharks {
		x, y := sh.Nodes[0], sh.Nodes[1]
		if x < -sh.Length || x > w.Width+sh.Length || y < -sh.Length || y > w.Height+sh.Length {
			t.Errorf("%s wandered off: snout at (%v, %v)", sh.Kind, x, y)
		}
		if sh.Kind.IsWhale() && sh.Hunger != hunger {
			t.Errorf("whale hunger changed: %v -> %v", hunger, sh.Hunger)
		}
	}
}

func TestDeterministic(t *testing.T) {
	a := newTestWorld(3, testSpecs)
	b := newTestWorld(3, testSpecs)
	for range 600 {
		a.Step(1.0 / 60)
		b.Step(1.0 / 60)
	}
	if a.Sharks[0].Nodes != b.Sharks[0].Nodes || a.Fish[17] != b.Fish[17] {
		t.Error("same seed produced different worlds")
	}
}

func TestResizeKeepsFishInside(t *testing.T) {
	w := newTestWorld(4, testSpecs)
	w.Resize(800, 600, 8, 12)
	for _, f := range w.Fish {
		if f.X < 0 || f.X > 800 || f.Y < 0 || f.Y > 600 {
			t.Fatalf("fish outside resized world: (%v, %v)", f.X, f.Y)
		}
	}
	w.Step(1.0 / 60)
}

func TestRandomSpecs(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 5))
	for range 50 {
		specs := RandomSpecs(r)
		if len(specs) < 3 || len(specs) > 5 {
			t.Fatalf("got %d specs", len(specs))
		}
		if specs[0].Size != 1 || specs[0].Kind.IsWhale() {
			t.Errorf("first spec should be the full-size shark: %+v", specs[0])
		}
		whales := 0
		for _, s := range specs {
			if s.Kind.IsWhale() {
				whales++
			}
		}
		if whales > 1 {
			t.Errorf("more than one whale: %+v", specs)
		}
	}
}
