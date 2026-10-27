package safe

import (
	"math"

	statepb "wowagent/gen/statev1"
)

// AddMargin: yards added to an aggro radius for an add (we fight around the target, not on it).
const AddMargin = 2

func dist3D(a, b *statepb.Obj) float32 {
	return float32(math.Sqrt(float64((a.X-b.X)*(a.X-b.X) + (a.Y-b.Y)*(a.Y-b.Y) + (a.Z-b.Z)*(a.Z-b.Z))))
}

// Adds lists the hostile monsters that would join a fight at the target's spot: not engaged with
// anyone, and either within their aggro radius of it (3D, at most ZAggro yards above or below:
// Creature::CanStartAttack) or called by the target or by another add (within the assistance range,
// 3D, repeated: the whole chain of a camp). A kobold on
// the floor above in Fargodeep mine is no add (it counted as one, 2D, and crowded the pulls).
func Adds(t *statepb.Obj, hostiles []*statepb.Obj, rangeOf func(*statepb.Obj) float32) []*statepb.Obj {
	var out []*statepb.Obj
	for _, h := range hostiles {
		if h.Guid == t.Guid || h.Target != 0 {
			continue
		}
		d := dist3D(h, t)
		aggro := math.Abs(float64(h.Z-t.Z)) <= ZAggro && d <= rangeOf(h)+AddMargin
		if aggro || d <= AssistRange+1 {
			out = append(out, h)
		}
	}
	// Each monster in the fight calls for help again every 3 s (Creature::CallAssistance,
	// CreatureFamilyAssistancePeriod), within the assistance range of where it stands: a camp joins
	// link by link (the wolf pack of 7 and the Fargodeep kobolds of the runs of 2026-10-26).
	in := map[uint64]bool{t.Guid: true}
	for _, o := range out {
		in[o.Guid] = true
	}
	for grew := true; grew; {
		grew = false
		for _, h := range hostiles {
			// On the fight's floor only: the server wants a line of sight between them, which floors
			// block (the stacked galleries of Fargodeep).
			if in[h.Guid] || h.Target != 0 || math.Abs(float64(h.Z-t.Z)) > ZAggro {
				continue
			}
			for _, m := range out {
				if dist3D(h, m) <= AssistRange+1 {
					out = append(out, h)
					in[h.Guid], grew = true, true
					break
				}
			}
		}
	}
	return out
}
