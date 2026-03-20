package check

// Worklist is a FIFO queue of constraints to be processed by the solver.
// New constraints are pushed to the back; kicked-out constraints are
// inserted at the front for priority re-processing.
//
// Internally a two-buffer deque: front (reversed stack for priority items)
// and back (FIFO queue with a read cursor). All operations are amortized O(1).
type Worklist struct {
	front    []Ct // reversed stack: kicked-out items, pop from end
	back     []Ct // normal FIFO queue
	backHead int  // read cursor into back
}

// Push appends a constraint to the back of the worklist.
func (w *Worklist) Push(ct Ct) {
	w.back = append(w.back, ct)
}

// PushFront inserts constraints at the front of the worklist.
// Used for kicked-out constraints that need priority re-processing.
// Items are stored in reverse order so Pop returns them in original order.
func (w *Worklist) PushFront(cts ...Ct) {
	if len(cts) == 0 {
		return
	}
	// Append in reverse so that Pop (which pops from end) yields FIFO order.
	for i := len(cts) - 1; i >= 0; i-- {
		w.front = append(w.front, cts[i])
	}
}

// Pop removes and returns the next constraint from the worklist.
// Priority (front) items are returned first, then back (FIFO) items.
func (w *Worklist) Pop() (Ct, bool) {
	// Drain front stack first (LIFO = original FIFO of PushFront batch).
	if n := len(w.front); n > 0 {
		ct := w.front[n-1]
		w.front[n-1] = nil
		w.front = w.front[:n-1]
		return ct, true
	}
	// Then drain back queue via cursor.
	if w.backHead < len(w.back) {
		ct := w.back[w.backHead]
		w.back[w.backHead] = nil
		w.backHead++
		return ct, true
	}
	return nil, false
}

// Len returns the number of constraints in the worklist.
func (w *Worklist) Len() int {
	return len(w.front) + len(w.back) - w.backHead
}

// Empty reports whether the worklist has no constraints.
func (w *Worklist) Empty() bool {
	return len(w.front) == 0 && w.backHead >= len(w.back)
}

// Reset clears all constraints from the worklist.
func (w *Worklist) Reset() {
	clear(w.front)
	w.front = w.front[:0]
	clear(w.back)
	w.back = w.back[:0]
	w.backHead = 0
}

// Drain returns all pending items in FIFO order and clears the worklist.
// Used by withDeferredScope to save/restore constraint scopes.
func (w *Worklist) Drain() []Ct {
	n := w.Len()
	if n == 0 {
		w.Reset()
		return nil
	}
	result := make([]Ct, 0, n)
	// Front items: reverse to restore FIFO order.
	for i := len(w.front) - 1; i >= 0; i-- {
		result = append(result, w.front[i])
	}
	// Back items: from cursor to end.
	result = append(result, w.back[w.backHead:]...)
	w.Reset()
	return result
}

// Load replaces the worklist contents with the given items.
// Used by withDeferredScope to restore a saved scope.
func (w *Worklist) Load(cts []Ct) {
	clear(w.front)
	w.front = w.front[:0]
	w.back = cts
	w.backHead = 0
}
