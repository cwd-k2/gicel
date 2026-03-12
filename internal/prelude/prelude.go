// Package prelude provides the standard type definitions for Gomputation.
package prelude

// Source is the Gomputation prelude source code.
// It defines the standard algebraic data types used by the language.
const Source = `
data Bool = True | False
data Unit = Unit
data Result a = Ok a | Err a
data Pair a b = Pair a b
data Maybe a = Just a | Nothing
data List a = Cons a (List a) | Nil
`
