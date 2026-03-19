package check

// Worklist is a FIFO queue of constraints to be processed by the solver.
// New constraints are pushed to the back; kicked-out constraints are
// inserted at the front for priority re-processing.
type Worklist struct {
	items []Ct
}

// Push appends a constraint to the back of the worklist.
func (w *Worklist) Push(ct Ct) {
	w.items = append(w.items, ct)
}

// PushFront inserts constraints at the front of the worklist.
// Used for kicked-out constraints that need priority re-processing.
func (w *Worklist) PushFront(cts ...Ct) {
	if len(cts) == 0 {
		return
	}
	w.items = append(cts, w.items...)
}

// Pop removes and returns the first constraint from the worklist.
func (w *Worklist) Pop() (Ct, bool) {
	if len(w.items) == 0 {
		return nil, false
	}
	ct := w.items[0]
	w.items[0] = nil // avoid holding reference
	w.items = w.items[1:]
	return ct, true
}

// Len returns the number of constraints in the worklist.
func (w *Worklist) Len() int {
	return len(w.items)
}

// Empty reports whether the worklist has no constraints.
func (w *Worklist) Empty() bool {
	return len(w.items) == 0
}

// Reset clears all constraints from the worklist.
func (w *Worklist) Reset() {
	clear(w.items)
	w.items = w.items[:0]
}
