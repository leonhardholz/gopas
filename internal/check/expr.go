package check

import "gopas/internal/ast"

type operand struct {
	typ Type
	val *value
	vr  bool
	obj *obj
	fn  bool
}

func (c *checker) expr(e ast.Expr) operand {
	op := c.operand(e, false)
	op.typ = factor(op.typ)
	return op
}

func factor(t Type) Type {
	if _, ok := t.(*subrangeType); ok {
		return host(t)
	}
	return t
}

func (c *checker) operand(e ast.Expr, target bool) (op operand) {
	defer c.locate(e)()
	defer func() {
		if e != nil && c.raw != nil {
			c.raw[e] = op
		}
	}()
	switch e := e.(type) {
	case *ast.Int, *ast.Real, *ast.Str:
		v := c.constant(e)
		return operand{typ: v.typ, val: v}
	case *ast.Nil:
		return operand{typ: nilT}
	case *ast.Name:
		return c.name(e.Ident, target)
	case *ast.Paren:
		inner := c.expr(e.X)
		return operand{typ: inner.typ, val: inner.val}
	case *ast.Unary:
		if e.Op == "not" {
			x := c.expr(e.X)
			if !isInvalid(x.typ) && host(x.typ) != booleanType {
				c.errorf(ErrNotBool)
				return operand{typ: invalid}
			}
			return operand{typ: booleanType}
		}
		x := c.expr(e.X)
		if !isInvalid(x.typ) && host(x.typ) != integerType && host(x.typ) != realType {
			c.errorf(ErrSignType)
			return operand{typ: invalid}
		}
		return operand{typ: x.typ, val: signed(e.Op, x.val)}
	case *ast.Binary:
		return c.binary(e)
	case *ast.CallExpr:
		o := c.lookup(e.Name)
		c.noteCall(e, o)
		t := c.call(e.Name, e.Args, false)
		return operand{typ: t, obj: o}
	case *ast.SetLit:
		return c.setLit(e)
	case *ast.Indexed:
		return c.indexed(e.Base, e.Indexes)
	case *ast.Select:
		base := c.operand(e.Base, true)
		rec, ok := base.typ.(*recordType)
		if isInvalid(base.typ) {
			return operand{typ: invalid}
		}
		if !ok {
			c.errorf(ErrNotRecord, e.Field)
			return operand{typ: invalid}
		}
		for _, f := range rec.fields {
			if f.name == e.Field {
				return operand{typ: f.typ, vr: base.vr}
			}
		}
		c.errorf(ErrNoField, e.Field)
		return operand{typ: invalid}
	case *ast.Loose:
		c.loose(e.Name)
		return operand{typ: invalid}
	case *ast.Deref:
		base := c.operand(e.Base, true)
		if isInvalid(base.typ) {
			return operand{typ: invalid}
		}
		switch t := base.typ.(type) {
		case *pointerType:
			if t.base == nil {
				c.errorf(ErrPtrOpen)
				return operand{typ: invalid}
			}
			// The operand of ^ is a variable-access. A function identifier
			// denotes the function result only as the whole left side of
			// an assignment, so f^ is not an identified variable.
			if !base.vr {
				c.errorf(ErrPtrVar)
				return operand{typ: invalid}
			}
			return operand{typ: t.base, vr: true}
		case *fileType:
			return operand{typ: t.elem, vr: true}
		case textType:
			return operand{typ: charType, vr: true}
		default:
			c.errorf(ErrDeref)
			return operand{typ: invalid}
		}
	default:
		c.errorf(ErrBadExpr)
		return operand{typ: invalid}
	}
}

func signed(op string, v *value) *value {
	if v == nil || !v.known {
		return v
	}
	out := *v
	if v.real {
		out.f = v.f
		if op == "-" {
			out.f = -v.f
		}
		return &out
	}
	if op == "-" {
		out.n = -v.n
	}
	return &out
}

func (c *checker) name(ident string, target bool) operand {
	o := c.lookup(ident)
	if o == nil {
		c.errorf(ErrNotDeclared, ident)
		return operand{typ: invalid}
	}
	if o.blocked {
		return operand{typ: invalid, obj: o}
	}
	if o.kind == kindVar && (o.name == "input" || o.name == "output") {
		if o.name == "input" {
			c.usedInput = true
		} else {
			c.usedOutput = true
		}
	}
	switch o.kind {
	case kindConst:
		return operand{typ: o.val.typ, val: o.val, obj: o}
	case kindVar:
		return operand{typ: o.typ, vr: true, obj: o}
	case kindFunc:
		if target && c.inFunc(o) {
			return operand{typ: o.typ, fn: true, obj: o}
		}
		if o.predef != "" && len(o.params) == 0 {
			return operand{typ: c.predefFunc(o.predef, nil), obj: o}
		}
		if len(o.params) != 0 {
			c.errorf(ErrFuncCall, ident)
			return operand{typ: invalid}
		}
		return operand{typ: o.typ, obj: o}
	default:
		c.errorf(ErrNotValue, ident)
		return operand{typ: invalid, obj: o}
	}
}

func (c *checker) inFunc(o *obj) bool {
	for _, f := range c.fns {
		if f == o {
			return true
		}
	}
	return false
}

func (c *checker) indexed(base ast.Expr, indexes []ast.Expr) operand {
	b := c.operand(base, true)
	t := b.typ
	vr := b.vr
	// An index denotes a component of an array variable. A string constant
	// has an array type and is not a variable.
	if !isInvalid(t) && !vr {
		c.errorf(ErrIndexVar)
	}
	for _, ix := range indexes {
		a, ok := t.(*arrayType)
		if isInvalid(t) {
			return operand{typ: invalid}
		}
		if !ok {
			c.errorf(ErrNotArray)
			return operand{typ: invalid}
		}
		index := c.expr(ix)
		if !isInvalid(index.typ) && !assignCompat(a.index, index.typ) {
			c.errorf(ErrIndexType)
		} else if index.val != nil && !index.val.fits(a.index) {
			c.errorf(ErrIndexRange)
		}
		t = a.elem
	}
	return operand{typ: t, vr: vr}
}

func (c *checker) setLit(e *ast.SetLit) operand {
	if len(e.Elems) == 0 {
		return operand{typ: emptySet}
	}
	var base Type
	for _, el := range e.Elems {
		lo := c.expr(el.Low)
		if !isInvalid(lo.typ) && !ordinal(lo.typ) {
			c.errorf(ErrSetMember)
			return operand{typ: invalid}
		}
		if base == nil {
			base = lo.typ
		} else if !isInvalid(lo.typ) && !compatible(base, lo.typ) {
			c.errorf(ErrSetTypes)
			return operand{typ: invalid}
		}
		if el.High != nil {
			hi := c.expr(el.High)
			if !isInvalid(hi.typ) && !compatible(base, hi.typ) {
				c.errorf(ErrSetRange)
			}
		}
	}
	return operand{typ: &setType{base: base, canon: true}}
}

func (c *checker) binary(e *ast.Binary) operand {
	l := c.expr(e.Left)
	r := c.expr(e.Right)
	if isInvalid(l.typ) || isInvalid(r.typ) {
		return operand{typ: invalid}
	}
	switch e.Op {
	case "+", "-", "*":
		if t, ok := c.arith(e.Op, l.typ, r.typ); ok {
			op := operand{typ: t}
			if sameType(t, integerType) {
				op.val = intOp(e.Op, l.val, r.val)
			}
			return op
		}
		if t, ok := setCombine(l.typ, r.typ, true); ok {
			return operand{typ: t}
		}
		c.errorf(ErrOpType, e.Op)
		return operand{typ: invalid}
	case "/":
		if numeric(l.typ) && numeric(r.typ) {
			return operand{typ: realType}
		}
		c.errorf(ErrDivType)
		return operand{typ: invalid}
	case "div", "mod":
		if sameType(host(l.typ), integerType) && sameType(host(r.typ), integerType) {
			op := operand{typ: integerType}
			if e.Op == "div" {
				op.val = intOp(e.Op, l.val, r.val)
			}
			return op
		}
		c.errorf(ErrOpInt, e.Op)
		return operand{typ: invalid}
	case "and", "or":
		if host(l.typ) == booleanType && host(r.typ) == booleanType {
			return operand{typ: booleanType}
		}
		c.errorf(ErrOpBool, e.Op)
		return operand{typ: invalid}
	case "=", "<>":
		if comparable(l.typ, r.typ) {
			return operand{typ: booleanType}
		}
		c.errorf(ErrOpType, e.Op)
		return operand{typ: invalid}
	case "<", ">":
		if ordered(l.typ, r.typ) {
			return operand{typ: booleanType}
		}
		c.errorf(ErrOpType, e.Op)
		return operand{typ: invalid}
	case "<=", ">=":
		if ordered(l.typ, r.typ) {
			return operand{typ: booleanType}
		}
		if _, ok := setCombine(l.typ, r.typ, false); ok {
			return operand{typ: booleanType}
		}
		c.errorf(ErrOpType, e.Op)
		return operand{typ: invalid}
	case "in":
		if isEmpty(r.typ) && ordinal(l.typ) {
			return operand{typ: booleanType}
		}
		s, ok := r.typ.(*setType)
		if ok && ordinal(l.typ) && compatible(l.typ, s.base) {
			return operand{typ: booleanType}
		}
		c.errorf(ErrIn)
		return operand{typ: invalid}
	default:
		c.errorf(ErrBadOp, e.Op)
		return operand{typ: invalid}
	}
}

func numeric(t Type) bool {
	h := host(t)
	return h == integerType || h == realType
}

func (c *checker) arith(op string, l, r Type) (Type, bool) {
	li := sameType(host(l), integerType)
	ri := sameType(host(r), integerType)
	lr := host(l) == realType
	rr := host(r) == realType
	if li && ri {
		return integerType, true
	}
	if (li || lr) && (ri || rr) && (lr || rr) {
		return realType, true
	}
	return nil, false
}

func comparable(l, r Type) bool {
	if isNil(l) && isNil(r) {
		return false
	}
	if isNil(l) {
		_, ok := r.(*pointerType)
		return ok
	}
	if isNil(r) {
		_, ok := l.(*pointerType)
		return ok
	}
	if ordinal(l) && ordinal(r) && sameType(host(l), host(r)) {
		return true
	}
	if numeric(l) && numeric(r) && (host(l) == realType || host(r) == realType) {
		return true
	}
	if _, ok := setCombine(l, r, false); ok {
		return true
	}
	if _, ok := l.(*pointerType); ok {
		_, ok := r.(*pointerType)
		return ok && sameType(l, r)
	}
	ln, lok := stringLen(l)
	rn, rok := stringLen(r)
	return lok && rok && ln == rn
}

func isNil(t Type) bool {
	_, ok := t.(nilType)
	return ok
}

func isEmpty(t Type) bool {
	_, ok := t.(emptySetType)
	return ok
}

// setCombine accepts two set operands, including the empty set [].
// When values is set, the result is the set type of the operation.
// Named set types are compatible only when both are packed or neither is.
// A set constructor has the canonical unpacked set-of-T type and may combine
// with either form. That result stays canonical.
func setCombine(l, r Type, values bool) (Type, bool) {
	ls, lok := l.(*setType)
	rs, rok := r.(*setType)
	le, re := isEmpty(l), isEmpty(r)
	if !(lok || le) || !(rok || re) {
		return nil, false
	}
	if lok && rok && !compatible(ls.base, rs.base) {
		return nil, false
	}
	if lok && rok && ls.packed != rs.packed && !ls.canon && !rs.canon {
		return nil, false
	}
	if !values {
		return booleanType, true
	}
	switch {
	case le && re:
		return emptySet, true
	case le:
		return r, true
	case re:
		return l, true
	}
	canon := ls.canon || rs.canon
	return &setType{packed: ls.packed && rs.packed && !canon, base: ls.base, canon: canon}, true
}

func ordered(l, r Type) bool {
	if ordinal(l) && ordinal(r) && sameType(host(l), host(r)) {
		return true
	}
	if numeric(l) && numeric(r) {
		return true
	}
	ln, lok := stringLen(l)
	rn, rok := stringLen(r)
	return lok && rok && ln == rn
}
