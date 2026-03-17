// Example: multi-module — selective and qualified imports.
//
// Demonstrates the three import forms:
//   - Open import:      import Geometry       (all names unqualified)
//   - Selective import:  import Color (name)   (only listed names)
//   - Qualified import:  import Math as M      (access via M.double)
//
// Also demonstrates multi-file CLI usage:
//
//	gicel run --module Geometry=geometry.gicel \
//	          --module Color=color.gicel       \
//	          --module Math=math.gicel         \
//	          main.gicel
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cwd-k2/gicel"
)

// Geometry module: defines Point and operations.
const geometrySource = `
data Point := MkPoint Int Int

mkPoint :: Int -> Int -> Point
mkPoint := \x y. MkPoint x y

getX :: Point -> Int
getX := \p. case p { MkPoint x _ -> x }

getY :: Point -> Int
getY := \p. case p { MkPoint _ y -> y }
`

// Color module: defines Color and a display name.
const colorSource = `
import Prelude

data Color := Red | Green | Blue

name :: Color -> String
name := \c. case c { Red -> "red"; Green -> "green"; Blue -> "blue" }
`

// Math module: simple arithmetic helpers.
const mathSource = `
import Prelude

double :: Int -> Int
double := \x. x + x

square :: Int -> Int
square := \x. x * x
`

// Main program uses all three import forms on different modules.
const mainSource = `
import Prelude

-- Open import: all Geometry names available directly
import Geometry

-- Selective import: only Color constructors and name function
import Color (Color(..), name)

-- Qualified import: access Math via M qualifier
import Math as M

-- Use open-imported Geometry functions
point := mkPoint 3 4

-- Use selectively imported Color names
colorName := name Red

-- Use qualified Math access
doubled := M.double (getX point)

main := (getX point, colorName, doubled)
`

func main() {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		log.Fatal(err)
	}

	// Register modules in dependency order.
	if err := eng.RegisterModule("Geometry", geometrySource); err != nil {
		log.Fatal("Geometry module: ", err)
	}
	if err := eng.RegisterModule("Color", colorSource); err != nil {
		log.Fatal("Color module: ", err)
	}
	if err := eng.RegisterModule("Math", mathSource); err != nil {
		log.Fatal("Math module: ", err)
	}

	rt, err := eng.NewRuntime(mainSource)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		log.Fatal("runtime error: ", err)
	}

	// (getX (mkPoint 3 4), name Red, M.double 3) => (3, "red", 6)
	fmt.Println("result =", result.Value)
	// Output: result = (3, "red", 6)
}
