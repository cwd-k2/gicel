### Data.Math

Mathematical functions: integer power, bitwise operations, and transcendental functions on Double.

CLI: `--packs math`

```gicel
import Prelude
import Data.Math
```

## Int Operations

| Function | Type                       | Description                                     |
| -------- | -------------------------- | ----------------------------------------------- |
| `pow`    | `Int -> Int -> Int`        | Integer exponentiation (negative exp returns 0) |
| `clamp`  | `Int -> Int -> Int -> Int` | `clamp lo hi x` — clamp x to [lo, hi]           |
| `divMod` | `Int -> Int -> (Int, Int)` | Euclidean division (non-negative remainder)     |

## Bitwise Operations

| Function   | Type                | Description                                |
| ---------- | ------------------- | ------------------------------------------ |
| `bitAnd`   | `Int -> Int -> Int` | Bitwise AND                                |
| `bitOr`    | `Int -> Int -> Int` | Bitwise OR                                 |
| `bitXor`   | `Int -> Int -> Int` | Bitwise XOR                                |
| `bitNot`   | `Int -> Int`        | Bitwise NOT (complement)                   |
| `shiftL`   | `Int -> Int -> Int` | Left shift (0 for negative or >= 64 shift) |
| `shiftR`   | `Int -> Int -> Int` | Arithmetic right shift                     |
| `popCount` | `Int -> Int`        | Number of set bits                         |

## Double Operations

| Function                    | Type                                   | Description               |
| --------------------------- | -------------------------------------- | ------------------------- |
| `sqrt`                      | `Double -> Double`                     | Square root               |
| `cbrt`                      | `Double -> Double`                     | Cube root                 |
| `sin`, `cos`, `tan`         | `Double -> Double`                     | Trigonometric functions   |
| `asin`, `acos`, `atan`      | `Double -> Double`                     | Inverse trigonometric     |
| `atan2`                     | `Double -> Double -> Double`           | Two-argument arctangent   |
| `exp`                       | `Double -> Double`                     | Exponential (e^x)         |
| `log`, `log2`, `log10`      | `Double -> Double`                     | Logarithms                |
| `powDouble`                 | `Double -> Double -> Double`           | Floating-point power      |
| `floorF`, `ceilF`, `roundF` | `Double -> Double`                     | Rounding (returns Double) |
| `isNaN`                     | `Double -> Bool`                       | NaN test                  |
| `isInfinite`                | `Double -> Bool`                       | Infinity test             |
| `clampDouble`               | `Double -> Double -> Double -> Double` | Clamp to [lo, hi]         |

Note: Prelude provides `floor :: Double -> Int` and `round :: Double -> Int` (returning Int). Data.Math provides `floorF`/`roundF`/`ceilF` returning Double.
