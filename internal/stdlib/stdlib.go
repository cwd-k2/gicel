// Package stdlib provides standard library packs for Gomputation.
package stdlib

import "github.com/cwd-k2/gomputation/internal/reg"

// Registrar is the registration interface that Engine implements.
type Registrar = reg.Registrar

// Pack configures a Registrar with a coherent set of types, primitives, and modules.
type Pack = reg.Pack
