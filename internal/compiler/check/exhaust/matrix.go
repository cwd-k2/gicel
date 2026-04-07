package exhaust

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// --- Pattern representation ---
//
// Reference: Luc Maranget, "Warnings for Pattern Matching" (JFP 2007).

// pat is the internal pattern representation for the algorithm.
type pat interface{ patTag() }

type pWild struct{} // _
type pCon struct {
	con   string
	arity int
	args  []pat
}                                            // C p1 ... pn
type pRecord struct{ fields map[string]pat } // { l1: p1, ... }
type pLit struct{ value any }                // 42, "hello", 'x'

func (pWild) patTag()   {}
func (pCon) patTag()    {}
func (pRecord) patTag() {}
func (pLit) patTag()    {}

type patVec []pat
type patMatrix []patVec

// conSig is a constructor's name and arity.
type conSig struct {
	name  string
	arity int
}

// ---- Convert ir.Pattern → pat ----

func coreToPat(p ir.Pattern) pat {
	switch pp := p.(type) {
	case *ir.PVar:
		return pWild{}
	case *ir.PWild:
		return pWild{}
	case *ir.PCon:
		args := make([]pat, len(pp.Args))
		for i, a := range pp.Args {
			args[i] = coreToPat(a)
		}
		return pCon{con: pp.Con, arity: len(pp.Args), args: args}
	case *ir.PRecord:
		fields := make(map[string]pat, len(pp.Fields))
		for _, f := range pp.Fields {
			fields[f.Label] = coreToPat(f.Pattern)
		}
		return pRecord{fields: fields}
	case *ir.PLit:
		return pLit{value: pp.Value}
	default:
		return pWild{}
	}
}

// ---- Specialize S(c, P) ----
// Keep rows whose column 0 matches constructor c, expanding sub-patterns.
// Wildcard rows are expanded to c.arity wildcards.

func specialize(mx patMatrix, con string, arity int) patMatrix {
	var result patMatrix
	for _, row := range mx {
		if len(row) == 0 {
			continue
		}
		switch p := row[0].(type) {
		case pCon:
			if p.con == con {
				newRow := make(patVec, 0, arity+len(row)-1)
				newRow = append(newRow, p.args...)
				newRow = append(newRow, row[1:]...)
				result = append(result, newRow)
			}
		case pWild:
			newRow := make(patVec, 0, arity+len(row)-1)
			for range arity {
				newRow = append(newRow, pWild{})
			}
			newRow = append(newRow, row[1:]...)
			result = append(result, newRow)
		}
	}
	return result
}

// ---- Default D(P) ----
// Keep rows whose column 0 is a wildcard/variable, dropping column 0.

func defaultMatrix(mx patMatrix) patMatrix {
	var result patMatrix
	for _, row := range mx {
		if len(row) == 0 {
			continue
		}
		if _, ok := row[0].(pWild); ok {
			result = append(result, row[1:])
		}
	}
	return result
}

// ---- Literal specialize ----

// specializeLit keeps rows whose column 0 matches a specific literal value.
// Wildcard rows are included (literal always matches subset of wildcard).
func specializeLit(mx patMatrix, val any) patMatrix {
	var result patMatrix
	for _, row := range mx {
		if len(row) == 0 {
			continue
		}
		switch p := row[0].(type) {
		case pLit:
			if p.value == val {
				result = append(result, row[1:])
			}
		case pWild:
			result = append(result, row[1:])
		}
	}
	return result
}

// columnHeadLits returns the set of literal values in column 0.
func columnHeadLits(mx patMatrix) map[any]bool {
	result := map[any]bool{}
	for _, row := range mx {
		if len(row) == 0 {
			continue
		}
		if lp, ok := row[0].(pLit); ok {
			result[lp.value] = true
		}
	}
	return result
}

// ---- Record specialize/default ----

// allRecordLabels collects all labels mentioned in column 0 of the matrix.
func allRecordLabels(mx patMatrix) []string {
	set := map[string]struct{}{}
	for _, row := range mx {
		if len(row) == 0 {
			continue
		}
		if p, ok := row[0].(pRecord); ok {
			for l := range p.fields {
				set[l] = struct{}{}
			}
		}
	}
	labels := make([]string, 0, len(set))
	for l := range set {
		labels = append(labels, l)
	}
	sort.Strings(labels)
	return labels
}

// specializeRecord expands column 0 into per-label columns.
func specializeRecord(mx patMatrix, labels []string) patMatrix {
	var result patMatrix
	for _, row := range mx {
		if len(row) == 0 {
			continue
		}
		switch p := row[0].(type) {
		case pRecord:
			newRow := make(patVec, 0, len(labels)+len(row)-1)
			for _, l := range labels {
				if sub, ok := p.fields[l]; ok {
					newRow = append(newRow, sub)
				} else {
					newRow = append(newRow, pWild{})
				}
			}
			newRow = append(newRow, row[1:]...)
			result = append(result, newRow)
		case pWild:
			newRow := make(patVec, 0, len(labels)+len(row)-1)
			for range labels {
				newRow = append(newRow, pWild{})
			}
			newRow = append(newRow, row[1:]...)
			result = append(result, newRow)
		}
	}
	return result
}

// ---- Helpers ----

// nilTypesWithTail builds a type slice of n nil entries followed by tail.
// Used to represent unknown sub-pattern types for record fields.
func nilTypesWithTail(n int, tail []types.Type) []types.Type {
	result := make([]types.Type, n, n+len(tail))
	return append(result, tail...)
}

// makeWildcardVec builds a pattern vector of n wildcards followed by tail.
func makeWildcardVec(n int, tail patVec) patVec {
	result := make(patVec, 0, n+len(tail))
	for range n {
		result = append(result, pWild{})
	}
	return append(result, tail...)
}

// columnHeadCons returns the set of constructor names in column 0.
func columnHeadCons(mx patMatrix) map[string]int {
	result := map[string]int{}
	for _, row := range mx {
		if len(row) == 0 {
			continue
		}
		if cp, ok := row[0].(pCon); ok {
			result[cp.con] = cp.arity
		}
	}
	return result
}

// reconstructCon builds a witness pattern from the recursive useful result.
func reconstructCon(con string, arity int, inner pat) pat {
	// Extract sub-patterns from the witness (best-effort).
	args := make([]pat, arity)
	for i := range args {
		args[i] = pWild{}
	}
	// If the inner result gives us sub-pattern info, propagate it.
	if arity > 0 {
		if ic, ok := inner.(pCon); ok && ic.con == con {
			copy(args, ic.args)
		}
	}
	return pCon{con: con, arity: arity, args: args}
}

// formatWitness renders a witness pattern for error messages.
func formatWitness(p pat) string {
	switch pp := p.(type) {
	case pCon:
		if len(pp.args) == 0 {
			return pp.con
		}
		args := make([]string, len(pp.args))
		for i, a := range pp.args {
			s := formatWitness(a)
			// Wrap nested constructors with args in parens.
			if ac, ok := a.(pCon); ok && len(ac.args) > 0 {
				s = "(" + s + ")"
			}
			args[i] = s
		}
		return pp.con + " " + strings.Join(args, " ")
	case pWild:
		return "_"
	case pLit:
		return fmt.Sprintf("%v", pp.value)
	case pRecord:
		if len(pp.fields) == 0 {
			return "{}"
		}
		var parts []string
		for l, v := range pp.fields {
			parts = append(parts, l+": "+formatWitness(v))
		}
		sort.Strings(parts)
		return "{ " + strings.Join(parts, ", ") + " }"
	default:
		return "_"
	}
}

// headTyCon extracts the outermost type constructor name from a type.
func headTyCon(ty types.Type) string {
	switch t := ty.(type) {
	case *types.TyCon:
		return t.Name
	case *types.TyApp:
		return headTyCon(t.Fun)
	case *types.TyFamilyApp:
		// Data families: the family app itself acts as a data type name.
		// Return the mangled name if we can determine it.
		return ""
	default:
		return ""
	}
}

// hasRecordPats returns true if column 0 of the matrix has any record patterns.
func hasRecordPats(mx patMatrix) bool {
	for _, row := range mx {
		if len(row) > 0 {
			if _, ok := row[0].(pRecord); ok {
				return true
			}
		}
	}
	return false
}
