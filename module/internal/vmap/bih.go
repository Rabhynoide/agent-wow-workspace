package vmap

import "math"

// bih is a Bounding Interval Hierarchy as stored in the vmap files (BIH::readFromFile). Each tree
// node is 3 words: header (bits 30-31 axis, 3 = leaf; bit 29 BVH2 node; low bits child offset) and two
// clip planes as float bits (or, for a leaf, the object count in word 1).
type bih struct {
	lo, hi  [3]float32
	tree    []uint32
	objects []uint32
}

const bihStack = 64

// intersectRay walks the hierarchy front to back (BIH::intersectRay). For each object reached, hit is
// called with the current maximum distance, which it shortens on a closer hit; it returns whether the
// object was hit, to stop early when stopAtFirst is set.
func (b *bih) intersectRay(org, dir [3]float32, maxDist *float32, stopAtFirst bool, hit func(obj uint32, maxDist *float32) bool) {
	if len(b.tree) == 0 {
		return
	}
	intervalMin, intervalMax := float32(-1), float32(-1)
	var invDir [3]float32
	for i := 0; i < 3; i++ {
		invDir[i] = 1 / dir[i] // ±Inf on a zero component, as in C++
		if math.Abs(float64(dir[i])) > 1e-6 {
			t1 := (b.lo[i] - org[i]) * invDir[i]
			t2 := (b.hi[i] - org[i]) * invDir[i]
			if t1 > t2 {
				t1, t2 = t2, t1
			}
			if t1 > intervalMin {
				intervalMin = t1
			}
			if t2 < intervalMax || intervalMax < 0 {
				intervalMax = t2
			}
			if intervalMax <= 0 || intervalMin >= *maxDist {
				return
			}
		}
	}
	if intervalMin > intervalMax {
		return
	}
	intervalMin = max(intervalMin, 0)
	intervalMax = min(intervalMax, *maxDist)

	var offsetFront, offsetBack, offsetFront3, offsetBack3 [3]int
	for i := 0; i < 3; i++ {
		offsetFront[i] = int(math.Float32bits(dir[i]) >> 31)
		offsetBack[i] = offsetFront[i] ^ 1
		offsetFront3[i] = offsetFront[i] * 3
		offsetBack3[i] = offsetBack[i] * 3
		offsetFront[i]++
		offsetBack[i]++
	}
	type stackNode struct {
		node        int
		tnear, tfar float32
	}
	var stack [bihStack]stackNode
	stackPos, node := 0, 0
	plane := func(i int) float32 { return math.Float32frombits(b.tree[i]) }
	for {
		for {
			if node+1 >= len(b.tree) {
				return // corrupt tree
			}
			tn := b.tree[node]
			axis := (tn & (3 << 30)) >> 30
			bvh2 := tn&(1<<29) != 0
			offset := int(tn &^ (7 << 29))
			if axis < 3 && node+2 >= len(b.tree) {
				return
			}
			if !bvh2 {
				if axis < 3 {
					tf := (plane(node+offsetFront[axis]) - org[axis]) * invDir[axis]
					tb := (plane(node+offsetBack[axis]) - org[axis]) * invDir[axis]
					if tf < intervalMin && tb > intervalMax { // between the clip zones
						break
					}
					back := offset + offsetBack3[axis]
					node = back
					if tf < intervalMin { // far node only
						if tb >= intervalMin {
							intervalMin = tb
						}
						continue
					}
					node = offset + offsetFront3[axis] // front
					if tb > intervalMax {              // near node only
						if tf <= intervalMax {
							intervalMax = tf
						}
						continue
					}
					if stackPos >= bihStack {
						return
					}
					tnear := intervalMin
					if tb >= intervalMin {
						tnear = tb
					}
					stack[stackPos] = stackNode{back, tnear, intervalMax}
					stackPos++
					if tf <= intervalMax {
						intervalMax = tf
					}
					continue
				}
				// leaf: test its objects
				n := int(b.tree[node+1])
				for ; n > 0; n, offset = n-1, offset+1 {
					if offset < len(b.objects) && hit(b.objects[offset], maxDist) && stopAtFirst {
						return
					}
				}
				break
			}
			if axis > 2 {
				return
			}
			tf := (plane(node+offsetFront[axis]) - org[axis]) * invDir[axis]
			tb := (plane(node+offsetBack[axis]) - org[axis]) * invDir[axis]
			node = offset
			if tf >= intervalMin {
				intervalMin = tf
			}
			if tb <= intervalMax {
				intervalMax = tb
			}
			if intervalMin > intervalMax {
				break
			}
		}
		for { // pop
			if stackPos == 0 {
				return
			}
			stackPos--
			intervalMin = stack[stackPos].tnear
			if *maxDist < intervalMin {
				continue
			}
			node = stack[stackPos].node
			intervalMax = stack[stackPos].tfar
			break
		}
	}
}
