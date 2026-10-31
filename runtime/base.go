package runtime

import "bytes"

// MaxInt is the largest value of Go's int, and of Pascal's integer.
// MinInt is the smallest.
const (
	MaxInt = int(^uint(0) >> 1)
	MinInt = -MaxInt - 1
)

// Bits is a set whose ordinals lie in 0..255. Pascal set types are distinct
// named types with this underlying type; operations convert to Bits and back.
type Bits [4]uint64

// Incl returns s with n added. Ordinals outside 0..255 are ignored.
func Incl(s Bits, n int) Bits {
	if n < 0 || n > 255 {
		return s
	}
	s[uint(n)/64] |= 1 << (uint(n) % 64)
	return s
}

// InclRange returns s with every ordinal from lo through hi added.
// Ordinals outside 0..255 are ignored.
func InclRange(s Bits, lo, hi int) Bits {
	return fill(s, lo, hi)
}

// fill sets every ordinal from lo through hi that lies in 0..255.
func fill(s Bits, lo, hi int) Bits {
	if lo < 0 {
		lo = 0
	}
	if hi > 255 {
		hi = 255
	}
	for n := lo; n <= hi; n++ {
		s[uint(n)/64] |= 1 << (uint(n) % 64)
	}
	return s
}

// In reports whether n is a member of s.
func In(n int, s Bits) bool {
	if n < 0 || n > 255 {
		return false
	}
	return s[uint(n)/64]&(1<<(uint(n)%64)) != 0
}

// Union is the set sum.
func Union(a, b Bits) Bits {
	for i := range a {
		a[i] |= b[i]
	}
	return a
}

// Inter is the set intersection.
func Inter(a, b Bits) Bits {
	for i := range a {
		a[i] &= b[i]
	}
	return a
}

// Diff is the set difference a − b.
func Diff(a, b Bits) Bits {
	for i := range a {
		a[i] &^= b[i]
	}
	return a
}

// Subset reports whether every member of a is a member of b.
func Subset(a, b Bits) bool {
	for i := range a {
		if a[i]&^b[i] != 0 {
			return false
		}
	}
	return true
}

// Idx converts a Pascal index to a zero-based offset.
// When check is set, an index outside lo..hi panics.
func Idx(i, lo, hi int, check bool) int {
	if check && (i < lo || i > hi) {
		panic("index out of range")
	}
	return i - lo
}

// Pack copies len(dst) components from src, starting at the zero-based index start.
// When check is set, a start that does not leave room for every component panics.
// Otherwise a start past the end copies nothing, and a start that runs past the
// end copies only the components that remain.
func Pack[T any](dst, src []T, start int, check bool) {
	if start < 0 || start > len(src) || start+len(dst) > len(src) {
		if check {
			panic("pack")
		}
		if start < 0 || start > len(src) {
			return
		}
	}
	copy(dst, src[start:])
}

// Unpack copies src into dst, starting at the zero-based index start.
// When check is set, a start that does not leave room for every component panics.
func Unpack[T any](src, dst []T, start int, check bool) {
	if start < 0 || start > len(dst) || start+len(src) > len(dst) {
		if check {
			panic("unpack")
		}
		if start < 0 || start > len(dst) {
			return
		}
	}
	copy(dst[start:], src)
}

// SetFit panics when s contains an ordinal outside lo..hi.
func SetFit(s Bits, lo, hi int) {
	allow := fill(Bits{}, lo, hi)
	for i := range s {
		if s[i]&^allow[i] != 0 {
			panic("set element out of range")
		}
	}
}

// InRange panics when n is outside lo..hi.
func InRange(n, lo, hi int) {
	if n < lo || n > hi {
		panic("value out of range")
	}
}

// Step adds dir to n. When check is set, n must lie in lo..hi and the result
// must too. The sum is not formed first, so a result past the ends of int
// is still reported.
func Step(n, lo, hi int, dir int, check bool) int {
	if check && (n < lo || n > hi || (dir > 0 && n >= hi) || (dir < 0 && n <= lo)) {
		panic("ordinal out of range")
	}
	return n + dir
}

// Add returns a+b. When check is set, signed overflow panics.
func Add(a, b int, check bool) int {
	c := a + b
	if check && ((b > 0 && c < a) || (b < 0 && c > a)) {
		panic("overflow")
	}
	return c
}

// Sub returns a-b. When check is set, signed overflow panics.
func Sub(a, b int, check bool) int {
	c := a - b
	if check && ((b < 0 && c < a) || (b > 0 && c > a)) {
		panic("overflow")
	}
	return c
}

// Mul returns a*b. When check is set, signed overflow panics.
func Mul(a, b int, check bool) int {
	if !check || a == 0 || b == 0 {
		return a * b
	}
	if (a == MinInt && b == -1) || (b == MinInt && a == -1) {
		panic("overflow")
	}
	c := a * b
	if c/a != b {
		panic("overflow")
	}
	return c
}

// Mod returns a mod b. A zero or negative divisor panics.
// For a positive divisor the remainder is in 0..b-1, so a negative
// dividend does not keep the sign Go's remainder operator uses.
func Mod(a, b int) int {
	if b <= 0 {
		panic("invalid modulus")
	}
	r := a % b
	if r < 0 {
		r += b
	}
	return r
}

// Quo returns a/b. Division by zero panics.
func Quo(a, b float64) float64 {
	if b == 0 {
		panic("division by zero")
	}
	return a / b
}

// AbsInt returns the absolute value of n.
// When check is set, abs of the most negative int panics.
func AbsInt(n int, check bool) int {
	if n < 0 {
		if check && n == MinInt {
			panic("overflow")
		}
		return -n
	}
	return n
}

// StrLess reports whether a is lexicographically before b.
// Both slices are Pascal strings of the same length.
func StrLess(a, b []byte) bool {
	return bytes.Compare(a, b) < 0
}

// B2I maps false to 0 and true to 1.
func B2I(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Chr returns the character with ordinal n.
// When check is set, n outside 0..255 panics.
func Chr(n int, check bool) byte {
	if check && (n < 0 || n > 255) {
		panic("chr out of range")
	}
	return byte(n)
}

// Trunc returns the integer part of x, toward zero.
func Trunc(x float64) int { return int(x) }
