package check

import (
	"fmt"
	"strconv"

	"gopas/internal/ast"
)

// Error is a violation of Standard Pascal that must be reported before the
// program runs. Line and Col are 1-based when the construct was located.
type Error struct {
	Code Code
	Line int
	Col  int
	Msg  string
}

func (e Error) Error() string {
	msg := e.Msg
	if e.Code != 0 {
		msg = fmt.Sprintf("%d: %s", e.Code, msg)
	}
	if e.Line == 0 {
		return msg
	}
	return fmt.Sprintf("%d:%d: %s", e.Line, e.Col, msg)
}

// Diagnostics are values, so Error itself is the error form.
var _ error = Error{}

type objKind int

const (
	kindConst objKind = iota
	kindType
	kindVar
	kindProc
	kindFunc
	kindField
)

type obj struct {
	name     string
	kind     objKind
	typ      Type
	val      *value
	params   []*parm
	predef   string
	forward  bool
	done     bool
	param    bool
	fromVar  bool
	byRef    bool
	block    *scope
	at       any // declaration, when a later check must point back at it
	withBase ast.Expr
	field    string
	schema   *confSchema
	bounds   []*obj
	// blocked is a use before the identifier's defining point.
	blocked bool
	// bound is a conformant-array bound-identifier. Its value is fixed
	// for an activation and is not a constant.
	bound bool
	// open is a type identifier whose definition is still being checked.
	open bool
	// ptrs are pointer types whose domain is this still-open type.
	ptrs []*pointerType
	// assigned is true once a function's body stores its result.
	assigned bool
	// inner is the scope of a routine's parameters and locals.
	inner *scope
	// packedField and tagField describe a field introduced by with.
	packedField bool
	tagField    bool
}

type parmKind int

const (
	parmValue parmKind = iota
	parmVar
	parmProc
	parmFunc
)

type parm struct {
	name    string
	kind    parmKind
	typ     Type
	params  []*parm
	result  Type
	schema  *confSchema
	section *confSection
	bounds  []*obj
}

// confSchema is the conformant-array-schema a parameter was declared with.
// index is the ordinal type of this dimension. nested is the component
// schema; otherwise elem is the component type.
type confSchema struct {
	packed bool
	index  Type
	elem   Type
	nested *confSchema
}

// confSection identifies one formal-parameter-section. Names written
// together, as in var a, b: integer, share a section. A procedural
// parameter is congruous only when the sections group the same way.
// Conformant arrays in one section share it so their bounds are checked
// together. The field keeps the struct from being empty: pointers to an
// empty struct can all be the same address, and that address is the map key.
type confSection struct{ distinct byte }

type scope struct {
	parent *scope
	objs   map[string]*obj
	block  *blockInfo
	// pending names are declared in this block and not yet at their defining point.
	pending map[string]bool
}

type blockInfo struct {
	parent   *blockInfo
	declared map[string]bool
	sites    map[string]*seq
	labelAt  map[string]any
	top      *seq
}

type seq struct {
	parent   *seq
	block    *blockInfo
	blockTop bool
}

type pendingGoto struct {
	name  string
	seq   *seq
	block *blockInfo
	at    ast.Stmt
}

type checker struct {
	errs       []Error
	scope      *scope
	seq        *seq
	fns        []*obj
	controls   []*obj
	waiting    map[string][]*pointerType
	section    map[string]bool
	gotos      []pendingGoto
	farGoto    bool
	hasInput   bool
	hasOutput  bool
	usedInput  bool
	usedOutput bool
	raw        map[ast.Expr]operand
	typeOf     map[ast.Type]Type
	callOf     map[any]*obj
	sigOf      map[ast.Routine]*obj
	bodyOf     map[ast.Routine]bool
	forOf      map[*ast.For]*obj
	// records are the record types currently being defined, innermost last.
	// A field identifier's region is its whole record, including text above
	// its defining-point and records nested inside it.
	records   []*recFields
	opt       Options
	line, col int
}

// recFields is the field identifiers of one record type being defined.
type recFields struct {
	all     map[string]bool
	defined map[string]bool
}

func newChecker() *checker {
	return &checker{
		waiting: map[string][]*pointerType{},
		raw:     map[ast.Expr]operand{},
		typeOf:  map[ast.Type]Type{},
		callOf:  map[any]*obj{},
		sigOf:   map[ast.Routine]*obj{},
		bodyOf:  map[ast.Routine]bool{},
		forOf:   map[*ast.For]*obj{},
	}
}

// Check resolves names and reports violations in p. Level 1 includes
// conformant-array parameters.
func Check(p *ast.Program) []Error {
	c := newChecker()
	c.run(p)
	return c.errs
}

func (c *checker) run(p *ast.Program) {
	c.scope = universe()
	c.pushBlock()
	seen := map[string]bool{}
	for i, name := range p.Files {
		at := fileAt(p, i)
		if seen[name] {
			c.at(at, func() { c.errorf(ErrDupProgramParam, name) })
			continue
		}
		seen[name] = true
		if name == "input" {
			c.hasInput = true
			c.insert(&obj{name: name, kind: kindVar, typ: textFile, fromVar: false, block: c.scope})
		}
		if name == "output" {
			c.hasOutput = true
			c.insert(&obj{name: name, kind: kindVar, typ: textFile, fromVar: false, block: c.scope})
		}
	}
	c.block(p.Labels, p.LabelAt, p.Consts, p.Types, p.Vars, p.Routines, p.Body)
	if p.MissingEnd != nil {
		c.at(p.MissingEnd, func() { c.errorf(ErrSyntax, "'end'") })
	}
	for i, name := range p.Files {
		if name == "input" || name == "output" {
			continue
		}
		at := fileAt(p, i)
		o := c.lookup(name)
		if o == nil || o.kind != kindVar || !isFile(o.typ) {
			c.at(at, func() { c.errorf(ErrProgramParamFile, name) })
		}
	}
	c.pop()
	for _, g := range c.gotos {
		c.resolveGoto(g)
	}
}

func (c *checker) noteCall(k any, o *obj) {
	if k != nil {
		c.callOf[k] = o
	}
}

func fileAt(p *ast.Program, i int) any {
	if p == nil || i < 0 || i >= len(p.FileAt) {
		return nil
	}
	return p.FileAt[i]
}

func (c *checker) errorf(code Code, args ...any) {
	err := Error{Code: code, Line: c.line, Col: c.col, Msg: code.format(args...)}
	if n := len(c.errs); n > 0 && c.errs[n-1] == err {
		return
	}
	c.errs = append(c.errs, err)
}

// locate reports the following errors at n, then restores the previous place.
func (c *checker) locate(n any) func() {
	prevL, prevC := c.line, c.col
	if p := ast.At(n); p.Line != 0 {
		c.line, c.col = p.Line, p.Col
	}
	return func() { c.line, c.col = prevL, prevC }
}

func (c *checker) at(n any, fn func()) {
	defer c.locate(n)()
	fn()
}

func (c *checker) pushBlock() {
	b := &blockInfo{
		parent:   nil,
		declared: map[string]bool{},
		sites:    map[string]*seq{},
	}
	if c.scope != nil && c.scope.block != nil {
		b.parent = c.scope.block
	}
	c.scope = &scope{parent: c.scope, objs: map[string]*obj{}, block: b}
	c.seq = &seq{block: b, blockTop: true}
	b.top = c.seq
}

func (c *checker) pop() {
	if c.scope != nil {
		c.scope = c.scope.parent
	}
}

func (c *checker) pushSeq() {
	c.seq = &seq{parent: c.seq, block: c.scope.block}
}

func (c *checker) insert(o *obj) bool {
	if o.block == nil {
		o.block = c.scope
	}
	if c.scope.pending != nil {
		delete(c.scope.pending, o.name)
	}
	if _, ok := c.scope.objs[o.name]; ok {
		c.errorf(ErrDupIdent, o.name)
		return false
	}
	c.scope.objs[o.name] = o
	return true
}

func (c *checker) lookup(name string) *obj {
	if o, ok := c.recordField(name); ok {
		return o
	}
	for s := c.scope; s != nil; s = s.parent {
		if s.pending[name] {
			c.errorf(ErrBeforeDef, name)
			return &obj{name: name, blocked: true, typ: invalid, val: &value{typ: invalid}}
		}
		if o, ok := s.objs[name]; ok {
			return o
		}
	}
	return nil
}

// hold marks names whose defining point is still ahead in this block.
// A use before that point must not see a declaration in an outer block.
func (c *checker) hold(name string) {
	if name == "" {
		return
	}
	if c.scope.pending == nil {
		c.scope.pending = map[string]bool{}
	}
	c.scope.pending[name] = true
}

func (c *checker) holdBlock(consts []*ast.ConstDef, types []*ast.TypeDef, vars []*ast.Decl, routines []ast.Routine) {
	for _, d := range consts {
		c.hold(d.Name)
	}
	for _, d := range types {
		c.hold(d.Name)
		holdEnums(d.Type, c.hold)
	}
	for _, d := range vars {
		for _, name := range d.Names {
			c.hold(name)
		}
		// An enumeration in a variable type has its defining-point here.
		// Its constants belong to the whole block, including text above.
		holdEnums(d.Type, c.hold)
	}
	for _, r := range routines {
		switch r := r.(type) {
		case *ast.Proc:
			c.hold(r.Name)
		case *ast.Func:
			c.hold(r.Name)
		}
	}
}

func holdEnums(t ast.Type, hold func(string)) {
	switch t := t.(type) {
	case *ast.Enum:
		for _, name := range t.Names {
			hold(name)
		}
	case *ast.Array:
		for _, ix := range t.Index {
			holdEnums(ix, hold)
		}
		holdEnums(t.Elem, hold)
	case *ast.Record:
		if t.Fields != nil {
			holdFields(t.Fields, hold)
		}
	case *ast.Set:
		holdEnums(t.Base, hold)
	case *ast.File:
		holdEnums(t.Elem, hold)
	}
}

func holdFields(list *ast.FieldList, hold func(string)) {
	for _, f := range list.Fixed {
		holdEnums(f.Type, hold)
	}
	if list.Variant == nil {
		return
	}
	for _, arm := range list.Variant.Arms {
		if arm.Fields != nil {
			holdFields(arm.Fields, hold)
		}
	}
}

func (c *checker) block(labels []string, labelAt []*ast.Name, consts []*ast.ConstDef, types []*ast.TypeDef, vars []*ast.Decl, routines []ast.Routine, body []ast.Stmt) {
	for i, lab := range labels {
		var at any
		if i < len(labelAt) {
			at = labelAt[i]
		}
		c.at(at, func() {
			if c.scope.block.declared[lab] {
				c.errorf(ErrDupLabel, lab)
				return
			}
			c.scope.block.declared[lab] = true
			if c.scope.block.labelAt == nil {
				c.scope.block.labelAt = map[string]any{}
			}
			c.scope.block.labelAt[lab] = at
		})
	}
	c.holdBlock(consts, types, vars, routines)
	for _, d := range consts {
		c.at(d, func() {
			v := c.constant(d.Value)
			if v == nil {
				v = &value{typ: invalid}
			}
			c.insert(&obj{name: d.Name, kind: kindConst, typ: v.typ, val: v})
		})
	}
	prevWaiting, prevSection := c.waiting, c.section
	c.waiting = map[string][]*pointerType{}
	c.section = map[string]bool{}
	for _, d := range types {
		c.section[d.Name] = true
	}
	for _, d := range types {
		c.at(d, func() {
			o := &obj{name: d.Name, kind: kindType, open: true}
			if !c.insert(o) {
				return
			}
			t := c.denote(d.Type)
			o.typ = t
			o.open = false
			for _, p := range o.ptrs {
				p.base = t
			}
			for _, p := range c.waiting[d.Name] {
				p.base = t
			}
			delete(c.waiting, d.Name)
		})
	}
	for name := range c.waiting {
		if c.section[name] {
			c.errorf(ErrNotDeclared, name)
			delete(c.waiting, name)
		}
	}
	c.waiting, c.section = prevWaiting, prevSection
	for _, d := range vars {
		c.at(d, func() {
			t := c.denote(d.Type)
			for _, name := range d.Names {
				c.insert(&obj{name: name, kind: kindVar, typ: t, fromVar: true})
			}
		})
	}
	fors := c.collectFors(body)
	prev := c.controls
	c.controls = append(append([]*obj{}, prev...), fors...)
	for _, r := range routines {
		c.routine(r)
	}
	c.controls = prev
	c.stmts(body)
	for name, ok := range c.scope.block.declared {
		if ok && c.scope.block.sites[name] == nil {
			c.at(c.scope.block.labelAt[name], func() { c.errorf(ErrLabelNoStmt, name) })
		}
	}
	for _, o := range c.scope.objs {
		if o.forward && !o.done {
			c.at(o.at, func() {
				if p := ast.NamePos(o.at); p.Line != 0 {
					c.line, c.col = p.Line, p.Col
				}
				c.errorf(ErrForward, o.name)
			})
		}
	}
}

func (c *checker) routine(r ast.Routine) {
	switch r := r.(type) {
	case *ast.Proc:
		c.proc(r)
	case *ast.Func:
		c.fun(r)
	}
}

func (c *checker) proc(p *ast.Proc) {
	defer c.locate(p)()
	if !p.Forward {
		if o := c.scope.objs[p.Name]; o != nil && o.kind == kindProc && o.forward && !o.done && len(p.Params) == 0 {
			o.done = true
			o.forward = false
			c.sigOf[p] = o
			c.bodyOf[p] = true
			c.routineBody(o, p.Labels, p.LabelAt, p.Consts, p.Types, p.Vars, p.Routines, p.Body)
			return
		}
	}
	params := c.signature(p.Params)
	o := &obj{name: p.Name, kind: kindProc, params: params, forward: p.Forward, done: !p.Forward, at: p}
	if !c.insert(o) {
		return
	}
	c.sigOf[p] = o
	c.bodyOf[p] = !p.Forward
	if !p.Forward {
		c.routineBody(o, p.Labels, p.LabelAt, p.Consts, p.Types, p.Vars, p.Routines, p.Body)
	}
}

func (c *checker) fun(f *ast.Func) {
	defer c.locate(f)()
	if !f.Forward && f.Result == "" && len(f.Params) == 0 {
		if o := c.scope.objs[f.Name]; o != nil && o.kind == kindFunc && o.forward && !o.done {
			o.done = true
			o.forward = false
			c.sigOf[f] = o
			c.bodyOf[f] = true
			c.fns = append(c.fns, o)
			c.routineBody(o, f.Labels, f.LabelAt, f.Consts, f.Types, f.Vars, f.Routines, f.Body)
			c.fns = c.fns[:len(c.fns)-1]
			if !o.assigned {
				c.errorf(ErrFuncResult, o.name)
			}
			return
		}
		c.errorf(ErrNoResult, f.Name)
	}
	var result Type = invalid
	if f.Result != "" {
		result = c.typeName(f.Result)
		if !isInvalid(result) && !simpleOrPointer(result) {
			c.errorf(ErrResultType, f.Name)
		}
	}
	params := c.signature(f.Params)
	o := &obj{name: f.Name, kind: kindFunc, typ: result, params: params, forward: f.Forward, done: !f.Forward, at: f}
	if !c.insert(o) {
		return
	}
	c.sigOf[f] = o
	c.bodyOf[f] = !f.Forward
	if !f.Forward {
		c.fns = append(c.fns, o)
		c.routineBody(o, f.Labels, f.LabelAt, f.Consts, f.Types, f.Vars, f.Routines, f.Body)
		c.fns = c.fns[:len(c.fns)-1]
		if !o.assigned {
			c.errorf(ErrFuncResult, o.name)
		}
	}
}

func simpleOrPointer(t Type) bool {
	switch host(t).(type) {
	case *basic, *enumType, *pointerType:
		return true
	default:
		return false
	}
}

func (c *checker) routineBody(o *obj, labels []string, labelAt []*ast.Name, consts []*ast.ConstDef, types []*ast.TypeDef, vars []*ast.Decl, routines []ast.Routine, body []ast.Stmt) {
	prev := c.seq
	c.pushBlock()
	o.inner = c.scope
	for _, p := range o.params {
		c.bindParm(p)
	}
	c.block(labels, labelAt, consts, types, vars, routines, body)
	c.pop()
	c.seq = prev
}

func (c *checker) bindParm(p *parm) {
	for _, b := range p.bounds {
		if _, ok := c.scope.objs[b.name]; ok {
			continue
		}
		c.insert(b)
	}
	switch p.kind {
	case parmProc:
		c.insert(&obj{name: p.name, kind: kindProc, params: p.params, param: true})
	case parmFunc:
		c.insert(&obj{name: p.name, kind: kindFunc, typ: p.result, params: p.params, param: true})
	default:
		c.insert(&obj{
			name: p.name, kind: kindVar, typ: p.typ, param: true,
			byRef: p.kind == parmVar, schema: p.schema, bounds: p.bounds,
		})
	}
}

func (c *checker) signature(formals []ast.Formal) []*parm {
	c.openParams()
	defer c.closeParams()
	// The whole formal parameter list is the region of each parameter and
	// bound identifier. A use before its defining-point must not see an
	// outer declaration of the same spelling.
	for _, name := range paramNames(formals) {
		c.hold(name)
	}
	var out []*parm
	for _, f := range formals {
		c.at(f, func() { c.formal(f, &out) })
	}
	return out
}

func paramNames(formals []ast.Formal) []string {
	var names []string
	for _, f := range formals {
		switch f := f.(type) {
		case *ast.Param:
			names = append(names, f.Names...)
		case *ast.ConfParam:
			names = append(names, f.Names...)
			names = append(names, boundNames(f.Schema)...)
		case *ast.ProcParam:
			names = append(names, f.Name)
		case *ast.FuncParam:
			names = append(names, f.Name)
		}
	}
	return names
}

func boundNames(s *ast.ConfSchema) []string {
	if s == nil {
		return nil
	}
	var names []string
	for _, ix := range s.Indexes {
		names = append(names, ix.Low, ix.High)
	}
	return append(names, boundNames(s.Nested)...)
}

func (c *checker) formal(f ast.Formal, out *[]*parm) {
	switch f := f.(type) {
	case *ast.Param:
		c.declareParams(f.Names)
		t := c.typeName(f.Type)
		if !f.ByRef && containsFile(t) {
			c.errorf(ErrFileParam)
		}
		kind := parmValue
		if f.ByRef {
			kind = parmVar
		}
		sec := &confSection{}
		for _, name := range f.Names {
			*out = append(*out, &parm{name: name, kind: kind, typ: t, section: sec})
		}
	case *ast.ConfParam:
		c.declareParams(f.Names)
		arr, schema, bounds := c.confSchema(f.Schema)
		if !f.ByRef && containsFile(arr) {
			c.errorf(ErrFileParam)
		}
		kind := parmValue
		if f.ByRef {
			kind = parmVar
		}
		sec := &confSection{}
		for _, name := range f.Names {
			*out = append(*out, &parm{
				name: name, kind: kind, typ: arr, schema: schema,
				section: sec, bounds: bounds,
			})
		}
	case *ast.ProcParam:
		c.declareParams([]string{f.Name})
		*out = append(*out, &parm{name: f.Name, kind: parmProc, params: c.signature(f.Params), section: &confSection{}})
	case *ast.FuncParam:
		c.declareParams([]string{f.Name})
		result := c.typeName(f.Result)
		if !isInvalid(result) && !simpleOrPointer(result) {
			c.errorf(ErrResultType, f.Name)
		}
		*out = append(*out, &parm{name: f.Name, kind: parmFunc, params: c.signature(f.Params), result: result, section: &confSection{}})
	}
}

// openParams opens the formal parameter list. closeParams drops that scope.
// The objects kept on a parm, the bound identifiers, have their block cleared
// so the procedure body can insert them.
func (c *checker) openParams() {
	var block *blockInfo
	if c.scope != nil {
		block = c.scope.block
	}
	c.scope = &scope{parent: c.scope, objs: map[string]*obj{}, block: block}
}

func (c *checker) closeParams() {
	for _, o := range c.scope.objs {
		o.block = nil
	}
	c.pop()
}

func (c *checker) declareParams(names []string) {
	for _, name := range names {
		c.insert(&obj{name: name, kind: kindVar})
	}
}

func (c *checker) confSchema(s *ast.ConfSchema) (Type, *confSchema, []*obj) {
	if s == nil || len(s.Indexes) == 0 {
		c.errorf(ErrConfSchema)
		return invalid, nil, nil
	}
	idx := s.Indexes[0]
	lo := c.bindBound(idx.Low)
	hi := c.bindBound(idx.High)
	ord := c.typeName(idx.Type)
	if !isInvalid(ord) && !ordinal(ord) {
		c.errorf(ErrIndexOrdinal, idx.Type)
		ord = invalid
	}
	c.noteBound(lo, ord)
	c.noteBound(hi, ord)
	var elem Type
	var nested *confSchema
	var more []*obj
	switch {
	case len(s.Indexes) > 1:
		rest := *s
		rest.Packed = false
		rest.Indexes = append([]ast.ConfIndex(nil), s.Indexes[1:]...)
		elem, nested, more = c.confSchema(&rest)
	case s.Nested != nil:
		elem, nested, more = c.confSchema(s.Nested)
	default:
		elem = c.typeName(s.Elem)
	}
	arr := &arrayType{
		packed: s.Packed, conformant: true,
		index: &subrangeType{host: host(ord), lo: boundValue(lo), hi: boundValue(hi)},
		elem:  elem,
	}
	var bounds []*obj
	if lo != nil {
		bounds = append(bounds, lo)
	}
	if hi != nil {
		bounds = append(bounds, hi)
	}
	return arr, &confSchema{packed: s.Packed, index: ord, elem: elem, nested: nested}, append(bounds, more...)
}

func (c *checker) bindBound(name string) *obj {
	o := &obj{name: name, kind: kindConst, bound: true, val: &value{known: false}}
	if !c.insert(o) {
		return nil
	}
	return o
}

func (c *checker) noteBound(o *obj, typ Type) {
	if o == nil {
		return
	}
	o.typ = typ
	o.val.typ = typ
}

func boundValue(o *obj) *value {
	if o == nil {
		return &value{known: false}
	}
	return o.val
}

func (c *checker) typeName(name string) Type {
	o := c.lookup(name)
	if o == nil {
		c.errorf(ErrNotDeclared, name)
		return invalid
	}
	if o.blocked {
		return invalid
	}
	if o.open {
		c.errorf(ErrIncomplete, name)
		return invalid
	}
	if o.kind != kindType {
		c.errorf(ErrNotType, name)
		return invalid
	}
	return o.typ
}

func (c *checker) denote(t ast.Type) Type {
	got := c.denoteType(t)
	if t != nil && c.typeOf != nil {
		c.typeOf[t] = got
	}
	return got
}

func (c *checker) denoteType(t ast.Type) Type {
	if t == nil {
		return invalid
	}
	defer c.locate(t)()
	switch t := t.(type) {
	case *ast.TypeName:
		got := c.typeName(t.Name)
		if t.Skip && !isInvalid(got) {
			c.errorf(ErrBadStmt)
		}
		return got
	case *ast.Enum:
		e := &enumType{names: append([]string{}, t.Names...)}
		for i, name := range t.Names {
			c.insert(&obj{
				name: name,
				kind: kindConst,
				typ:  e,
				val:  &value{typ: e, known: true, n: int64(i)},
			})
		}
		return e
	case *ast.Subrange:
		lo := c.constant(t.Low)
		hi := c.constant(t.High)
		if lo == nil || hi == nil || !ordinal(lo.typ) || !ordinal(hi.typ) || !sameType(host(lo.typ), host(hi.typ)) {
			c.errorf(ErrSubrange)
			return invalid
		}
		if lo.known && hi.known && lo.n > hi.n {
			c.errorf(ErrEmptySubrange)
		}
		return &subrangeType{host: host(lo.typ), lo: lo, hi: hi}
	case *ast.Array:
		return c.arrayType(t.Packed, t.Index, t.Elem)
	case *ast.Record:
		return c.recordType(t)
	case *ast.Set:
		base := c.denote(t.Base)
		if !isInvalid(base) && !ordinal(base) {
			c.errorf(ErrSetBase)
			return invalid
		}
		if !isInvalid(base) && ordinal(base) && !setBaseOK(base) {
			c.errorf(ErrSetSize)
			return invalid
		}
		return &setType{packed: t.Packed, base: base}
	case *ast.File:
		elem := c.denote(t.Elem)
		if containsFile(elem) {
			c.errorf(ErrFileOfFile)
			return invalid
		}
		return &fileType{packed: t.Packed, elem: elem}
	case *ast.Pointer:
		return c.pointerTo(t.To)
	default:
		c.errorf(ErrBadType)
		return invalid
	}
}

func (c *checker) arrayType(packed bool, index []ast.Type, elem ast.Type) Type {
	if len(index) == 0 {
		return invalid
	}
	// Index types are denoted from left to right, then the component.
	// A later index can name a constant introduced by an earlier index.
	idx := c.denote(index[0])
	var inner Type
	if len(index) == 1 {
		inner = c.denote(elem)
	} else {
		inner = c.arrayType(packed, index[1:], elem)
	}
	if !isInvalid(idx) && !ordinal(idx) {
		c.errorf(ErrIndexOrdinal)
		return invalid
	}
	if lo, hi, ok := ordinalSpan(idx); ok && lo <= hi {
		if _, fit := fitLen(lo, hi); !fit {
			c.errorf(ErrArraySize)
		}
	}
	return &arrayType{packed: packed, index: idx, elem: inner}
}

func (c *checker) recordType(r *ast.Record) Type {
	rec := &recordType{packed: r.Packed}
	c.records = append(c.records, &recFields{all: fieldNames(r), defined: map[string]bool{}})
	seen := map[string]bool{}
	if r.Fields != nil {
		rec.variant = c.fieldList(r.Fields, rec, seen)
	}
	c.records = c.records[:len(c.records)-1]
	return rec
}

// recordField reports a field of a record type currently being defined.
// The closest record wins. A use before that field's defining-point is an
// error and does not see a type of the same spelling.
func (c *checker) recordField(name string) (*obj, bool) {
	for i := len(c.records) - 1; i >= 0; i-- {
		r := c.records[i]
		if !r.all[name] {
			continue
		}
		if !r.defined[name] {
			c.errorf(ErrBeforeDef, name)
			return &obj{name: name, blocked: true, typ: invalid, val: &value{typ: invalid}}, true
		}
		return &obj{name: name, kind: kindField, typ: invalid}, true
	}
	return nil, false
}

func (c *checker) defineFields(names []string) {
	if len(c.records) == 0 {
		return
	}
	top := c.records[len(c.records)-1]
	for _, name := range names {
		top.defined[name] = true
	}
}

func fieldNames(r *ast.Record) map[string]bool {
	m := map[string]bool{}
	if r.Fields != nil {
		collectFieldNames(r.Fields, m)
	}
	return m
}

func collectFieldNames(list *ast.FieldList, m map[string]bool) {
	for _, f := range list.Fixed {
		for _, name := range f.Names {
			m[name] = true
		}
	}
	if list.Variant == nil {
		return
	}
	if list.Variant.Tag != "" {
		m[list.Variant.Tag] = true
	}
	for _, arm := range list.Variant.Arms {
		if arm.Fields != nil {
			collectFieldNames(arm.Fields, m)
		}
	}
}

func (c *checker) fieldList(list *ast.FieldList, rec *recordType, seen map[string]bool) *variantPart {
	for _, f := range list.Fixed {
		// The field identifiers precede their type.
		c.defineFields(f.Names)
		t := c.denote(f.Type)
		for _, name := range f.Names {
			if seen[name] {
				c.errorf(ErrDupIdent, name)
				continue
			}
			seen[name] = true
			rec.fields = append(rec.fields, field{name: name, typ: t})
		}
	}
	if list.Variant == nil {
		return nil
	}
	v := list.Variant
	if v.Tag != "" {
		c.defineFields([]string{v.Tag})
	}
	tag := c.typeName(v.Type)
	if !isInvalid(tag) && !ordinal(tag) {
		c.errorf(ErrTagOrdinal, v.Type)
	}
	if v.Tag != "" {
		if seen[v.Tag] {
			c.errorf(ErrDupIdent, v.Tag)
		} else {
			seen[v.Tag] = true
			rec.fields = append(rec.fields, field{name: v.Tag, typ: tag, tag: true})
		}
	}
	part := &variantPart{tag: tag}
	covered := map[int64]bool{}
	for _, arm := range v.Arms {
		a := variantArm{}
		for _, lab := range arm.Labels {
			val := c.constant(lab)
			if val == nil || !ordinal(val.typ) || !compatible(val.typ, tag) {
				c.errorf(ErrTagLabel)
				continue
			}
			if val.known {
				if covered[val.n] {
					c.errorf(ErrDupVariant)
				}
				covered[val.n] = true
				a.labels = append(a.labels, val.n)
			}
		}
		if arm.Fields != nil {
			a.next = c.fieldList(arm.Fields, rec, seen)
		}
		part.arms = append(part.arms, a)
	}
	if !isInvalid(tag) && !covers(tag, covered) {
		c.errorf(ErrVariantCover, v.Type)
	}
	return part
}

// covers reports whether got lists every value of tag once. A subrange is
// covered by its own bounds, not by every value of the host type.
func covers(tag Type, got map[int64]bool) bool {
	lo, hi, ok := ordinalSpan(tag)
	if !ok || hi < lo {
		return false
	}
	if int64(len(got)) != hi-lo+1 {
		return false
	}
	for n := lo; n <= hi; n++ {
		if !got[n] {
			return false
		}
	}
	return true
}

func (c *checker) pointerTo(name string) Type {
	if o, ok := c.recordField(name); ok {
		if !o.blocked {
			c.errorf(ErrNotType, name)
		}
		return invalid
	}
	if c.section[name] {
		if o := c.scope.objs[name]; o != nil && o.kind == kindType {
			if o.open {
				p := &pointerType{}
				o.ptrs = append(o.ptrs, p)
				return p
			}
			return &pointerType{base: o.typ}
		}
		p := &pointerType{}
		c.waiting[name] = append(c.waiting[name], p)
		return p
	}
	return &pointerType{base: c.typeName(name)}
}

func (c *checker) constant(e ast.Expr) *value {
	v := c.constantVal(e)
	if e != nil && v != nil && c.raw != nil {
		if _, ok := c.raw[e]; !ok {
			op := operand{typ: v.typ, val: v}
			if n, ok := e.(*ast.Name); ok && !isInvalid(v.typ) {
				op.obj = c.lookup(n.Ident)
			}
			c.raw[e] = op
		}
	}
	return v
}

func (c *checker) constantVal(e ast.Expr) *value {
	defer c.locate(e)()
	switch e := e.(type) {
	case *ast.Int:
		n, err := strconv.ParseInt(e.Text, 10, 64)
		if err != nil {
			c.errorf(ErrIntRange)
			return &value{typ: invalid}
		}
		return &value{typ: integerType, known: true, n: n}
	case *ast.Real:
		f, err := strconv.ParseFloat(e.Text, 64)
		if err != nil {
			c.errorf(ErrExponent)
			return &value{typ: invalid}
		}
		return &value{typ: realType, real: true, f: f, known: true}
	case *ast.Str:
		if e.Value == "" {
			c.errorf(ErrEmptyString)
			return &value{typ: invalid}
		}
		for _, r := range e.Value {
			if r > 255 {
				c.errorf(ErrCharRange)
				return &value{typ: invalid}
			}
		}
		if len([]rune(e.Value)) == 1 {
			r := []rune(e.Value)[0]
			return &value{typ: charType, known: true, n: int64(r), text: e.Value}
		}
		return &value{typ: stringOf(len([]rune(e.Value))), text: e.Value}
	case *ast.Paren:
		return c.constant(e.X)
	case *ast.Name:
		o := c.lookup(e.Ident)
		if o != nil && o.blocked {
			return &value{typ: invalid}
		}
		if o == nil || o.kind != kindConst || o.bound {
			c.errorf(ErrNotConst, e.Ident)
			return &value{typ: invalid}
		}
		return o.val
	case *ast.Unary:
		v := c.constant(e.X)
		if v == nil || isInvalid(v.typ) {
			return &value{typ: invalid}
		}
		if e.Op == "not" {
			c.errorf(ErrNotConstOp)
			return &value{typ: invalid}
		}
		if host(v.typ) != integerType && host(v.typ) != realType {
			c.errorf(ErrSign, e.X)
			return &value{typ: invalid}
		}
		out := *v
		if v.real {
			out.f = -v.f
			if e.Op == "+" {
				out.f = v.f
			}
		} else if v.known {
			if e.Op == "-" {
				out.n = -v.n
			}
		}
		return &out
	default:
		c.errorf(ErrNotConstExpr)
		return &value{typ: invalid}
	}
}

func stringOf(n int) Type {
	if n <= 1 {
		return charType
	}
	hi := &value{typ: integerType, known: true, n: int64(n)}
	lo := &value{typ: integerType, known: true, n: 1}
	return &arrayType{
		packed: true,
		index:  &subrangeType{host: integerType, lo: lo, hi: hi},
		elem:   charType,
	}
}
