package vclock

// Relation represents the causal relationship between two vector clocks.
type Relation int

const (
	Before     Relation = iota // vc1 happened-before vc2
	After                      // vc1 happened-after vc2
	Equal                      // vc1 == vc2
	Concurrent                 // vc1 and vc2 are concurrent (conflict!)
)

// VClock is a vector clock: a map from node ID to version counter.
type VClock map[string]int

// New creates a fresh, empty vector clock.
func New() VClock {
	return make(VClock)
}

// Increment increments the clock for the given node.
func (vc VClock) Increment(nodeID string) VClock {
	result := vc.Copy()
	result[nodeID]++
	return result
}

// Merge returns a new VClock that is the element-wise maximum of vc and other.
// This is the standard merge/sync operation during replication.
func (vc VClock) Merge(other VClock) VClock {
	result := vc.Copy()
	for nodeID, otherVal := range other {
		if localVal, exists := result[nodeID]; !exists || otherVal > localVal {
			result[nodeID] = otherVal
		}
	}
	return result
}

// Compare determines the causal relationship between vc and other.
func (vc VClock) Compare(other VClock) Relation {
	vcDominates := false
	otherDominates := false

	// Check all keys from vc
	for nodeID, vcVal := range vc {
		otherVal := other[nodeID]
		if vcVal > otherVal {
			vcDominates = true
		} else if vcVal < otherVal {
			otherDominates = true
		}
	}

	// Check keys in other that may not be in vc
	for nodeID, otherVal := range other {
		if _, exists := vc[nodeID]; !exists && otherVal > 0 {
			otherDominates = true
		}
	}

	switch {
	case vcDominates && !otherDominates:
		return After // vc is strictly newer than other
	case otherDominates && !vcDominates:
		return Before // vc is strictly older than other
	case !vcDominates && !otherDominates:
		return Equal
	default:
		return Concurrent // both have entries the other doesn't — conflict!
	}
}

// Copy creates a deep copy of the vector clock.
func (vc VClock) Copy() VClock {
	result := make(VClock, len(vc))
	for k, v := range vc {
		result[k] = v
	}
	return result
}
