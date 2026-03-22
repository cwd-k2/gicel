package syntax

// This file previously held DeclDataFamily, AssocDataDecl, and AssocDataDef.
// In unified syntax, all of these are absorbed:
//   - Associated data declarations → TyRowTypeDecl in data body rows
//   - Associated data definitions → ImplField (IsType=true) in impl body
//   - Standalone data families → data declarations with case bodies
//
// This file is kept as a placeholder during the migration.
// It will be removed once all references are updated.
