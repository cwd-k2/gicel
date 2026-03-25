package family

import "github.com/cwd-k2/gicel/internal/compiler/check/env"

// Type aliases for the canonical definitions in the env package.
type TypeFamilyInfo = env.TypeFamilyInfo
type TFParam = env.TFParam
type TFDep = env.TFDep
type TFEquation = env.TFEquation
type MatchResult = env.MatchResult

// Re-export match result constants.
const (
	MatchSuccess       = env.MatchSuccess
	MatchFail          = env.MatchFail
	MatchIndeterminate = env.MatchIndeterminate
)
