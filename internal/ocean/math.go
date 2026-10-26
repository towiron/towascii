// Package ocean simulates fish schools, sharks and whales seen from above.
// Ported from stackedlol/ascii-sharks (MIT), see THIRD_PARTY_LICENSES.md.
package ocean

import (
	"math"
	"math/rand/v2"
)

const Tau = math.Pi * 2

func Clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func Smoothstep(e0, e1, x float64) float64 {
	t := Clamp((x-e0)/(e1-e0), 0, 1)
	return t * t * (3 - 2*t)
}

// AngleDiff is the shortest signed angle from a to b, in (-π, π].
func AngleDiff(a, b float64) float64 {
	d := math.Mod(b-a, Tau)
	if d > math.Pi {
		d -= Tau
	}
	if d < -math.Pi {
		d += Tau
	}
	return d
}

// Hash3 is a cheap deterministic integer hash into [0, 1). The uint32
// arithmetic mirrors JS Math.imul and >>> so textures match the original.
func Hash3(x, y, z int) float64 {
	h := uint32(x)*374761393 ^ uint32(y)*668265263 ^ uint32(z)*1442695041
	h = (h ^ h>>13) * 1274126177
	h ^= h >> 16
	return float64(h) / 4294967296
}

const (
	k1           = 1.0 / 260
	k2           = 1.0 / 310
	k3           = 1.0 / 190
	currentScale = 3400
)

// current is a divergence-free flow (the curl of a drifting stream function),
// so it moves schools around without bunching them into one corner.
func current(x, y, t float64) (float64, float64) {
	a := x*k1 + t*0.07
	b := y*k2 - t*0.05
	c := (x+y)*k3 + t*0.11
	u := (-k2*math.Sin(a)*math.Sin(b) + 0.5*k3*math.Cos(c)) * currentScale
	v := (-k1*math.Cos(a)*math.Cos(b) - 0.5*k3*math.Cos(c)) * currentScale
	return u, v
}

func randRange(r *rand.Rand, lo, hi float64) float64 {
	return lo + r.Float64()*(hi-lo)
}

func randInt(r *rand.Rand, lo, hi int) int {
	return lo + r.IntN(hi-lo+1)
}

func randSign(r *rand.Rand) float64 {
	if r.Float64() < 0.5 {
		return 1
	}
	return -1
}
