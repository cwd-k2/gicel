package check

import "testing"

func TestDoBlockPrePostThreading(t *testing.T) {
	source := `
data Unit = Unit
consume :: Computation { handle : Unit } {} Unit
consume := assumption
main :: Computation { handle : Unit } {} Unit
main := do { x <- consume; pure x }
`
	checkSource(t, source, nil)
}

func TestDoBlockPrePostMultiBind(t *testing.T) {
	source := `
data Unit = Unit
step1 :: Computation { a : Unit } { b : Unit } Unit
step1 := assumption
step2 :: Computation { b : Unit } {} Unit
step2 := assumption
main :: Computation { a : Unit } {} Unit
main := do { x <- step1; step2 }
`
	checkSource(t, source, nil)
}

func TestDoBlockPrePostWithMult(t *testing.T) {
	source := `
data Mult = Unrestricted | Affine | Linear
data Unit = Unit
consume :: Computation { handle : Unit @Linear } {} Unit
consume := assumption
main :: Computation { handle : Unit @Linear } {} Unit
main := do { x <- consume; pure x }
`
	checkSource(t, source, nil)
}

func TestDoBlockPrePostChainWithMult(t *testing.T) {
	source := `
data Mult = Unrestricted | Affine | Linear
data Unit = Unit
open :: Computation {} { handle : Unit @Linear } Unit
open := assumption
use :: Computation { handle : Unit @Linear } { handle : Unit @Linear } Unit
use := assumption
close :: Computation { handle : Unit @Linear } {} Unit
close := assumption
main :: Computation {} {} Unit
main := do { open; use; close }
`
	checkSource(t, source, nil)
}
