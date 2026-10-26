package render

import (
	"math/rand/v2"
	"testing"

	"ascii-wallpaper/internal/ocean"
)

func BenchmarkFrame(b *testing.B) {
	rng := rand.New(rand.NewPCG(1, 2))
	s := &Scene{
		World:    ocean.NewWorld([]ocean.Spec{{Kind: ocean.Classic, Size: 1}, {Kind: ocean.Hammerhead, Size: 0.8}, {Kind: ocean.Tiger, Size: 0.75}, {Kind: ocean.Humpback, Size: 0.9}}, rng),
		Renderer: NewRenderer(false),
	}
	s.Resize(1920, 1080)
	for range 60 {
		s.Frame(1.0 / 30)
	}
	b.ResetTimer()
	for b.Loop() {
		s.Frame(1.0 / 30)
	}
}
