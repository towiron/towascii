package ocean

import "math/rand/v2"

type Species struct {
	Name     string
	Cruise   float64
	MaxSpeed float64
	MinSpeed float64
	// Weight is the relative odds of a new school being this species.
	Weight     int
	SchoolSize [2]int
	// Spacing is personal space in grid cells (x, y), so every fish gets its own glyph.
	Spacing [2]float64
	// Different behaviour mixes produce different collective shapes (Couzin et
	// al. 2002): strong alignment gives polarised schools and ribbons, low
	// alignment gives swarms, and a tangential pull (Mill) gives a bait ball.
	Align        float64
	Cohesion     float64
	Jitter       float64
	Mill         float64
	VerticalDrag float64
}

var FishSpecies = []Species{
	{Name: "sardine", Cruise: 36, MaxSpeed: 64, MinSpeed: 18, Weight: 3, SchoolSize: [2]int{18, 28}, Spacing: [2]float64{1.8, 1.1},
		Align: 1.6, Cohesion: 1.6, Jitter: 40, Mill: 70, VerticalDrag: 0.5},
	{Name: "anchovy", Cruise: 46, MaxSpeed: 78, MinSpeed: 22, Weight: 3, SchoolSize: [2]int{20, 32}, Spacing: [2]float64{1.5, 1},
		Align: 3.2, Cohesion: 0.45, Jitter: 30, Mill: 0, VerticalDrag: 1.4},
	{Name: "silverside", Cruise: 40, MaxSpeed: 70, MinSpeed: 20, Weight: 3, SchoolSize: [2]int{12, 20}, Spacing: [2]float64{2, 1.1},
		Align: 2.4, Cohesion: 1.1, Jitter: 35, Mill: 0, VerticalDrag: 1.6},
	{Name: "mackerel", Cruise: 32, MaxSpeed: 60, MinSpeed: 16, Weight: 2, SchoolSize: [2]int{5, 9}, Spacing: [2]float64{2.6, 1.3},
		Align: 2, Cohesion: 0.9, Jitter: 25, Mill: 0, VerticalDrag: 2},
	{Name: "goby", Cruise: 26, MaxSpeed: 56, MinSpeed: 10, Weight: 1, SchoolSize: [2]int{8, 12}, Spacing: [2]float64{1.4, 1.1},
		Align: 0.35, Cohesion: 1.4, Jitter: 110, Mill: 0, VerticalDrag: 0.3},
}

var totalFishWeight = func() int {
	n := 0
	for _, s := range FishSpecies {
		n += s.Weight
	}
	return n
}()

func pickSpecies(r *rand.Rand) int {
	x := r.Float64() * float64(totalFishWeight)
	for i, s := range FishSpecies {
		x -= float64(s.Weight)
		if x <= 0 {
			return i
		}
	}
	return 0
}
