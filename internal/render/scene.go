package render

import (
	"math"
	"math/rand/v2"

	"ascii-wallpaper/internal/ocean"
)

const (
	simStep     = 1.0 / 60
	maxSimSteps = 4
)

// Scene is one screen's ocean: a world stepped at a fixed rate and its renderer.
type Scene struct {
	World    *ocean.World
	Renderer *Renderer
	acc      float64
}

func NewScene(rng *rand.Rand, white bool) *Scene {
	return &Scene{
		World:    ocean.NewWorld(ocean.RandomSpecs(rng), rng),
		Renderer: NewRenderer(white),
	}
}

func (s *Scene) Resize(width, height float64) Layout {
	l := s.Renderer.Resize(width, height)
	s.World.Resize(width, height, l.CellW, l.CellH)
	return l
}

// Frame advances the simulation by dt in fixed steps, dropping time it cannot
// catch up on (after sleep or a stall) instead of fast-forwarding.
func (s *Scene) Frame(dt float64) []Item {
	dt = math.Min(dt, 0.1)
	s.acc += dt
	steps := 0
	for s.acc >= simStep && steps < maxSimSteps {
		s.World.Step(simStep)
		s.acc -= simStep
		steps++
	}
	if steps == maxSimSteps {
		s.acc = 0
	}
	return s.Renderer.Render(s.World, dt)
}
