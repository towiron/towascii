package ocean

// Fish: Reynolds boids (separation / alignment / cohesion) in Couzin-style
// zones, with a blind spot behind each fish. Alarm spreads from fish to fish
// through the school, so a whole school bolts even when only its edge can see
// the shark. Fleeing fish break sideways around the shark's path, which gives
// the fountain effect seen in real bait balls.
//
// Sharks: Reynolds steering (wander + pursuit + separation) on a
// follow-the-leader spine. Hunger builds over time and drops when they eat.
// They prefer fish in sparse spots, so stragglers get picked off. Eaten fish
// come back later as new schools swimming in from the edges. Whales share the
// steering but never hunt.

import (
	"math"
	"math/rand/v2"
	"slices"
)

const SharkNodes = 20

type Fish struct {
	X, Y, VX, VY float64
	Species      int
	// Alarm goes from 0 (calm) to 1 (panicking) and spreads to neighbours.
	Alarm     float64
	Alive     bool
	RespawnAt float64
	// Phase is the tail-beat phase.
	Phase float64
	// Turn is the smoothed angular velocity (rad/s); hard turns flash silver.
	Turn float64
	// Depth goes from 0.55 (far) to 1 (near).
	Depth float64
	// MillDir is the rotation sense a school shares when it mills into a bait ball.
	MillDir float64
}

type Shark struct {
	ID     int
	Kind   Kind
	Length float64
	// Nodes are spine positions, snout first: x0, y0, x1, y1, ...
	Nodes       [SharkNodes * 2]float64
	Heading     float64
	Speed       float64
	Phase       float64
	Turn        float64
	Hunger      float64
	Wander      float64
	OrbitDir    float64
	OrbitRadius float64
	Target      int
	RetargetAt  float64
	// Depth goes from 0 (far, dim) to 1 (near, bright).
	Depth float64
}

func (s *Shark) Hunts() bool {
	return !s.Kind.IsWhale()
}

type Event struct {
	X, Y float64
}

const (
	fishSeparationK = 420
	fishAlignK      = 1.4
	fishCohesionK   = 1
	fishFleeK       = 1100
	blindCos        = -0.6 // ~127° either side of the heading is visible
	noTarget        = -1
)

type World struct {
	Width, Height float64
	Time          float64
	Fish          []Fish
	Sharks        []Shark
	// Events are fish eaten since the renderer last drained them.
	Events []Event

	rng   *rand.Rand
	specs []Spec

	perception  float64
	sepX, sepY  []float64
	foreignSep  float64
	bucketCols  int
	bucketRows  int
	bucketHead  []int32
	bucketCount []uint16
	bucketNext  []int32
	lastSpawn   float64
}

func NewWorld(specs []Spec, rng *rand.Rand) *World {
	return &World{rng: rng, specs: specs, perception: 60, foreignSep: 40}
}

func (w *World) Resize(width, height, cellW, cellH float64) {
	first := w.Width == 0
	w.Width, w.Height = width, height
	// Spacing is set in grid cells so every fish lands in its own cell.
	w.sepX = w.sepX[:0]
	w.sepY = w.sepY[:0]
	for _, sp := range FishSpecies {
		w.sepX = append(w.sepX, sp.Spacing[0]*cellW)
		w.sepY = append(w.sepY, sp.Spacing[1]*cellH)
	}
	w.perception = math.Max(cellH*3, cellW*5.5)
	w.foreignSep = cellW * 3

	if first || len(w.Fish) == 0 {
		w.seed()
		return
	}
	for i := range w.Fish {
		f := &w.Fish[i]
		f.X = Clamp(f.X, 0, width)
		f.Y = Clamp(f.Y, 0, height)
	}
}

func (w *World) seed() {
	r := w.rng
	W, H := w.Width, w.Height
	minSide := math.Min(W, H)
	baseLength := Clamp(minSide*0.44, 170, 430)
	n := len(w.specs)

	w.Sharks = w.Sharks[:0]
	for i, spec := range w.specs {
		length := baseLength * spec.Size
		if spec.Kind.IsWhale() {
			length *= 1.3
		}
		orbitRadius := minSide * randRange(r, 0.28, 0.4)
		a := float64(i)/float64(n)*Tau + randRange(r, -0.4, 0.4)
		orbitDir := 1.0
		if i%2 == 1 {
			orbitDir = -1
		}
		x := W/2 + math.Cos(a)*orbitRadius*(W/minSide)*0.9
		y := H/2 + math.Sin(a)*orbitRadius
		heading := a + orbitDir*math.Pi/2
		sh := Shark{
			ID:          i + 1,
			Kind:        spec.Kind,
			Length:      length,
			Heading:     heading,
			Speed:       30,
			Phase:       randRange(r, 0, Tau),
			Hunger:      randRange(r, 0.25, 0.75),
			OrbitDir:    orbitDir,
			OrbitRadius: orbitRadius,
			Target:      noTarget,
			Depth:       spec.Size,
		}
		seg := length / (SharkNodes - 1)
		for k := range SharkNodes {
			sh.Nodes[k*2] = x - math.Cos(heading)*seg*float64(k)
			sh.Nodes[k*2+1] = y - math.Sin(heading)*seg*float64(k)
		}
		w.Sharks = append(w.Sharks, sh)
	}
	// Smaller (farther) creatures draw first so the big ones swim over them.
	slices.SortStableFunc(w.Sharks, func(a, b Shark) int {
		switch {
		case a.Length < b.Length:
			return -1
		case a.Length > b.Length:
			return 1
		}
		return 0
	})

	total := int(Clamp(math.Round(W*H/4400), 85, 340))
	w.Fish = w.Fish[:0]
	for len(w.Fish) < total {
		species := pickSpecies(r)
		sp := FishSpecies[species]
		size := min(randInt(r, sp.SchoolSize[0], sp.SchoolSize[1]), total-len(w.Fish))
		millDir := randSign(r)
		cx := randRange(r, W*0.06, W*0.94)
		cy := randRange(r, H*0.08, H*0.92)
		// Mostly level headings: fish cruise sideways far more than up or down.
		dir := randRange(r, -0.5, 0.5)
		if r.Float64() >= 0.5 {
			dir += math.Pi
		}
		spread := math.Sqrt(float64(size))
		for range size {
			w.Fish = append(w.Fish, Fish{
				X:       cx + randRange(r, -1, 1)*w.sepX[species]*spread,
				Y:       cy + randRange(r, -1, 1)*w.sepY[species]*spread*0.6,
				VX:      math.Cos(dir) * sp.Cruise,
				VY:      math.Sin(dir) * sp.Cruise,
				Species: species,
				Alive:   true,
				Phase:   randRange(r, 0, Tau),
				Depth:   randRange(r, 0.55, 1),
				MillDir: millDir,
			})
		}
	}
	w.bucketNext = make([]int32, len(w.Fish))
}

func (w *World) Step(dt float64) {
	w.Time += dt
	w.rebuildHash()
	w.stepFish(dt)
	w.stepSharks(dt)
	w.respawn()
}

func (w *World) rebuildHash() {
	size := w.perception
	const pad = 2
	w.bucketCols = int(math.Ceil(w.Width/size)) + pad*2
	w.bucketRows = int(math.Ceil(w.Height/size)) + pad*2
	n := w.bucketCols * w.bucketRows
	if len(w.bucketHead) != n {
		w.bucketHead = make([]int32, n)
		w.bucketCount = make([]uint16, n)
	}
	for i := range w.bucketHead {
		w.bucketHead[i] = -1
		w.bucketCount[i] = 0
	}
	if len(w.bucketNext) != len(w.Fish) {
		w.bucketNext = make([]int32, len(w.Fish))
	}
	for i := range w.Fish {
		f := &w.Fish[i]
		if !f.Alive {
			continue
		}
		b := w.bucketOf(f.X, f.Y)
		w.bucketNext[i] = w.bucketHead[b]
		w.bucketHead[b] = int32(i)
		w.bucketCount[b]++
	}
}

func (w *World) bucketCoords(x, y float64) (int, int) {
	size := w.perception
	c := int(Clamp(math.Floor(x/size)+2, 0, float64(w.bucketCols-1)))
	r := int(Clamp(math.Floor(y/size)+2, 0, float64(w.bucketRows-1)))
	return c, r
}

func (w *World) bucketOf(x, y float64) int {
	c, r := w.bucketCoords(x, y)
	return r*w.bucketCols + c
}

func (w *World) stepFish(dt float64) {
	W, H, t := w.Width, w.Height, w.Time
	R := w.perception
	R2 := R * R
	cols, rows := w.bucketCols, w.bucketRows
	calmDecay := math.Exp(-dt * 0.7)
	contagion := math.Min(1, dt*6)

	for i := range w.Fish {
		f := &w.Fish[i]
		if !f.Alive {
			continue
		}
		sp := &FishSpecies[f.Species]
		// Alarmed fish want more room: the school flashes apart around a
		// shark and closes back up once the threat fades.
		spread := 1 + 1.8*f.Alarm
		sepX := w.sepX[f.Species] * spread
		sepY := w.sepY[f.Species] * spread

		speed := math.Hypot(f.VX, f.VY)
		if speed == 0 {
			speed = 1
		}
		hx, hy := f.VX/speed, f.VY/speed

		var ax, ay, sx, sy, avx, avy, cx, cy, neighbourAlarm float64
		n := 0

		bc, br := w.bucketCoords(f.X, f.Y)
		for r := max(0, br-1); r <= min(rows-1, br+1); r++ {
			for c := max(0, bc-1); c <= min(cols-1, bc+1); c++ {
				for j := w.bucketHead[r*cols+c]; j != -1; j = w.bucketNext[j] {
					if int(j) == i {
						continue
					}
					o := &w.Fish[j]
					dx, dy := o.X-f.X, o.Y-f.Y
					d2 := dx*dx + dy*dy
					if d2 >= R2 || d2 <= 1e-6 {
						continue
					}
					d := math.Sqrt(d2)
					// Blind zone directly behind the fish.
					if (dx*hx+dy*hy)/d <= blindCos {
						continue
					}
					// Elliptical personal space: wide along x, tight along y.
					qx, qy := dx/sepX, dy/sepY
					if q2 := qx*qx + qy*qy; q2 < 1 {
						q := math.Sqrt(q2)
						if q == 0 {
							q = 1e-3
						}
						k := (1 - q) / q
						sx -= qx * k
						sy -= qy * k
					}
					if o.Species == f.Species {
						avx += o.VX
						avy += o.VY
						cx += dx
						cy += dy
						n++
					} else if d < w.foreignSep {
						// Different species keep their own schools.
						k := ((1 - d/w.foreignSep) / d) * 0.6
						sx -= dx * k
						sy -= dy * k
					}
					neighbourAlarm = math.Max(neighbourAlarm, o.Alarm)
				}
			}
		}

		ax += sx * fishSeparationK
		ay += sy * fishSeparationK
		if n > 0 {
			fn := float64(n)
			alignK := fishAlignK * sp.Align
			cohesionK := fishCohesionK * sp.Cohesion * (1 - 0.8*f.Alarm)
			ax += (avx/fn-f.VX)*alignK + (cx/fn)*cohesionK
			ay += (avy/fn-f.VY)*alignK + (cy/fn)*cohesionK
			if sp.Mill > 0 {
				// Circle the local centre: the school spins into a bait ball.
				cl := math.Hypot(cx, cy)
				if cl == 0 {
					cl = 1
				}
				ax += (-cy / cl) * f.MillDir * sp.Mill * (1 - f.Alarm)
				ay += (cx / cl) * f.MillDir * sp.Mill * (1 - f.Alarm)
			}
		}

		// Predators. Distance is measured to the nearest point along the front
		// half of the body, so fish also give the flanks a wide berth.
		fear := 0.0
		for si := range w.Sharks {
			sh := &w.Sharks[si]
			fearR := sh.Length*0.6 + 60
			best := math.Inf(1)
			var bx, by float64
			for k := 0; k <= 12; k += 3 {
				nx, ny := sh.Nodes[k*2], sh.Nodes[k*2+1]
				if d2 := (f.X-nx)*(f.X-nx) + (f.Y-ny)*(f.Y-ny); d2 < best {
					best, bx, by = d2, nx, ny
				}
			}
			d := math.Sqrt(best)
			if d >= fearR || d < 1e-3 {
				continue
			}
			wt := 1 - d/fearR
			wt *= wt
			awayX, awayY := (f.X-bx)/d, (f.Y-by)/d
			// Fountain effect: break to whichever side of the path you're on.
			shx, shy := math.Cos(sh.Heading), math.Sin(sh.Heading)
			side := 1.0
			if shx*(f.Y-sh.Nodes[1])-shy*(f.X-sh.Nodes[0]) < 0 {
				side = -1
			}
			ax += (awayX*0.7 - shy*side*0.9) * fishFleeK * wt
			ay += (awayY*0.7 + shx*side*0.9) * fishFleeK * wt
			fear = math.Max(fear, math.Sqrt(wt))
		}

		// Alarm: direct threat sets it, neighbours pass it along (slightly
		// weaker each hop), and it fades on its own.
		alarm := math.Max(f.Alarm*calmDecay, fear)
		if relayed := neighbourAlarm * 0.86; relayed > alarm {
			alarm += (relayed - alarm) * contagion
		}
		f.Alarm = alarm

		const m = 36
		if f.X < m {
			ax += (m - f.X) * 7
		} else if f.X > W-m {
			ax -= (f.X - (W - m)) * 7
		}
		if f.Y < m {
			ay += (m - f.Y) * 7
		} else if f.Y > H-m {
			ay -= (f.Y - (H - m)) * 7
		}

		// Ambient current and a little individual restlessness.
		curX, curY := current(f.X, f.Y, t)
		ax += curX*0.5 + (w.rng.Float64()-0.5)*sp.Jitter
		ay += curY*0.5 + (w.rng.Float64()-0.5)*sp.Jitter
		// Swim level, except when bolting from danger.
		ay -= f.VY * sp.VerticalDrag * (1 - 0.7*f.Alarm)

		// Relax toward a preferred speed that rises with panic.
		preferred := sp.Cruise * (1 + 1.3*f.Alarm)
		ax += (preferred - speed) * hx * 1.6
		ay += (preferred - speed) * hy * 1.6

		before := math.Atan2(f.VY, f.VX)
		f.VX += ax * dt
		f.VY += ay * dt
		maxSpeed := sp.MaxSpeed * (1 + 1.8*f.Alarm)
		if s := math.Hypot(f.VX, f.VY); s > maxSpeed {
			f.VX *= maxSpeed / s
			f.VY *= maxSpeed / s
		} else if s < sp.MinSpeed && s > 1e-3 {
			f.VX *= sp.MinSpeed / s
			f.VY *= sp.MinSpeed / s
		}
		f.Turn += (AngleDiff(before, math.Atan2(f.VY, f.VX))/dt - f.Turn) * math.Min(1, dt*8)
		f.Phase += dt * (7 + math.Hypot(f.VX, f.VY)*0.12)
		f.X = Clamp(f.X+f.VX*dt, -60, W+60)
		f.Y = Clamp(f.Y+f.VY*dt, -60, H+60)
	}
}

func (w *World) stepSharks(dt float64) {
	W, H, t := w.Width, w.Height, w.Time
	minSide := math.Min(W, H)
	cxW, cyW := W/2, H/2

	for si := range w.Sharks {
		sh := &w.Sharks[si]
		L := sh.Length
		hx, hy := sh.Nodes[0], sh.Nodes[1]
		hunts := sh.Hunts()

		if hunts {
			sh.Hunger = math.Min(1, sh.Hunger+dt*0.03)
		}

		dx, dy := math.Cos(sh.Heading), math.Sin(sh.Heading)

		// Wander: a slowly drifting offset on a circle ahead of the snout.
		sh.Wander = Clamp(sh.Wander+(w.rng.Float64()-0.5)*dt*3.2, -1.1, 1.1)
		dx += math.Cos(sh.Heading+sh.Wander) * 0.45
		dy += math.Sin(sh.Heading+sh.Wander) * 0.45

		hunting := false
		closing := 0.0
		if hunts && t >= sh.RetargetAt {
			sh.Target = w.findPrey(sh)
			sh.RetargetAt = t + 0.45
		}
		if hunts && sh.Target != noTarget && w.Fish[sh.Target].Alive && sh.Hunger > 0.3 {
			// Pursuit: aim at where the fish is about to be.
			tf := &w.Fish[sh.Target]
			tx := tf.X + tf.VX*0.35 - hx
			ty := tf.Y + tf.VY*0.35 - hy
			d := math.Hypot(tx, ty)
			if d == 0 {
				d = 1
			}
			wt := 1.4 + 2.6*sh.Hunger
			dx += (tx / d) * wt
			dy += (ty / d) * wt
			hunting = true
			closing = Clamp(1-d/(L*0.9), 0, 1)
		} else {
			// Patrol: a lazy circuit around the middle of the screen.
			rx := (hx - cxW) / (W / minSide)
			ry := hy - cyW
			rr := math.Hypot(rx, ry)
			if rr == 0 {
				rr = 1
			}
			radius := sh.OrbitRadius * (1 + 0.15*math.Sin(t*0.05+float64(sh.ID)))
			radial := Clamp((radius-rr)/radius, -1, 1)
			dx += ((-ry/rr)*sh.OrbitDir + (rx/rr)*radial*0.9) * 0.55
			dy += ((rx/rr)*sh.OrbitDir + (ry/rr)*radial*0.9) * 0.55
		}

		for oi := range w.Sharks {
			if oi == si {
				continue
			}
			other := &w.Sharks[oi]
			for k := 0; k < SharkNodes; k += 4 {
				ox := hx - other.Nodes[k*2]
				oy := hy - other.Nodes[k*2+1]
				d := math.Hypot(ox, oy)
				rng := (L + other.Length) * 0.45
				if d < rng && d > 1e-3 {
					wt := (1 - d/rng) * 3
					dx += (ox / d) * wt
					dy += (oy / d) * wt
				}
			}
		}

		// Turn back once the snout leaves the frame, so bodies dip off-screen
		// now and then but never wander away for good.
		over := max(-hx, hx-W, -hy, hy-H)
		if over > -L*0.05 {
			wt := Clamp((over+L*0.05)/(L*0.35), 0, 1) * 3.5
			toX, toY := cxW-hx, cyW-hy
			d := math.Hypot(toX, toY)
			if d == 0 {
				d = 1
			}
			dx += (toX / d) * wt
			dy += (toY / d) * wt
		}

		desired := math.Atan2(dy, dx)
		// No pivoting on the spot: the turning circle grows with speed.
		agility := 0.8
		if hunting {
			agility = 1.7 + closing
		}
		agility = math.Min(agility, sh.Speed/(L*0.22))
		maxTurn := agility * dt
		turned := Clamp(AngleDiff(sh.Heading, desired), -maxTurn, maxTurn)
		sh.Heading += turned
		sh.Turn += (turned/dt - sh.Turn) * math.Min(1, dt*3)

		cruise := 30 + L*0.04
		if !hunts {
			cruise *= 0.55
		}
		targetSpeed := cruise
		if hunting {
			targetSpeed = cruise * (1.45 + closing*1.5)
		}
		sh.Speed += (targetSpeed - sh.Speed) * math.Min(1, dt*1.1)
		sh.Phase += dt * (2 + sh.Speed/24)

		sh.Nodes[0] += math.Cos(sh.Heading) * sh.Speed * dt
		sh.Nodes[1] += math.Sin(sh.Heading) * sh.Speed * dt

		// Follow-the-leader spine, with a stiffness limit per joint so the
		// body bends into a C at most and never folds back on itself.
		seg := L / (SharkNodes - 1)
		const maxBend = 0.16
		prevAngle := sh.Heading + math.Pi
		for k := 1; k < SharkNodes; k++ {
			px, py := sh.Nodes[(k-1)*2], sh.Nodes[(k-1)*2+1]
			angle := math.Atan2(sh.Nodes[k*2+1]-py, sh.Nodes[k*2]-px)
			angle = prevAngle + Clamp(AngleDiff(prevAngle, angle), -maxBend, maxBend)
			sh.Nodes[k*2] = px + math.Cos(angle)*seg
			sh.Nodes[k*2+1] = py + math.Sin(angle)*seg
			prevAngle = angle
		}

		if hunts && sh.Hunger > 0.15 {
			mouthR := L*0.05 + 8
			mx, my := sh.Nodes[0], sh.Nodes[1]
			for fi := range w.Fish {
				f := &w.Fish[fi]
				if !f.Alive {
					continue
				}
				if (f.X-mx)*(f.X-mx)+(f.Y-my)*(f.Y-my) < mouthR*mouthR {
					f.Alive = false
					f.RespawnAt = t + randRange(w.rng, 3, 8)
					sh.Hunger = math.Max(0, sh.Hunger-0.16)
					w.Events = append(w.Events, Event{X: f.X, Y: f.Y})
				}
			}
		}
	}
}

// findPrey picks a fish in the forward cone, preferring close fish in sparse
// buckets (stragglers) over fish buried in a dense school, since dense schools
// confuse predators.
func (w *World) findPrey(sh *Shark) int {
	rng := sh.Length*0.9 + 160
	hx, hy := sh.Nodes[0], sh.Nodes[1]
	fx, fy := math.Cos(sh.Heading), math.Sin(sh.Heading)
	best := noTarget
	bestScore := math.Inf(1)
	currentScore := math.Inf(1)

	for i := range w.Fish {
		f := &w.Fish[i]
		if !f.Alive {
			continue
		}
		dx, dy := f.X-hx, f.Y-hy
		d := math.Hypot(dx, dy)
		if d > rng || d < 1e-3 {
			continue
		}
		cos := (dx*fx + dy*fy) / d
		if cos < 0.2 {
			continue
		}
		crowd := float64(w.bucketCount[w.bucketOf(f.X, f.Y)])
		score := d * (1 + (1-cos)*0.8) * (1 + crowd*0.07)
		if score < bestScore {
			bestScore = score
			best = i
		}
		if i == sh.Target {
			currentScore = score
		}
	}
	// Stick with the current target unless something is clearly better.
	if sh.Target != noTarget && currentScore < bestScore*1.3 {
		return sh.Target
	}
	return best
}

func (w *World) respawn() {
	r := w.rng
	t, W, H := w.Time, w.Width, w.Height
	var due []int
	for i := range w.Fish {
		if !w.Fish[i].Alive && w.Fish[i].RespawnAt <= t {
			due = append(due, i)
		}
	}
	if len(due) == 0 {
		return
	}
	if len(due) < 6 && t-w.lastSpawn < 9 {
		return
	}
	w.lastSpawn = t

	// A new school swims in from a random edge.
	side := randInt(r, 0, 3)
	along := randRange(r, 0.15, 0.85)
	x, y := along*W, along*H
	switch side {
	case 0:
		x = -20
	case 1:
		x = W + 20
	case 2:
		y = -20
	case 3:
		y = H + 20
	}
	inward := math.Atan2(H/2-y, W/2-x) + randRange(r, -0.5, 0.5)
	species := pickSpecies(r)
	sp := FishSpecies[species]
	millDir := randSign(r)
	if len(due) > sp.SchoolSize[1] {
		due = due[:sp.SchoolSize[1]]
	}
	for _, i := range due {
		f := &w.Fish[i]
		f.Alive = true
		f.Species = species
		f.Alarm = 0
		f.Turn = 0
		f.MillDir = millDir
		f.X = x + randRange(r, -1, 1)*w.perception
		f.Y = y + randRange(r, -1, 1)*w.perception
		f.VX = math.Cos(inward) * sp.Cruise * 1.4
		f.VY = math.Sin(inward) * sp.Cruise * 1.4
	}
}
