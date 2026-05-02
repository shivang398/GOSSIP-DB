package vclock

import "testing"

func TestIncrement(t *testing.T) {
	vc := New()
	vc = vc.Increment("node1")
	vc = vc.Increment("node1")
	vc = vc.Increment("node2")

	if vc["node1"] != 2 {
		t.Errorf("expected node1=2, got %d", vc["node1"])
	}
	if vc["node2"] != 1 {
		t.Errorf("expected node2=1, got %d", vc["node2"])
	}
}

func TestMerge(t *testing.T) {
	a := VClock{"node1": 3, "node2": 1}
	b := VClock{"node1": 1, "node2": 5, "node3": 2}

	merged := a.Merge(b)

	if merged["node1"] != 3 { t.Errorf("expected node1=3, got %d", merged["node1"]) }
	if merged["node2"] != 5 { t.Errorf("expected node2=5, got %d", merged["node2"]) }
	if merged["node3"] != 2 { t.Errorf("expected node3=2, got %d", merged["node3"]) }
}

func TestCompare_Before(t *testing.T) {
	a := VClock{"node1": 1}
	b := VClock{"node1": 2}
	if a.Compare(b) != Before {
		t.Error("expected a to be Before b")
	}
}

func TestCompare_After(t *testing.T) {
	a := VClock{"node1": 3}
	b := VClock{"node1": 1}
	if a.Compare(b) != After {
		t.Error("expected a to be After b")
	}
}

func TestCompare_Equal(t *testing.T) {
	a := VClock{"node1": 2, "node2": 1}
	b := VClock{"node1": 2, "node2": 1}
	if a.Compare(b) != Equal {
		t.Error("expected a to be Equal to b")
	}
}

func TestCompare_Concurrent(t *testing.T) {
	// node1 wrote a, node2 wrote b — neither knew about the other
	a := VClock{"node1": 1}
	b := VClock{"node2": 1}
	if a.Compare(b) != Concurrent {
		t.Error("expected a and b to be Concurrent")
	}
}

func TestCopy(t *testing.T) {
	original := VClock{"node1": 5}
	copied := original.Copy()
	copied["node1"] = 999 // mutate copy

	if original["node1"] != 5 {
		t.Error("copy mutated the original")
	}
}
