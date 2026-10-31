package check

import "math"

// Types are what expressions denote. The AST records how a type was written;
// values of Type are the types those denotations mean. Aliases such as
// `number = integer` share the integer type. Each new array, record, set,
// file, pointer, enumeration, or subrange is its own type.
type Type interface{ typeNode() }

type basic struct {
	name    string
	ordinal bool
}

func (*basic) typeNode() {}

type enumType struct {
	names []string
}

func (*enumType) typeNode() {}

type subrangeType struct {
	host   Type
	lo, hi *value
}

func (*subrangeType) typeNode() {}

type arrayType struct {
	packed bool
	index  Type
	elem   Type
	// conformant is a parameter type from a conformant-array-schema.
	// It is not an array-type, so it cannot be the actual of a value
	// conformant array.
	conformant bool
}

func (*arrayType) typeNode() {}

type field struct {
	name string
	typ  Type
	tag  bool
}

type recordType struct {
	packed  bool
	fields  []field
	variant *variantPart
}

// variantPart is one case in a record. Each arm's next part is the variant
// nested inside that arm.
type variantPart struct {
	tag  Type
	arms []variantArm
}

type variantArm struct {
	labels []int64
	next   *variantPart
}

func (*recordType) typeNode() {}

type setType struct {
	packed bool
	base   Type
	canon  bool
}

func (*setType) typeNode() {}

type fileType struct {
	packed bool
	elem   Type
}

func (*fileType) typeNode() {}

type pointerType struct {
	base Type
}

func (*pointerType) typeNode() {}

// textType is the required type text. It is a file of char with line structure.
type textType struct{}

func (textType) typeNode() {}

// nilType is the type of the bare nil value, before a pointer context fixes it.
type nilType struct{}

func (nilType) typeNode() {}

// emptySetType is the type of []. The target of an assignment decides the set type.
type emptySetType struct{}

func (emptySetType) typeNode() {}

// invalidType marks an expression that already has an error.
type invalidType struct{}

func (invalidType) typeNode() {}

var (
	integerType Type = &basic{name: "integer", ordinal: true}
	realType    Type = &basic{name: "real"}
	booleanType Type = &basic{name: "boolean", ordinal: true}
	charType    Type = &basic{name: "char", ordinal: true}
	textFile    Type = textType{}
	nilT        Type = nilType{}
	emptySet    Type = emptySetType{}
	invalid     Type = invalidType{}
)

func isInvalid(t Type) bool { return t == nil || t == invalid }

func host(t Type) Type {
	for {
		s, ok := t.(*subrangeType)
		if !ok {
			return t
		}
		t = s.host
	}
}

func ordinal(t Type) bool {
	switch host(t).(type) {
	case *basic:
		b := host(t).(*basic)
		return b.ordinal
	case *enumType:
		return true
	default:
		return false
	}
}

// ordinalSpan is the inclusive range of a finite ordinal. ok is false for
// integer, for a subrange whose bounds are not known, and for other types.
func ordinalSpan(t Type) (lo, hi int64, ok bool) {
	if s, isSub := t.(*subrangeType); isSub {
		if s.lo == nil || s.hi == nil || !s.lo.known || !s.hi.known {
			return 0, 0, false
		}
		return s.lo.n, s.hi.n, true
	}
	switch h := host(t).(type) {
	case *enumType:
		if len(h.names) == 0 {
			return 0, -1, true
		}
		return 0, int64(len(h.names) - 1), true
	case *basic:
		switch h {
		case booleanType:
			return 0, 1, true
		case charType:
			return 0, 255, true
		default:
			return 0, 0, false
		}
	default:
		return 0, 0, false
	}
}

// fitLen is the number of values in lo..hi when that count fits in a Go array
// length. The subtraction is done in unsigned arithmetic so a span past
// maxint is rejected instead of wrapping.
func fitLen(lo, hi int64) (int, bool) {
	if lo > hi {
		return 0, false
	}
	if lo == math.MinInt64 && hi == math.MaxInt64 {
		return 0, false
	}
	count := uint64(hi) - uint64(lo) + 1
	if count == 0 || count > uint64(math.MaxInt) {
		return 0, false
	}
	return int(count), true
}

// setBaseOK reports whether t fits in a 256-bit set: at most 256 values,
// and every ordinal in 0..255.
func setBaseOK(t Type) bool {
	lo, hi, ok := ordinalSpan(t)
	if !ok || lo < 0 || hi > 255 || hi-lo+1 > 256 {
		return false
	}
	return true
}

func isFile(t Type) bool {
	switch host(t).(type) {
	case *fileType, textType:
		return true
	default:
		return false
	}
}

// containsFile reports a file type or a structured type with a file component.
func containsFile(t Type) bool {
	switch t := t.(type) {
	case *fileType, textType:
		return true
	case *arrayType:
		return containsFile(t.elem)
	case *recordType:
		for _, f := range t.fields {
			if containsFile(f.typ) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// sameType reports identical types. Two string types of equal length are
// compatible and assignment-compatible, but they are not the same type:
// a variable parameter requires the identical type.
func sameType(a, b Type) bool {
	return a == b
}

func compatible(a, b Type) bool {
	if sameType(a, b) || isInvalid(a) || isInvalid(b) {
		return true
	}
	if ordinal(a) && ordinal(b) && sameType(host(a), host(b)) {
		return true
	}
	as, aok := a.(*setType)
	bs, bok := b.(*setType)
	if aok && bok && compatible(as.base, bs.base) {
		return true
	}
	return false
}

// assignCompat reports whether a value of type src may be assigned to dst.
func assignCompat(dst, src Type) bool {
	if isInvalid(dst) || isInvalid(src) {
		return true
	}
	if _, ok := src.(nilType); ok {
		_, ok := dst.(*pointerType)
		return ok
	}
	if _, ok := src.(emptySetType); ok {
		_, ok := dst.(*setType)
		return ok
	}
	if containsFile(dst) || containsFile(src) {
		return false
	}
	if sameType(dst, realType) && sameType(src, integerType) {
		return true
	}
	if sameType(dst, src) {
		return true
	}
	if ordinal(dst) && ordinal(src) && sameType(host(dst), host(src)) {
		return true
	}
	ds, dok := dst.(*setType)
	ss, sok := src.(*setType)
	if dok && sok && compatible(ds.base, ss.base) {
		return true
	}
	dn, dok := stringLen(dst)
	sn, sok := stringLen(src)
	return dok && sok && dn == sn
}

// stringLen reports n when t is a packed array [1..n] of char with n > 1.
// The component type is char itself. A subrange of char is a different type.
func stringLen(t Type) (int, bool) {
	a, ok := t.(*arrayType)
	if !ok || !a.packed || a.elem != charType {
		return 0, false
	}
	s, ok := a.index.(*subrangeType)
	if !ok || host(s.host) != integerType || s.lo == nil || s.hi == nil || !s.lo.known || !s.hi.known {
		return 0, false
	}
	if s.lo.n != 1 || s.hi.n <= 1 {
		return 0, false
	}
	return int(s.hi.n), true
}

// value is a constant known at check time. known is false when the value
// exists but its ordinal is not determined, as with maxint.
type value struct {
	typ   Type
	known bool
	n     int64
	real  bool
	f     float64
	text  string
}

// intOp is the result of an integer operation on two known values.
// A result outside -maxint..maxint is not known: that overflow is a
// designated error and is performed at run time.
func intOp(op string, l, r *value) *value {
	if l == nil || r == nil || !l.known || !r.known || l.real || r.real {
		return nil
	}
	a, b := l.n, r.n
	var c int64
	switch op {
	case "+":
		c = a + b
		if (b > 0 && c < a) || (b < 0 && c > a) {
			return nil
		}
	case "-":
		c = a - b
		if (b < 0 && c < a) || (b > 0 && c > a) {
			return nil
		}
	case "*":
		if a == 0 || b == 0 {
			c = 0
			break
		}
		if (a == math.MinInt64 && b == -1) || (b == math.MinInt64 && a == -1) {
			return nil
		}
		c = a * b
		if c/a != b {
			return nil
		}
	case "div":
		if b == 0 || (a == math.MinInt64 && b == -1) {
			return nil
		}
		c = a / b
	default:
		return nil
	}
	return &value{typ: integerType, known: true, n: c}
}

func (v *value) fits(dst Type) bool {
	if v == nil || !v.known || !ordinal(dst) {
		return true
	}
	s, ok := dst.(*subrangeType)
	if !ok || s.lo == nil || s.hi == nil || !s.lo.known || !s.hi.known {
		return true
	}
	return v.n >= s.lo.n && v.n <= s.hi.n
}
