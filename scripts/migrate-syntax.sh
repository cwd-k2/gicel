#!/bin/bash
# Migrate GICEL test source strings from old syntax to unified syntax.
# Run from project root: ./scripts/migrate-syntax.sh
#
# Transformations:
# 1. case arrow: "-> " → "=> " (inside case blocks only — heuristic)
# 2. class → data (keyword in declarations)
# 3. instance → impl (keyword in declarations)
# 4. =: → => (in type family equations)
# 5. ADT constructors: "data T a := A | B a" → "data T := \a. { A: (); B: a; }"
#    (This is too complex for sed — handled separately)

set -euo pipefail

TARGET_DIR="${1:-internal/compiler/check}"

echo "=== Phase 1: case arrow -> to => ==="
# In case alternatives: Pattern -> Body  →  Pattern => Body
# Heuristic: lines matching "  X -> " where X is a pattern (starts with uppercase or is a wildcard)
# This is imperfect but covers most cases.
find "$TARGET_DIR" -name '*_test.go' -exec grep -l '\-> ' {} \; | while read f; do
    # Only replace -> that appear inside case blocks (after pattern, before expression)
    # Skip function type arrows (Type -> Type)
    # Heuristic: if line contains a pattern followed by ->, it's a case arrow
    sed -i '' 's/\([A-Z][a-zA-Z0-9_]* [a-z_]*\) -> /\1 => /g' "$f"
    sed -i '' 's/\([A-Z][a-zA-Z0-9_]*\) -> /\1 => /g' "$f"
    sed -i '' 's/_ -> /_ => /g' "$f"
done

echo "=== Phase 2: class keyword → data ==="
# class ClassName params { ... } → data ClassName := \params. { ... }
# This is a simple keyword replacement; structural changes need manual work.
find "$TARGET_DIR" -name '*_test.go' -exec sed -i '' 's/^class /data /g; s/\nclass /\ndata /g' {} \;

echo "=== Phase 3: instance keyword → impl ==="
find "$TARGET_DIR" -name '*_test.go' -exec sed -i '' 's/^instance /impl /g; s/\ninstance /\nimpl /g' {} \;

echo "=== Phase 4: =: → => (type family equations) ==="
find "$TARGET_DIR" -name '*_test.go' -exec sed -i '' 's/ =: / => /g' {} \;

echo "Done. Manual review and structural changes still needed."
