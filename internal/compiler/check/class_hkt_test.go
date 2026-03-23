package check

import "testing"

// =============================================================================
// Poly-kinded class declarations
// =============================================================================

func TestPolyKindedClassDecl(t *testing.T) {
	// class Functor (f: k -> Type) with implicit kind variable k
	source := `
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }

form Functor := \(f: k -> Type). {
  fmap: \ a b. (a -> b) -> f a -> f b
}

impl Functor Maybe := {
  fmap := \g mx. case mx { Nothing => Nothing; Just x => Just (g x) }
}
`
	checkSource(t, source, nil)
}

func TestPolyKindedClassUseMethod(t *testing.T) {
	// Use a poly-kinded class method
	source := `
form Bool := { True: Bool; False: Bool; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }

form Functor := \(f: k -> Type). {
  fmap: \ a b. (a -> b) -> f a -> f b
}

impl Functor Maybe := {
  fmap := \g mx. case mx { Nothing => Nothing; Just x => Just (g x) }
}

test := fmap (\x. True) (Just True)
`
	checkSource(t, source, nil)
}

func TestMonoKindedClassStillWorks(t *testing.T) {
	// Ensure existing mono-kinded classes still work (no regression)
	source := `
form Bool := { True: Bool; False: Bool; }

form Eq := \a. {
  eq: a -> a -> Bool
}

impl Eq Bool := {
  eq := \x y. True
}

test := eq True False
`
	checkSource(t, source, nil)
}

// =============================================================================
// ClassInfo kind params
// =============================================================================

func TestClassInfoKindParams(t *testing.T) {
	// Verify that kind params are tracked in ClassInfo
	source := `
form MyClass := \(f: k -> Type). {
  method: \ a. f a -> f a
}
`
	checkSource(t, source, nil)
	// The test just verifies no errors; structural check would require
	// inspecting the Checker state which we don't expose.
}

func TestClassMultipleKindVars(t *testing.T) {
	// Multiple kind variables in a class
	source := `
form BiMap := \(f: k -> j -> Type). {
  bimap: \ a b c d. (a -> c) -> (b -> d) -> f a b -> f c d
}
`
	checkSource(t, source, nil)
}

func TestPolyKindedClassWithSuperclass(t *testing.T) {
	// Poly-kinded class with superclass constraint
	source := `
form Bool := { True: Bool; False: Bool; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }

form Functor := \(f: k -> Type). {
  fmap: \ a b. (a -> b) -> f a -> f b
}

impl Functor Maybe := {
  fmap := \g mx. case mx { Nothing => Nothing; Just x => Just (g x) }
}

form Applicative := \(f: k -> Type). Functor f => {
  pure: \ a. a -> f a
}
`
	checkSource(t, source, nil)
}

// =============================================================================
// Instance kind matching
// =============================================================================

func TestInstanceKindMatch(t *testing.T) {
	// instance Functor Maybe — Maybe: Type -> Type, k unifies with Type
	source := `
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }

form Functor := \(f: k -> Type). {
  fmap: \ a b. (a -> b) -> f a -> f b
}

impl Functor Maybe := {
  fmap := \g mx. case mx { Nothing => Nothing; Just x => Just (g x) }
}
`
	checkSource(t, source, nil)
}
