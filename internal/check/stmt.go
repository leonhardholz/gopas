package check

import (
	"unicode"

	"gopas/internal/ast"
)

func universe() *scope {
	s := &scope{objs: map[string]*obj{}}
	types := []struct {
		name string
		typ  Type
	}{
		{"integer", integerType},
		{"real", realType},
		{"boolean", booleanType},
		{"char", charType},
		{"text", textFile},
	}
	for _, t := range types {
		s.objs[t.name] = &obj{name: t.name, kind: kindType, typ: t.typ}
	}
	s.objs["false"] = &obj{name: "false", kind: kindConst, typ: booleanType, val: &value{typ: booleanType, known: true, n: 0}}
	s.objs["true"] = &obj{name: "true", kind: kindConst, typ: booleanType, val: &value{typ: booleanType, known: true, n: 1}}
	s.objs["maxint"] = &obj{name: "maxint", kind: kindConst, typ: integerType, val: &value{typ: integerType, known: true, n: int64(^uint(0) >> 1)}}
	for _, name := range []string{
		"rewrite", "put", "reset", "get", "read", "readln", "write", "writeln",
		"page", "new", "dispose", "pack", "unpack",
	} {
		s.objs[name] = &obj{name: name, kind: kindProc, predef: name}
	}
	for _, name := range []string{
		"abs", "sqr", "sin", "cos", "exp", "ln", "sqrt", "arctan",
		"trunc", "round", "ord", "chr", "succ", "pred", "odd", "eof", "eoln",
	} {
		s.objs[name] = &obj{name: name, kind: kindFunc, predef: name}
	}
	return s
}

func (c *checker) stmts(list []ast.Stmt) {
	for _, s := range list {
		c.stmt(s)
	}
}

func (c *checker) stmt(s ast.Stmt) {
	if s == nil {
		return
	}
	defer c.locate(s)()
	switch s := s.(type) {
	case *ast.Labeled:
		c.label(s.Label)
		c.stmt(s.Stmt)
	case *ast.Assign:
		c.assign(s.Left, s.Value)
	case *ast.Call:
		c.noteCall(s, c.lookup(s.Name))
		c.call(s.Name, s.Args, true)
	case *ast.Read:
		c.read(s)
	case *ast.Write:
		c.write(s)
	case *ast.Goto:
		c.gotos = append(c.gotos, pendingGoto{name: s.Label, seq: c.seq, block: c.scope.block, at: s})
	case *ast.Compound:
		c.pushSeq()
		c.stmts(s.Body)
		c.seq = c.seq.parent
		if s.Open {
			c.errorf(ErrSyntax, "'end'")
		}
	case *ast.Loose:
		c.loose(s.Name)
	case *ast.If:
		c.condition(s.Cond)
		c.stmt(s.Then)
		c.stmt(s.Else)
	case *ast.While:
		c.condition(s.Cond)
		c.pushSeq()
		c.stmt(s.Body)
		c.seq = c.seq.parent
	case *ast.Repeat:
		c.pushSeq()
		c.stmts(s.Body)
		c.seq = c.seq.parent
		c.condition(s.Cond)
	case *ast.For:
		c.forStmt(s)
	case *ast.Case:
		c.caseStmt(s)
	case *ast.With:
		c.withStmt(s)
	default:
		c.errorf(ErrBadStmt)
	}
}

func (c *checker) loose(name string) {
	if name != "" && c.lookup(name) == nil {
		c.errorf(ErrNotDeclared, name)
		return
	}
	c.errorf(ErrBadStmt)
}

func (c *checker) label(name string) {
	b := c.scope.block
	if !b.declared[name] {
		c.errorf(ErrUndefLabel, name)
		return
	}
	if _, ok := b.sites[name]; ok {
		c.errorf(ErrDupLabel, name)
		return
	}
	b.sites[name] = c.seq
}

func (c *checker) condition(e ast.Expr) {
	op := c.expr(e)
	if !isInvalid(op.typ) && host(op.typ) != booleanType {
		c.errorf(ErrCondBool)
	}
}

func (c *checker) assign(left, right ast.Expr) {
	dst := c.operand(left, true)
	src := c.expr(right)
	if dst.obj != nil && c.controlled(dst.obj) {
		c.errorf(ErrThreaten, dst.obj.name)
	}
	if dst.fn {
		if dst.obj != nil {
			dst.obj.assigned = true
		}
		if !assignCompat(dst.typ, src.typ) {
			c.errorf(ErrResultMismatch)
		}
		return
	}
	if !dst.vr {
		if !isInvalid(dst.typ) {
			c.errorf(ErrNotVar)
		}
		return
	}
	if !assignCompat(dst.typ, src.typ) {
		c.errorf(ErrTypeConflict)
		return
	}
	if src.val != nil && !src.val.fits(dst.typ) {
		c.errorf(ErrRange)
	}
}

func (c *checker) controlled(o *obj) bool {
	for _, ctrl := range c.controls {
		if ctrl == o {
			return true
		}
	}
	return false
}

func (c *checker) forStmt(s *ast.For) {
	o := c.lookup(s.Name)
	if o == nil || o.kind != kindVar || !o.fromVar || !varInBlock(o, c.scope) {
		c.errorf(ErrForVar, s.Name)
	} else if !ordinal(o.typ) {
		c.errorf(ErrForOrdinal, s.Name)
	} else {
		from := c.expr(s.From)
		to := c.expr(s.To)
		if !assignCompat(o.typ, from.typ) || !assignCompat(o.typ, to.typ) {
			c.errorf(ErrForBounds)
		}
	}
	var ctrl *obj
	if o != nil && o.kind == kindVar && o.fromVar && varInBlock(o, c.scope) {
		ctrl = o
	}
	if c.forOf != nil {
		c.forOf[s] = o
	}
	prev := c.controls
	if ctrl != nil {
		c.threatObj(ctrl)
		c.controls = append(c.controls, ctrl)
	}
	c.pushSeq()
	c.stmt(s.Body)
	c.seq = c.seq.parent
	c.controls = prev
}

func (c *checker) caseStmt(s *ast.Case) {
	op := c.expr(s.Expr)
	if !isInvalid(op.typ) && !ordinal(op.typ) {
		c.errorf(ErrCaseOrdinal)
	}
	seen := map[int64]bool{}
	for _, arm := range s.Arms {
		for _, lab := range arm.Labels {
			v := c.constant(lab)
			if v == nil || isInvalid(v.typ) {
				continue
			}
			if !ordinal(v.typ) || !compatible(v.typ, op.typ) {
				c.errorf(ErrCaseLabel)
			} else if v.known {
				if seen[v.n] {
					c.errorf(ErrDupCase)
				}
				seen[v.n] = true
			}
		}
		c.stmt(arm.Stmt)
	}
}

// varInBlock reports that o is a variable of the function or program block
// currently being checked. A with statement pushes a scope in that same block.
func varInBlock(o *obj, s *scope) bool {
	return o != nil && s != nil && o.block != nil && o.block.block != nil && o.block.block == s.block
}

func (c *checker) withStmt(s *ast.With) {
	for _, rec := range s.Recs {
		op := c.operand(rec, true)
		if isInvalid(op.typ) {
			c.pushWith(nil, rec)
			continue
		}
		r, ok := op.typ.(*recordType)
		if !ok || !op.vr {
			c.errorf(ErrWith)
			c.pushWith(nil, rec)
			continue
		}
		c.pushWith(r, rec)
	}
	c.stmt(s.Body)
	for range s.Recs {
		c.pop()
	}
}

func (c *checker) pushWith(r *recordType, base ast.Expr) {
	c.scope = &scope{parent: c.scope, objs: map[string]*obj{}, block: c.scope.block}
	if r == nil {
		return
	}
	for _, f := range r.fields {
		c.scope.objs[f.name] = &obj{
			name: f.name, kind: kindVar, typ: f.typ, block: c.scope,
			withBase: base, field: f.name, packedField: r.packed, tagField: f.tag,
		}
	}
}

func (c *checker) read(s *ast.Read) {
	name := "read"
	if s.Ln {
		name = "readln"
	}
	o := c.lookup(name)
	if o == nil {
		c.errorf(ErrNotDeclared, name)
		return
	}
	if o.predef == "" {
		args := make([]ast.Expr, len(s.Vars))
		copy(args, s.Vars)
		c.callObj(o, args, true)
		return
	}
	vars := s.Vars
	var file operand
	hasFile := false
	if len(vars) > 0 {
		first := c.operand(vars[0], true)
		if isFile(first.typ) && first.vr {
			file = first
			hasFile = true
			vars = vars[1:]
		}
	}
	if !hasFile {
		if !c.hasInput {
			c.errorf(ErrNeedInput, name)
		}
		c.usedInput = true
		file = operand{typ: textFile, vr: true}
		hasFile = true
	}
	if _, text := file.typ.(textType); text || !hasFile {
		if s.Ln && len(vars) == 0 {
			return
		}
		if !s.Ln && len(vars) == 0 {
			c.errorf(ErrReadVar)
			return
		}
		for _, v := range vars {
			op := c.operand(v, true)
			c.threatObj(op.obj)
			if !op.vr && !isInvalid(op.typ) {
				c.errorf(ErrReadVar)
			}
			c.walkComponent(v, true)
			h := host(op.typ)
			if !isInvalid(op.typ) && h != integerType && h != realType && h != charType {
				c.errorf(ErrReadType)
			}
		}
		return
	}
	if s.Ln {
		c.errorf(ErrReadText)
		return
	}
	ft := file.typ.(*fileType)
	if len(vars) == 0 {
		c.errorf(ErrReadVar)
		return
	}
	for _, v := range vars {
		op := c.operand(v, true)
		c.threatObj(op.obj)
		if !op.vr && !isInvalid(op.typ) {
			c.errorf(ErrReadVar)
		}
		c.forbidComponent(v)
		if !assignCompat(op.typ, ft.elem) {
			c.errorf(ErrReadMismatch)
		}
	}
}

func (c *checker) write(s *ast.Write) {
	name := "write"
	if s.Ln {
		name = "writeln"
	}
	o := c.lookup(name)
	if o == nil {
		c.errorf(ErrNotDeclared, name)
		return
	}
	if o.predef == "" {
		if o.kind != kindProc {
			c.errorf(ErrNotProc, name)
			return
		}
		c.noteCall(s, o)
		var args []ast.Expr
		for _, a := range s.Args {
			if a.Width != nil || a.Frac != nil {
				c.errorf(ErrWriteWidthFile)
			}
			args = append(args, a.Value)
		}
		c.match(o.params, args)
		return
	}
	args := s.Args
	var file operand
	hasFile := false
	if len(args) > 0 && args[0].Width == nil && args[0].Frac == nil {
		first := c.operand(args[0].Value, true)
		if isFile(first.typ) && first.vr {
			file = first
			hasFile = true
			args = args[1:]
		}
	}
	if !hasFile {
		if !c.hasOutput {
			c.errorf(ErrNeedOutput, name)
		}
		c.usedOutput = true
		file = operand{typ: textFile}
	}
	if _, text := file.typ.(textType); text {
		if s.Ln && len(args) == 0 {
			return
		}
		for _, a := range args {
			c.writeArg(a, true)
		}
		return
	}
	if s.Ln {
		c.errorf(ErrWriteText)
		return
	}
	ft := file.typ.(*fileType)
	if len(args) == 0 {
		c.errorf(ErrWriteValue)
		return
	}
	for _, a := range args {
		if a.Width != nil || a.Frac != nil {
			c.errorf(ErrWriteWidthFile)
		}
		op := c.expr(a.Value)
		if !assignCompat(ft.elem, op.typ) {
			c.errorf(ErrWriteType)
		}
	}
}

func (c *checker) writeArg(a ast.WriteArg, text bool) {
	op := c.expr(a.Value)
	if text && !isInvalid(op.typ) && !writable(op.typ) {
		c.errorf(ErrWriteKind)
	}
	if a.Width != nil {
		w := c.expr(a.Width)
		if !isInvalid(w.typ) && host(w.typ) != integerType {
			c.errorf(ErrWriteWidth)
		}
	}
	if a.Frac != nil {
		if host(op.typ) != realType {
			c.errorf(ErrWriteFrac)
		}
		f := c.expr(a.Frac)
		if !isInvalid(f.typ) && host(f.typ) != integerType {
			c.errorf(ErrWriteFracInt)
		}
	}
}

func writable(t Type) bool {
	h := host(t)
	if h == integerType || h == realType || h == booleanType || h == charType {
		return true
	}
	_, ok := stringLen(t)
	return ok
}

func (c *checker) call(name string, args []ast.Expr, asStmt bool) Type {
	o := c.lookup(name)
	if o == nil {
		c.errorf(ErrNotDeclared, name)
		return invalid
	}
	return c.callObj(o, args, asStmt)
}

func (c *checker) callObj(o *obj, args []ast.Expr, asStmt bool) Type {
	if o.predef != "" {
		if o.kind == kindProc && !asStmt {
			c.errorf(ErrIsProc, o.name)
			return invalid
		}
		if o.kind == kindFunc && asStmt {
			c.errorf(ErrIsFunc, o.name)
			return invalid
		}
		return c.predef(o, args, asStmt)
	}
	switch o.kind {
	case kindProc:
		if !asStmt {
			c.errorf(ErrIsProc, o.name)
			return invalid
		}
		c.match(o.params, args)
		return invalid
	case kindFunc:
		if asStmt {
			c.errorf(ErrIsFunc, o.name)
			return invalid
		}
		c.match(o.params, args)
		return o.typ
	default:
		c.errorf(ErrNotRoutine, o.name)
		return invalid
	}
}

func (c *checker) match(params []*parm, args []ast.Expr) {
	if len(params) != len(args) {
		c.errorf(ErrArgCount)
		return
	}
	groups := map[*confSection]confGroup{}
	for i, p := range params {
		switch p.kind {
		case parmVar:
			if p.schema != nil {
				c.confActual(p, args[i], true, groups)
				continue
			}
			op := c.operand(args[i], true)
			c.threatObj(op.obj)
			if !op.vr && !isInvalid(op.typ) {
				c.errorf(ErrVarParam)
			}
			c.forbidComponent(args[i])
			if !sameType(p.typ, op.typ) {
				c.errorf(ErrVarType)
			}
		case parmValue:
			if p.schema != nil {
				c.confActual(p, args[i], false, groups)
				continue
			}
			op := c.expr(args[i])
			if !assignCompat(p.typ, op.typ) {
				c.errorf(ErrValType)
			}
		case parmProc, parmFunc:
			name, ok := args[i].(*ast.Name)
			if !ok {
				c.errorf(ErrProcArg)
				continue
			}
			o := c.lookup(name.Ident)
			if o == nil || o.predef != "" || !congruous(p, o) {
				c.errorf(ErrProcMismatch)
			}
		}
	}
}

// confGroup is the actuals already seen for one conformant section.
type confGroup struct {
	typ    Type
	bounds [][2]int64
	known  bool
}

func (c *checker) confActual(p *parm, arg ast.Expr, byRef bool, groups map[*confSection]confGroup) {
	var op operand
	if byRef {
		op = c.operand(arg, true)
		c.threatObj(op.obj)
		if !op.vr && !isInvalid(op.typ) {
			c.errorf(ErrVarParam)
		}
		c.forbidComponent(arg)
	} else {
		op = c.expr(arg)
	}
	if isInvalid(op.typ) {
		return
	}
	if !conformable(op.typ, p.schema) {
		c.errorf(ErrConfType)
		return
	}
	// A value conformant array copies an array-type. A conformant-array
	// parameter is not an array-type, whether it is itself by value or by
	// reference. It may still be passed to a variable conformant array.
	if !byRef && conformantArray(op.typ) {
		c.errorf(ErrConfValue)
		return
	}
	bounds, known := c.schemaBounds(op.typ, p.schema)
	if prev, seen := groups[p.section]; seen {
		switch {
		case known && prev.known && !sameBounds(prev.bounds, bounds):
			c.errorf(ErrConfBounds)
		case !sameType(prev.typ, op.typ):
			c.errorf(ErrConfSame)
		}
		return
	}
	groups[p.section] = confGroup{typ: op.typ, bounds: bounds, known: known}
}

func conformantArray(t Type) bool {
	a, ok := t.(*arrayType)
	return ok && a.conformant
}

func conformable(actual Type, schema *confSchema) bool {
	a, ok := actual.(*arrayType)
	if !ok || schema == nil || a.packed != schema.packed {
		return false
	}
	if !compatible(a.index, schema.index) {
		return false
	}
	if schema.nested != nil {
		return conformable(a.elem, schema.nested)
	}
	return sameType(a.elem, schema.elem)
}

// schemaBounds reports the actual index bounds when each is known, and reports
// a bound that falls outside the schema's ordinal type.
func (c *checker) schemaBounds(actual Type, schema *confSchema) ([][2]int64, bool) {
	var out [][2]int64
	for schema != nil {
		a, ok := actual.(*arrayType)
		if !ok {
			return nil, false
		}
		lo, hi, known := indexLimits(a.index)
		if !known {
			return nil, false
		}
		if !(&value{known: true, n: lo}).fits(schema.index) || !(&value{known: true, n: hi}).fits(schema.index) {
			c.errorf(ErrConfRange)
		}
		out = append(out, [2]int64{lo, hi})
		actual = a.elem
		schema = schema.nested
	}
	return out, true
}

func indexLimits(t Type) (int64, int64, bool) {
	if s, ok := t.(*subrangeType); ok {
		if s.lo == nil || s.hi == nil || !s.lo.known || !s.hi.known {
			return 0, 0, false
		}
		return s.lo.n, s.hi.n, true
	}
	switch h := host(t).(type) {
	case *enumType:
		if len(h.names) == 0 {
			return 0, 0, false
		}
		return 0, int64(len(h.names) - 1), true
	case *basic:
		switch h {
		case booleanType:
			return 0, 1, true
		case charType:
			return 0, unicode.MaxRune, true
		}
	}
	return 0, 0, false
}

func sameBounds(a, b [][2]int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sameSchema(a, b *confSchema) bool {
	if a == nil || b == nil || a.packed != b.packed || !sameType(a.index, b.index) {
		return false
	}
	if a.nested != nil || b.nested != nil {
		return sameSchema(a.nested, b.nested)
	}
	return sameType(a.elem, b.elem)
}

func congruous(p *parm, o *obj) bool {
	if p.kind == parmProc && o.kind != kindProc || p.kind == parmFunc && o.kind != kindFunc {
		return false
	}
	if p.kind == parmFunc && !sameType(p.result, o.typ) {
		return false
	}
	return congruousParams(p.params, o.params)
}

func congruousParm(a, b *parm) bool {
	if a.kind != b.kind {
		return false
	}
	if a.kind == parmFunc && !sameType(a.result, b.result) {
		return false
	}
	return congruousParams(a.params, b.params)
}

// congruousParams reports that two parameter lists group their parameters
// into the same sections. var a, b: integer is one section; var a: integer;
// var b: integer is two.
func congruousParams(a, b []*parm) bool {
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		if i >= len(a) || j >= len(b) {
			return false
		}
		na, nb := sectionLen(a, i), sectionLen(b, j)
		if na != nb {
			return false
		}
		for k := 0; k < na; k++ {
			if !congruousOne(a[i+k], b[j+k]) {
				return false
			}
		}
		i += na
		j += nb
	}
	return true
}

func sectionLen(ps []*parm, i int) int {
	sec := ps[i].section
	n := 1
	for i+n < len(ps) && ps[i+n].section == sec {
		n++
	}
	return n
}

func congruousOne(a, b *parm) bool {
	if a.kind != b.kind {
		return false
	}
	switch a.kind {
	case parmValue, parmVar:
		return sameParmType(a, b)
	default:
		return congruousParm(a, b)
	}
}

func sameParmType(a, b *parm) bool {
	if a.schema != nil || b.schema != nil {
		return sameSchema(a.schema, b.schema)
	}
	return sameType(a.typ, b.typ)
}

func (c *checker) threatObj(o *obj) {
	if o != nil && c.controlled(o) {
		c.errorf(ErrThreaten, o.name)
	}
}

// forbidComponent rejects a variable parameter that is a tag field or a
// component of a packed variable. A file or a pointer still has its own
// identity inside a packed structure; new and reset receive those in the
// ISO acceptance test. readChar allows one character of a string type,
// which textfile read accepts.
func (c *checker) forbidComponent(e ast.Expr) {
	c.walkComponent(e, false)
}

func packedCarrier(t Type) bool {
	if isFile(t) {
		return true
	}
	_, ok := t.(*pointerType)
	return ok
}

func (c *checker) walkComponent(e ast.Expr, readChar bool) {
	if e == nil {
		return
	}
	if op, ok := c.raw[e]; ok && op.obj != nil && op.obj.withBase != nil {
		if op.obj.tagField {
			c.errorf(ErrTagVar)
			return
		}
		if op.obj.packedField && !packedCarrier(op.typ) {
			c.errorf(ErrPackedVar)
			return
		}
	}
	switch e := e.(type) {
	case *ast.Select:
		base, ok := c.raw[e.Base]
		rec, rok := base.typ.(*recordType)
		if ok && rok {
			for _, f := range rec.fields {
				if f.name != e.Field {
					continue
				}
				if f.tag {
					c.errorf(ErrTagVar)
					return
				}
				if rec.packed && !packedCarrier(f.typ) {
					c.errorf(ErrPackedVar)
					return
				}
				break
			}
		}
		c.walkComponent(e.Base, readChar)
	case *ast.Indexed:
		if base, ok := c.raw[e.Base]; ok {
			if a, ok := base.typ.(*arrayType); ok && a.packed && !packedCarrier(a.elem) {
				// Textfile read may store into one character of a string.
				// A variable parameter may not.
				_, str := stringLen(base.typ)
				if !str || !readChar {
					c.errorf(ErrPackedVar)
					return
				}
			}
		}
		c.walkComponent(e.Base, readChar)
	}
}

func (c *checker) predef(o *obj, args []ast.Expr, asStmt bool) Type {
	switch o.predef {
	case "abs", "sqr":
		return c.numericFunc(args, true)
	case "sin", "cos", "exp", "ln", "sqrt", "arctan":
		return c.numericFunc(args, false)
	case "trunc", "round":
		if !c.oneReal(o.predef, args) {
			return invalid
		}
		return integerType
	case "ord":
		if len(args) != 1 {
			c.errorf(ErrOrdArgs)
			return invalid
		}
		op := c.expr(args[0])
		if !isInvalid(op.typ) && !ordinal(op.typ) {
			c.errorf(ErrOrdType)
		}
		return integerType
	case "chr":
		if !c.oneInteger(args) {
			return invalid
		}
		return charType
	case "succ", "pred":
		if len(args) != 1 {
			c.errorf(ErrOneArg, o.predef)
			return invalid
		}
		op := c.expr(args[0])
		if !isInvalid(op.typ) && !ordinal(op.typ) {
			c.errorf(ErrNeedOrdinal, o.predef)
			return invalid
		}
		return op.typ
	case "odd":
		if !c.oneInteger(args) {
			return invalid
		}
		return booleanType
	case "eof":
		c.fileArg(args, false)
		return booleanType
	case "eoln":
		c.fileArg(args, true)
		return booleanType
	case "rewrite", "reset", "get", "put":
		c.fileVar(args, false)
		return invalid
	case "page":
		c.fileVar(args, true)
		return invalid
	case "new", "dispose":
		c.newDispose(o.predef, args)
		return invalid
	case "pack", "unpack":
		c.transfer(o.predef, args)
		return invalid
	case "read", "write":
		c.errorf(ErrNeedList, o.predef)
		return invalid
	case "readln":
		if len(args) != 0 {
			c.errorf(ErrReadList)
			return invalid
		}
		if !c.hasInput {
			c.errorf(ErrReadInput)
		}
		c.usedInput = true
		return invalid
	case "writeln":
		if len(args) != 0 {
			c.errorf(ErrWriteList)
			return invalid
		}
		if !c.hasOutput {
			c.errorf(ErrWriteOutput)
		}
		c.usedOutput = true
		return invalid
	default:
		c.errorf(ErrBadPredef, o.predef)
		return invalid
	}
}

func (c *checker) predefFunc(name string, args []ast.Expr) Type {
	o := c.lookup(name)
	if o == nil {
		return invalid
	}
	return c.predef(o, args, false)
}

func (c *checker) numericFunc(args []ast.Expr, keep bool) Type {
	if len(args) != 1 {
		c.errorf(ErrFuncArgs)
		return invalid
	}
	op := c.expr(args[0])
	if !isInvalid(op.typ) && !numeric(op.typ) {
		c.errorf(ErrFuncNum)
		return invalid
	}
	if keep && sameType(host(op.typ), integerType) {
		return integerType
	}
	return realType
}

func (c *checker) oneReal(name string, args []ast.Expr) bool {
	if len(args) != 1 {
		c.errorf(ErrFuncArgs)
		return false
	}
	op := c.expr(args[0])
	if !isInvalid(op.typ) && host(op.typ) != realType {
		c.errorf(ErrNeedReal, name)
		return false
	}
	return true
}

func (c *checker) oneInteger(args []ast.Expr) bool {
	if len(args) != 1 {
		c.errorf(ErrFuncArgs)
		return false
	}
	op := c.expr(args[0])
	if !isInvalid(op.typ) && host(op.typ) != integerType {
		c.errorf(ErrFuncInt)
		return false
	}
	return true
}

func (c *checker) fileArg(args []ast.Expr, textOnly bool) {
	if len(args) == 0 {
		if !c.hasInput {
			c.errorf(ErrInput)
		}
		c.usedInput = true
		return
	}
	if len(args) != 1 {
		c.errorf(ErrArgCount)
		return
	}
	op := c.operand(args[0], true)
	if !op.vr || !isFile(op.typ) {
		c.errorf(ErrNeedFile)
		return
	}
	c.forbidComponent(args[0])
	if textOnly {
		if _, ok := op.typ.(textType); !ok {
			c.errorf(ErrNeedText)
		}
	}
}

func (c *checker) fileVar(args []ast.Expr, textOnly bool) {
	if len(args) == 0 && textOnly {
		if !c.hasOutput {
			c.errorf(ErrPage)
		}
		c.usedOutput = true
		return
	}
	c.fileArg(args, textOnly)
}

// transfer checks pack(a, i, z) and unpack(z, a, i). a is an unpacked array
// variable, z a packed array variable, and both arrays have the same
// component type. i is assignment-compatible with the index type of a.
func (c *checker) transfer(name string, args []ast.Expr) {
	if len(args) != 3 {
		c.errorf(ErrThree, name)
		return
	}
	var unpackedExpr, packedExpr, indexExpr ast.Expr
	if name == "pack" {
		unpackedExpr, indexExpr, packedExpr = args[0], args[1], args[2]
	} else {
		packedExpr, unpackedExpr, indexExpr = args[0], args[1], args[2]
	}
	unpacked := c.transferArray(name, unpackedExpr, false, name == "unpack")
	packed := c.transferArray(name, packedExpr, true, name == "pack")
	if unpacked != nil && packed != nil && !sameType(unpacked.elem, packed.elem) {
		c.errorf(ErrPackComp, name)
	}
	if unpacked == nil {
		c.expr(indexExpr)
		return
	}
	index := c.expr(indexExpr)
	if !isInvalid(index.typ) && !assignCompat(unpacked.index, index.typ) {
		c.errorf(ErrPackIndex, name)
	} else if index.val != nil && !index.val.fits(unpacked.index) {
		c.errorf(ErrPackRange, name)
	} else if packed != nil && index.val != nil && index.val.known && !packSpan(unpacked, packed, index.val.n) {
		c.errorf(ErrPackSpan, name)
	}
}

// packSpan reports whether n elements of the packed array fit in the unpacked
// array starting at start. Unknown bounds are left for run time.
func packSpan(unpacked, packed *arrayType, start int64) bool {
	_, uhi, uok := ordinalSpan(unpacked.index)
	plo, phi, pok := ordinalSpan(packed.index)
	if !uok || !pok {
		return true
	}
	n, nok := fitLen(plo, phi)
	if !nok {
		return false
	}
	room, rok := fitLen(start, uhi)
	if !rok {
		return false
	}
	return room >= n
}

func (c *checker) transferArray(name string, e ast.Expr, packed, dest bool) *arrayType {
	op := c.operand(e, true)
	if dest {
		c.threatObj(op.obj)
	}
	if !op.vr && !isInvalid(op.typ) {
		c.errorf(ErrNeedVar, name)
	}
	a, ok := op.typ.(*arrayType)
	if !ok {
		if !isInvalid(op.typ) {
			c.errorf(ErrNeedArray, name)
		}
		return nil
	}
	if a.packed != packed {
		if packed {
			c.errorf(ErrNeedPacked, name)
		} else {
			c.errorf(ErrNeedUnpacked, name)
		}
	}
	return a
}

// newDispose checks new(p, c1, …, cn) and dispose(p, c1, …, cn). Each constant
// selects a variant of the record the previous constant selected. c1 selects
// a variant of p's record.
func (c *checker) newDispose(name string, args []ast.Expr) {
	if len(args) < 1 {
		c.errorf(ErrNeedPtr, name)
		return
	}
	op := c.operand(args[0], true)
	c.threatObj(op.obj)
	_, isPtr := op.typ.(*pointerType)
	if !op.vr && !isInvalid(op.typ) && !(name == "dispose" && (c.opt.DisposeExpr || isPtr)) {
		c.errorf(ErrNeedPtr, name)
	}
	c.forbidComponent(args[0])
	p, ok := op.typ.(*pointerType)
	if !ok {
		if !isInvalid(op.typ) {
			c.errorf(ErrNeedPtr, name)
		}
		for _, a := range args[1:] {
			c.constant(a)
		}
		return
	}
	if len(args) == 1 {
		return
	}
	rec, ok := p.base.(*recordType)
	if !ok || rec.variant == nil {
		c.errorf(ErrNeedVariant, name)
		for _, a := range args[1:] {
			c.constant(a)
		}
		return
	}
	part := rec.variant
	dead := false
	for _, a := range args[1:] {
		v := c.constant(a)
		if dead {
			continue
		}
		if part == nil {
			c.errorf(ErrTagArgs, name)
			dead = true
			continue
		}
		if v == nil || isInvalid(v.typ) || !v.known || !ordinal(v.typ) || !compatible(v.typ, part.tag) {
			if v == nil || !isInvalid(v.typ) {
				c.errorf(ErrNewTag, name)
			}
			dead = true
			continue
		}
		if !v.fits(part.tag) {
			c.errorf(ErrTagRange, name)
			dead = true
			continue
		}
		arm := variantByLabel(part, v.n)
		if arm == nil {
			c.errorf(ErrNoVariant, name)
			dead = true
			continue
		}
		part = arm.next
	}
}

func variantByLabel(part *variantPart, n int64) *variantArm {
	for i := range part.arms {
		for _, lab := range part.arms[i].labels {
			if lab == n {
				return &part.arms[i]
			}
		}
	}
	return nil
}

func (c *checker) collectFors(stmts []ast.Stmt) []*obj {
	var out []*obj
	var walk func(ast.Stmt)
	walk = func(s ast.Stmt) {
		if s == nil {
			return
		}
		switch s := s.(type) {
		case *ast.For:
			if o := c.lookup(s.Name); o != nil && o.kind == kindVar && o.fromVar && varInBlock(o, c.scope) {
				out = append(out, o)
			}
			walk(s.Body)
		case *ast.Labeled:
			walk(s.Stmt)
		case *ast.Compound:
			for _, st := range s.Body {
				walk(st)
			}
		case *ast.If:
			walk(s.Then)
			walk(s.Else)
		case *ast.While:
			walk(s.Body)
		case *ast.Repeat:
			for _, st := range s.Body {
				walk(st)
			}
		case *ast.Case:
			for _, arm := range s.Arms {
				walk(arm.Stmt)
			}
		case *ast.With:
			walk(s.Body)
		}
	}
	for _, s := range stmts {
		walk(s)
	}
	return out
}

func (c *checker) resolveGoto(g pendingGoto) {
	defer c.locate(g.at)()
	b := g.block
	var site *seq
	var home *blockInfo
	for b != nil {
		if b.declared[g.name] {
			site = b.sites[g.name]
			home = b
			break
		}
		b = b.parent
	}
	if home == nil {
		c.errorf(ErrUndefLabel, g.name)
		return
	}
	if site == nil {
		return
	}
	if containsSeq(site, g.seq) {
		return
	}
	if site == home.top && encloses(home, g.block) {
		if home != g.block {
			c.farGoto = true
		}
		return
	}
	c.errorf(ErrGoto, g.name)
}

func containsSeq(outer, inner *seq) bool {
	for s := inner; s != nil; s = s.parent {
		if s == outer {
			return true
		}
	}
	return false
}

func encloses(outer, inner *blockInfo) bool {
	for b := inner; b != nil; b = b.parent {
		if b == outer {
			return true
		}
	}
	return false
}
