package stdlib

// CoreSource contains Computation-essential definitions: IxMonad class,
// Computation instance, kind-lifting alias, effect alias, and the then combinator.
// Compiled as the "Core" module by NewEngine() and auto-imported into all modules.
var CoreSource = mustReadSource("core")

// PreludeSource is the standard prelude: data types, type classes, instances,
// arithmetic, list operations, and string operations.
var PreludeSource = mustReadSource("prelude")

// Prelude registers primitives and compiles the Prelude module.
var Prelude Pack = func(e Registrar) error {
	// --- Num primitives (21) ---
	e.RegisterPrim("_addInt", addIntImpl)
	e.RegisterPrim("_subInt", subIntImpl)
	e.RegisterPrim("_mulInt", mulIntImpl)
	e.RegisterPrim("_divInt", divIntImpl)
	e.RegisterPrim("_modInt", modIntImpl)
	e.RegisterPrim("_negInt", negIntImpl)
	e.RegisterPrim("_eqInt", eqIntImpl)
	e.RegisterPrim("_cmpInt", cmpIntImpl)
	e.RegisterPrim("_showInt", numShowIntImpl)
	e.RegisterPrim("_addDouble", addDoubleImpl)
	e.RegisterPrim("_subDouble", subDoubleImpl)
	e.RegisterPrim("_mulDouble", mulDoubleImpl)
	e.RegisterPrim("_negDouble", negDoubleImpl)
	e.RegisterPrim("_eqDouble", eqDoubleImpl)
	e.RegisterPrim("_cmpDouble", cmpDoubleImpl)
	e.RegisterPrim("_showDouble", showDoubleImpl)
	e.RegisterPrim("_divDouble", divDoubleImpl)
	e.RegisterPrim("_toDouble", toDoubleImpl)
	e.RegisterPrim("_round", roundImpl)
	e.RegisterPrim("_floor", floorImpl)
	e.RegisterPrim("_ceiling", ceilingImpl)
	e.RegisterPrim("_truncate", truncateImpl)

	// --- List primitives (18) ---
	e.RegisterPrim("_listFromSlice", fromSliceImpl)
	e.RegisterPrim("_listToSlice", toSliceImpl)
	e.RegisterPrim("_listLength", lengthImpl)
	e.RegisterPrim("_listConcat", concatImpl)
	e.RegisterPrim("_listFoldl", foldlImpl)
	e.RegisterPrim("_listTake", takeImpl)
	e.RegisterPrim("_listDrop", dropImpl)
	e.RegisterPrim("_listIndex", indexImpl)
	e.RegisterPrim("_listReplicate", replicateImpl)
	e.RegisterPrim("_listReverse", reverseImpl)
	e.RegisterPrim("_listZip", zipImpl)
	e.RegisterPrim("_listUnzip", unzipImpl)
	e.RegisterPrim("_listDropWhile", dropWhileImpl)
	e.RegisterPrim("_listSpan", spanImpl)
	e.RegisterPrim("_listSortBy", sortByImpl)
	e.RegisterPrim("_listScanl", scanlImpl)
	e.RegisterPrim("_listUnfoldr", unfoldrImpl)
	e.RegisterPrim("_listIterateN", iterateNImpl)

	// --- Str primitives (18) ---
	e.RegisterPrim("_eqStr", eqStrImpl)
	e.RegisterPrim("_cmpStr", cmpStrImpl)
	e.RegisterPrim("_appendStr", appendStrImpl)
	e.RegisterPrim("_emptyStr", emptyStrImpl)
	e.RegisterPrim("_lengthStr", lengthStrImpl)
	e.RegisterPrim("_eqRune", eqRuneImpl)
	e.RegisterPrim("_cmpRune", cmpRuneImpl)
	e.RegisterPrim("_charAt", charAtImpl)
	e.RegisterPrim("_substring", substringImpl)
	e.RegisterPrim("_toUpper", toUpperImpl)
	e.RegisterPrim("_toLower", toLowerImpl)
	e.RegisterPrim("_trim", trimImpl)
	e.RegisterPrim("_contains", containsImpl)
	e.RegisterPrim("_split", splitImpl)
	e.RegisterPrim("_join", joinImpl)
	e.RegisterPrim("_readInt", readIntImpl)
	e.RegisterPrim("_toRunes", toRunesImpl)
	e.RegisterPrim("_fromRunes", fromRunesImpl)

	// Fusion rule: packed roundtrip elimination.
	e.RegisterRewriteRule(strPackedRoundtrip)

	return e.RegisterModule("Prelude", PreludeSource)
}
