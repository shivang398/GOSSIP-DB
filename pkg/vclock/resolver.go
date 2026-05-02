package vclock

// ResolvableValue is a minimal interface that both store.Value and any future
// CRDT type must satisfy to be used with the Reconcile function.
// This breaks the import cycle between vclock ↔ store.
type ResolvableValue interface {
	GetTimestamp() int64
	GetVersion() map[string]int
}

// ConflictResolution is the strategy used to resolve concurrent writes.
type ConflictResolution int

const (
	// LWWResolver uses the physical timestamp as the tiebreaker for concurrent writes.
	LWWResolver ConflictResolution = iota
)

// Resolver is the interface for conflict resolution strategies.
type Resolver[T ResolvableValue] interface {
	Resolve(local, incoming T) T
}

// LWWGeneric is the Last-Write-Wins resolver that works with any ResolvableValue.
type LWWGeneric[T ResolvableValue] struct{}

func (LWWGeneric[T]) Resolve(local, incoming T) T {
	if incoming.GetTimestamp() > local.GetTimestamp() {
		return incoming
	}
	return local
}

// ReconcileVC determines the causal winner between two values using their vector clocks.
// Returns the winning value and a bool indicating whether a conflict was detected.
func ReconcileVC[T ResolvableValue](local, incoming T, resolver Resolver[T]) (T, bool) {
	localVC := VClock(local.GetVersion())
	incomingVC := VClock(incoming.GetVersion())

	switch incomingVC.Compare(localVC) {
	case After:
		return incoming, false
	case Before, Equal:
		return local, false
	default: // Concurrent
		winner := resolver.Resolve(local, incoming)
		return winner, true
	}
}

