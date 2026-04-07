package ir

// unwindLeftSpine iteratively descends the left spine of App nodes,
// treating TyApp.Expr and TyLam.Body as transparent wrappers. It returns
// the spine head — the first non-{App,TyApp,TyLam} node encountered —
// and the right-side arguments collected from each App along the way.
//
// Rights are in unwind order: rights[0] is the Arg of the outermost App,
// rights[len-1] is the Arg of the innermost App. Callers flush rights in
// reverse (index len-1 down to 0) to preserve left-to-right evaluation
// order for operations like free-variable computation.
//
// This helper exists so that free-variable traversal, index assignment,
// and any future spine-walking passes share a single canonical unwind
// skeleton. It intentionally does NOT visit the wrapper nodes; passes
// that need to visit every node on the spine (such as Walk in walk.go)
// have their own implementation because that semantic differs.
func unwindLeftSpine(cur Core) (head Core, rights []Core) {
	for {
		switch n := cur.(type) {
		case *App:
			rights = append(rights, n.Arg)
			cur = n.Fun
			continue
		case *TyApp:
			cur = n.Expr
			continue
		case *TyLam:
			cur = n.Body
			continue
		default:
			return cur, rights
		}
	}
}
