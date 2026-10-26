package ocean

import "math/rand/v2"

type Kind string

const (
	Classic     Kind = "classic"
	Hammerhead  Kind = "hammerhead"
	Zebra       Kind = "zebra"
	Tiger       Kind = "tiger"
	GreatHammer Kind = "greathammer"
	Mako        Kind = "mako"
	Thresher    Kind = "thresher"
	Nurse       Kind = "nurse"
	Humpback    Kind = "humpback"
	Orca        Kind = "orca"
)

var (
	SharkKinds = []Kind{Classic, Hammerhead, Zebra, Tiger, GreatHammer, Mako, Thresher, Nurse}
	WhaleKinds = []Kind{Humpback, Orca}
	AllKinds   = append(append([]Kind{}, SharkKinds...), WhaleKinds...)
)

func (k Kind) IsWhale() bool {
	return k == Humpback || k == Orca
}

// Spec says which predator to put in the water; Size is relative to the biggest.
type Spec struct {
	Kind Kind
	Size float64
}

// RandomSpecs picks the cast for one screen: a few sharks of mixed kinds at
// different depths, sometimes with a whale cruising among them.
func RandomSpecs(r *rand.Rand) []Spec {
	n := randInt(r, 3, 4)
	specs := make([]Spec, 0, n+1)
	for i := range n {
		size := 1.0
		if i > 0 {
			size = randRange(r, 0.68, 0.9)
		}
		specs = append(specs, Spec{Kind: SharkKinds[r.IntN(len(SharkKinds))], Size: size})
	}
	if r.Float64() < 0.6 {
		specs = append(specs, Spec{Kind: WhaleKinds[r.IntN(len(WhaleKinds))], Size: randRange(r, 0.85, 1)})
	}
	return specs
}
