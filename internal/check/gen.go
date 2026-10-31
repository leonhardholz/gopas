package check

import (
	"fmt"
	"go/format"
	"sort"
	"strconv"
	"strings"

	"gopas/internal/ast"
	"gopas/runtime"
)

// Options selects the checks the generated program performs.
// The defaults match Turbo Pascal: short-circuit boolean operators,
// wrap on overflow, and no range checks. Division by zero always panics.
// Range checks cover subrange stores, set members, for-statement bounds,
// chr, succ, pred, case labels, and pack and unpack.
// An index outside a Go array still panics, because a Go array cannot
// turn that check off without leaving the array's bounds.
type Options struct {
	CompleteBool bool
	Overflow     bool
	Range        bool
	// DisposeExpr allows dispose of a pointer expression. The standard
	// requires a pointer variable, so this is off unless a program such as
	// the ISO acceptance test needs it.
	DisposeExpr bool
}

// Generate translates p into a Go program in package main.
// The runtime is copied into that file. When p violates the language
// rules, Generate returns those errors and no source.
func Generate(p *ast.Program, opt *Options) (string, []Error) {
	if p == nil {
		return "", []Error{{Msg: "no program"}}
	}
	c := newChecker()
	if opt != nil {
		c.opt = *opt
	}
	c.run(p)
	if len(c.errs) > 0 {
		return "", c.errs
	}
	g := &gen{c: c, strs: map[int]bool{}, withTmp: map[ast.Expr]string{}}
	if opt != nil {
		g.opt = *opt
	}
	g.program(p)
	if len(g.problems) > 0 {
		return g.file(), g.problems
	}
	src := g.file()
	if g.part != 0 {
		var err error
		src, err = runtime.Bundle(src, g.part)
		if err != nil {
			return src, []Error{{Msg: err.Error()}}
		}
	}
	pretty, err := format.Source([]byte(src))
	if err != nil {
		return src, []Error{{Msg: "go/format: " + err.Error()}}
	}
	return string(pretty), nil
}

type gen struct {
	c        *checker
	opt      Options
	b        strings.Builder
	ind      int
	n        int
	tn       int
	part     runtime.Part
	needMax  bool
	strs     map[int]bool
	names    map[Type]string
	queue    []Type
	aliases  []string
	withTmp  map[ast.Expr]string
	withLive []ast.Expr
	emitted  map[*obj]bool
	taken    map[string]bool
	enumName map[enumKey]string
	loop     string
	pc       string
	fn       *obj
	depth    int
	frames   []*labFrame
	ready    []*labFrame
	frameAt  int
	active   []*labFrame
	resName  map[*obj]string
	problems []Error
	// copyOf reports whether a conformant value parameter is copied.
	// Missing entries are copied too.
	copyOf map[*parm]bool
	// closure is a routine that must be a function variable, because it
	// uses an enclosing activation. Every other routine is a Go function.
	closure map[*obj]bool
	fnGo    map[*obj]string
	prog    *scope
	level   map[string]string
	top     bool
	// uses is the parameters and locals of the routines being generated.
	// A read of one is recorded on the object, so a nested parameter is
	// not a use of an outer variable with the same spelling.
	uses []*useFrame
	used map[*obj]bool
}

// useFrame is one routine's parameters and locals that Go would reject
// if they were never read. The Go name is the one to discard.
type useFrame struct {
	objs []*obj
	name map[*obj]string
}

// labFrame is one statement list that contains a label or a goto.
// pc is declared in the enclosing function before any nested routine,
// so a goto in a nested routine can store the target label there.
type labFrame struct {
	pc     string
	loop   string
	first  int
	depth  int
	labels map[int]bool
}

func (g *gen) errf(format string, args ...any) {
	g.problems = append(g.problems, Error{Msg: fmt.Sprintf(format, args...)})
}

func (g *gen) line(format string, args ...any) {
	for i := 0; i < g.ind; i++ {
		g.b.WriteByte('\t')
	}
	if len(args) == 0 {
		g.b.WriteString(format)
	} else {
		fmt.Fprintf(&g.b, format, args...)
	}
	g.b.WriteByte('\n')
}

func (g *gen) use(p runtime.Part) { g.part |= p }

func (g *gen) useFile(t Type) {
	if _, ok := t.(*fileType); ok {
		g.use(runtime.PartFile)
		return
	}
	g.use(runtime.PartText)
}

func (g *gen) fresh() string {
	s := fmt.Sprintf("_t%d", g.n)
	g.n++
	return s
}

func (g *gen) file() string {
	types := g.declTypes()
	var b strings.Builder
	b.WriteString("package main\n\n")
	if g.needMax {
		b.WriteString("const maxint = int(^uint(0) >> 1)\n\n")
	}
	if len(g.strs) > 0 {
		ns := make([]int, 0, len(g.strs))
		for n := range g.strs {
			ns = append(ns, n)
		}
		sort.Ints(ns)
		b.WriteString("type (\n")
		for _, n := range ns {
			fmt.Fprintf(&b, "\tstr%d [%d]uint8\n", n, n)
		}
		b.WriteString(")\n\n")
	}
	b.WriteString(types)
	b.WriteString(g.b.String())
	return b.String()
}

func (g *gen) program(p *ast.Program) {
	g.names = map[Type]string{}
	g.emitted = map[*obj]bool{}
	g.classify(p)
	g.reserveRuntime()
	if g.c.usedInput {
		g.use(runtime.PartText)
		g.line("var %s rtText", g.hoist("input"))
		g.line("var rtProgramInput *rtText")
	}
	if g.c.usedOutput {
		g.use(runtime.PartText)
		g.line("var %s rtText", g.hoist("output"))
		g.line("var rtProgramOutput *rtText")
	}
	if g.c.farGoto {
		g.line("var _unwind *int")
	}
	g.top = true
	g.emitConsts(p.Consts)
	g.bindTypes(p.Types)
	for _, d := range p.Vars {
		g.goType(g.typ(d.Type))
	}
	g.emitVars(p.Vars)
	g.top = false
	g.nameLifted(p.Routines)
	g.emitLifted(p.Routines)
	g.line("func main() {")
	g.ind++
	if g.c.usedInput {
		g.line("%s = rtInput()", g.hoist("input"))
		g.line("rtProgramInput = &%s", g.hoist("input"))
	}
	if g.c.usedOutput {
		g.line("%s = rtOutput()", g.hoist("output"))
		g.line("rtProgramOutput = &%s", g.hoist("output"))
		g.line("defer %s.Flush()", g.hoist("output"))
	}
	g.bindFiles(p.Files)
	g.finish(blk{labels: p.Labels, routines: p.Routines, body: p.Body})
	g.ind--
	g.line("}")
}

type blk struct {
	labels   []string
	consts   []*ast.ConstDef
	types    []*ast.TypeDef
	vars     []*ast.Decl
	routines []ast.Routine
	body     []ast.Stmt
	result   Type
	files    []string
}

func (g *gen) block(b blk) {
	g.emitConsts(b.consts)
	g.bindTypes(b.types)
	for _, d := range b.vars {
		g.goType(g.typ(d.Type))
	}
	g.emitVars(b.vars)
	g.bindFiles(b.files)
	g.finish(b)
}

func (g *gen) finish(b blk) {
	frames := g.collectFrames(b.body)
	for _, f := range frames {
		g.line("var %s int = %d", f.pc, f.first)
	}
	g.frames = append(g.frames, frames...)
	savedReady, savedAt := g.ready, g.frameAt
	g.ready, g.frameAt = frames, 0
	g.emitRoutines(b.routines)
	g.statements(b.body, b.labels)
	g.discardUses()
	g.frames = g.frames[:len(g.frames)-len(frames)]
	g.ready, g.frameAt = savedReady, savedAt
	if b.result != nil {
		g.line("return %s", g.resultVar(g.fn))
	}
}

// pushUses starts a routine's list of names Go requires to be read.
func (g *gen) pushUses() {
	g.uses = append(g.uses, &useFrame{name: map[*obj]string{}})
}

// track remembers a parameter or local. name is its Go spelling.
func (g *gen) track(o *obj, name string) {
	if o == nil || name == "" || name == "_" || len(g.uses) == 0 {
		return
	}
	f := g.uses[len(g.uses)-1]
	if _, ok := f.name[o]; ok {
		return
	}
	f.name[o] = name
	f.objs = append(f.objs, o)
}

// noteUse records a use of o. A plain assignment of the whole variable does not.
func (g *gen) noteUse(o *obj) {
	if o == nil {
		return
	}
	if g.used == nil {
		g.used = map[*obj]bool{}
	}
	g.used[o] = true
}

// discardUses emits _ = name for every tracked name that was never read.
// The reference does not take the address, so the value can stay in a
// register and the routine can be inlined.
func (g *gen) discardUses() {
	if len(g.uses) == 0 {
		return
	}
	f := g.uses[len(g.uses)-1]
	g.uses = g.uses[:len(g.uses)-1]
	for _, o := range f.objs {
		if g.used[o] {
			continue
		}
		g.line("_ = %s", f.name[o])
	}
}

func (g *gen) emitConsts(defs []*ast.ConstDef) {
	for _, d := range defs {
		name := g.declName(d.Name)
		v := g.value(d.Value)
		// A string constant is an array, which Go cannot put in a const.
		if _, ok := stringLen(g.op(d.Value).typ); ok {
			g.line("var %s = %s", name, v)
			continue
		}
		g.line("const %s = %s", name, v)
	}
}

func (g *gen) emitVars(defs []*ast.Decl) {
	for _, d := range defs {
		t := g.goType(g.typ(d.Type))
		for _, name := range d.Names {
			n := g.declName(name)
			g.line("var %s %s", n, t)
			if !g.top && g.fn != nil && g.fn.inner != nil {
				g.track(g.fn.inner.objs[name], n)
			}
		}
	}
}

func (g *gen) declName(name string) string {
	if g.top {
		return g.hoist(name)
	}
	return g.goName(name)
}

// hoist is the Go name of a program-level declaration. The name is chosen
// once, and it avoids package-level names that belong to the runtime.
func (g *gen) hoist(name string) string {
	if g.level == nil {
		g.level = map[string]string{}
	}
	if n, ok := g.level[name]; ok {
		return n
	}
	n := g.unique(g.goName(name))
	g.level[name] = n
	return n
}

func (g *gen) reserveRuntime() {
	for _, n := range runtimeNames {
		g.reserve(n)
	}
}

func (g *gen) bindFiles(files []string) {
	for _, name := range files {
		if name == "input" || name == "output" {
			continue
		}
		g.use(runtime.PartText)
		id := g.goName(name)
		if n, ok := g.level[name]; ok {
			id = n
		}
		g.line("%s.Bind(%s)", id, strconv.Quote(name))
	}
}

// emitRoutines writes the routines that close over this activation.
// Routines that do not are package-level functions, emitted by emitLifted.
func (g *gen) emitRoutines(rs []ast.Routine) {
	var closed []ast.Routine
	for _, r := range rs {
		if g.closed(g.c.sigOf[r]) {
			closed = append(closed, r)
		}
	}
	for _, r := range closed {
		g.funcVar(r)
	}
	for _, r := range closed {
		g.funcBody(r)
	}
}

func (g *gen) funcVar(r ast.Routine) {
	o := g.c.sigOf[r]
	if o == nil || g.emitted[o] {
		return
	}
	g.emitted[o] = true
	g.noteConfCopy(r)
	parts := g.sigParts(o.params)
	res := ""
	if o.kind == kindFunc {
		res = " " + g.resultGo(o.typ, o.params)
	}
	g.line("var %s func(%s)%s", g.fnName(o), strings.Join(parts, ", "), res)
}

func (g *gen) funcBody(r ast.Routine) {
	if !g.c.bodyOf[r] {
		return
	}
	o := g.c.sigOf[r]
	if o == nil {
		return
	}
	g.noteConfCopy(r)
	parts := g.sigParts(o.params)
	res := ""
	var result Type
	b := blk{}
	switch r := r.(type) {
	case *ast.Proc:
		b = blk{labels: r.Labels, consts: r.Consts, types: r.Types, vars: r.Vars, routines: r.Routines, body: r.Body}
	case *ast.Func:
		result = o.typ
		res = " " + g.resultGo(result, o.params)
		b = blk{labels: r.Labels, consts: r.Consts, types: r.Types, vars: r.Vars, routines: r.Routines, body: r.Body, result: result}
	}
	prevFn, prevDepth := g.fn, g.depth
	prevActive := g.active
	g.fn = o
	g.depth++
	g.active = nil
	if g.closed(o) {
		g.line("%s = func(%s)%s {", g.fnName(o), strings.Join(parts, ", "), res)
	} else {
		g.line("func %s(%s)%s {", g.fnName(o), strings.Join(parts, ", "), res)
	}
	g.ind++
	if result != nil {
		g.line("var %s %s", g.resultVar(o), g.resultGo(result, o.params))
	}
	g.pushUses()
	g.trackParams(o)
	g.prologue(o.params)
	g.block(b)
	g.ind--
	g.line("}")
	g.fn, g.depth = prevFn, prevDepth
	g.active = prevActive
	if g.closed(o) {
		// Assignment alone does not count as a use.
		g.line("_ = %s", g.fnName(o))
	}
}

func (g *gen) closed(o *obj) bool {
	return o != nil && g.closure[o]
}

func (g *gen) fnName(o *obj) string {
	if o == nil {
		return "missing"
	}
	if o.param {
		g.noteUse(o)
	}
	if n, ok := g.fnGo[o]; ok {
		return n
	}
	return g.goName(o.name)
}

// trackParams records the routine's parameters and bound identifiers.
func (g *gen) trackParams(o *obj) {
	if o == nil {
		return
	}
	for _, p := range o.params {
		name := g.goName(p.name)
		if g.copies(p) {
			name = "_" + name
		}
		var ob *obj
		if o.inner != nil {
			ob = o.inner.objs[p.name]
		}
		g.track(ob, name)
		for _, b := range p.bounds {
			g.track(b, g.goName(b.name))
		}
	}
}

func (g *gen) reserve(name string) {
	if g.taken == nil {
		g.taken = map[string]bool{}
	}
	g.taken[g.goName(name)] = true
}

// runtimeNames are package-level names the bundled runtime keeps.
// A lifted routine must not reuse them, or func main.
var runtimeNames = []string{
	"main", "init", "store", "remember", "openStore", "fill", "pad",
	"fitScientific", "signedSci", "structField", "writeVal", "readVal",
}

func (g *gen) nameLifted(rs []ast.Routine) {
	g.nameLiftedList(rs)
}

func (g *gen) nameLiftedList(rs []ast.Routine) {
	for _, r := range rs {
		o := g.c.sigOf[r]
		if o != nil && g.c.bodyOf[r] && !g.closed(o) {
			if g.fnGo == nil {
				g.fnGo = map[*obj]string{}
			}
			if _, ok := g.fnGo[o]; !ok {
				g.fnGo[o] = g.unique(g.goName(o.name))
			}
		}
		g.nameLiftedList(nestedRoutines(r))
	}
}

func (g *gen) emitLifted(rs []ast.Routine) {
	for _, r := range rs {
		o := g.c.sigOf[r]
		if o != nil && g.c.bodyOf[r] && !g.closed(o) {
			g.funcBody(r)
		}
		g.emitLifted(nestedRoutines(r))
	}
}

type routInfo struct {
	obj    *obj
	parent *routInfo
	labels []string
	refs   []*obj
}

func nestedRoutines(r ast.Routine) []ast.Routine {
	switch r := r.(type) {
	case *ast.Proc:
		return r.Routines
	case *ast.Func:
		return r.Routines
	default:
		return nil
	}
}

func routineLabels(r ast.Routine) []string {
	switch r := r.(type) {
	case *ast.Proc:
		return r.Labels
	case *ast.Func:
		return r.Labels
	default:
		return nil
	}
}

// classify marks routines that use an enclosing activation.
// A routine that only uses program variables can be a Go function,
// which the compiler can inline. A nested routine that reads its
// parent's variables stays a function variable.
func (g *gen) classify(p *ast.Program) {
	g.closure = map[*obj]bool{}
	for _, r := range p.Routines {
		if o := g.c.sigOf[r]; o != nil && o.block != nil {
			g.prog = o.block
			break
		}
	}
	if g.prog == nil {
		for _, op := range g.c.raw {
			o := op.obj
			if o != nil && o.block != nil && o.block.parent != nil && o.block.parent.parent == nil {
				g.prog = o.block
				break
			}
		}
	}
	var infos []*routInfo
	var walk func(ast.Routine, *routInfo)
	walk = func(r ast.Routine, parent *routInfo) {
		o := g.c.sigOf[r]
		if o == nil || !g.c.bodyOf[r] {
			return
		}
		info := &routInfo{obj: o, parent: parent, labels: routineLabels(r)}
		g.scanRoutine(o, r, info)
		infos = append(infos, info)
		for _, n := range nestedRoutines(r) {
			walk(n, info)
		}
	}
	for _, r := range p.Routines {
		walk(r, nil)
	}
	for {
		changed := false
		for _, info := range infos {
			for _, callee := range info.refs {
				if callee == info.obj || !g.closure[callee] || ownedBy(info.obj, callee) {
					continue
				}
				if g.closeUntilContainer(info, callee) {
					changed = true
				}
			}
		}
		if !changed {
			return
		}
	}
}

func (g *gen) scanRoutine(o *obj, r ast.Routine, info *routInfo) {
	var consts []*ast.ConstDef
	var types []*ast.TypeDef
	var vars []*ast.Decl
	var body []ast.Stmt
	switch r := r.(type) {
	case *ast.Proc:
		consts, types, vars, body = r.Consts, r.Types, r.Vars, r.Body
	case *ast.Func:
		consts, types, vars, body = r.Consts, r.Types, r.Vars, r.Body
	default:
		return
	}
	for _, d := range consts {
		g.scanExpr(o, d.Value, info)
	}
	for _, d := range types {
		g.scanType(o, d.Type, info)
	}
	for _, d := range vars {
		g.scanType(o, d.Type, info)
	}
	g.scanStmts(o, body, routineLabels(r), info)
}

func (g *gen) scanStmts(o *obj, body []ast.Stmt, labels []string, info *routInfo) {
	for _, s := range body {
		g.scanStmt(o, s, labels, info)
	}
}

func (g *gen) scanStmt(o *obj, s ast.Stmt, labels []string, info *routInfo) {
	if s == nil {
		return
	}
	switch s := s.(type) {
	case *ast.Assign:
		g.scanExpr(o, s.Left, info)
		g.scanExpr(o, s.Value, info)
	case *ast.Read:
		for _, v := range s.Vars {
			g.scanExpr(o, v, info)
		}
	case *ast.Call:
		g.noteRoutine(g.c.callOf[s], info)
		for _, a := range s.Args {
			g.scanExpr(o, a, info)
		}
	case *ast.Write:
		for _, a := range s.Args {
			g.scanExpr(o, a.Value, info)
			g.scanExpr(o, a.Width, info)
			g.scanExpr(o, a.Frac, info)
		}
	case *ast.Compound:
		g.scanStmts(o, s.Body, labels, info)
	case *ast.If:
		g.scanExpr(o, s.Cond, info)
		g.scanStmt(o, s.Then, labels, info)
		g.scanStmt(o, s.Else, labels, info)
	case *ast.While:
		g.scanExpr(o, s.Cond, info)
		g.scanStmt(o, s.Body, labels, info)
	case *ast.Repeat:
		g.scanStmts(o, s.Body, labels, info)
		g.scanExpr(o, s.Cond, info)
	case *ast.For:
		g.scanExpr(o, s.From, info)
		g.scanExpr(o, s.To, info)
		g.scanStmt(o, s.Body, labels, info)
	case *ast.Case:
		g.scanExpr(o, s.Expr, info)
		for _, arm := range s.Arms {
			for _, lab := range arm.Labels {
				g.scanExpr(o, lab, info)
			}
			g.scanStmt(o, arm.Stmt, labels, info)
		}
	case *ast.With:
		for _, rec := range s.Recs {
			g.scanExpr(o, rec, info)
		}
		g.scanStmt(o, s.Body, labels, info)
	case *ast.Labeled:
		g.scanStmt(o, s.Stmt, labels, info)
	case *ast.Goto:
		if !listed(s.Label, labels) {
			g.closeForGoto(info, s.Label)
		}
	}
}

// closeForGoto keeps every routine out to the label's block as a closure,
// so the generated function can store the target frame. The block that
// declares the label stays a normal function.
func (g *gen) closeForGoto(info *routInfo, label string) {
	for cur := info; cur != nil; cur = cur.parent {
		g.closure[cur.obj] = true
		if cur.parent != nil && listed(label, cur.parent.labels) {
			return
		}
	}
}

func listed(name string, labels []string) bool {
	for _, lab := range labels {
		if lab == name {
			return true
		}
	}
	return false
}

func (g *gen) scanExpr(o *obj, e ast.Expr, info *routInfo) {
	if e == nil {
		return
	}
	switch e := e.(type) {
	case *ast.Name:
		g.noteName(o, e, info)
	case *ast.CallExpr:
		g.noteRoutine(g.c.callOf[e], info)
		for _, a := range e.Args {
			g.scanExpr(o, a, info)
		}
	case *ast.Paren:
		g.scanExpr(o, e.X, info)
	case *ast.Unary:
		g.scanExpr(o, e.X, info)
	case *ast.Binary:
		g.scanExpr(o, e.Left, info)
		g.scanExpr(o, e.Right, info)
	case *ast.Indexed:
		g.scanExpr(o, e.Base, info)
		for _, ix := range e.Indexes {
			g.scanExpr(o, ix, info)
		}
	case *ast.Select:
		g.scanExpr(o, e.Base, info)
	case *ast.Deref:
		g.scanExpr(o, e.Base, info)
	case *ast.SetLit:
		for _, el := range e.Elems {
			g.scanExpr(o, el.Low, info)
			g.scanExpr(o, el.High, info)
		}
	}
}

func (g *gen) scanType(o *obj, t ast.Type, info *routInfo) {
	switch t := t.(type) {
	case *ast.Subrange:
		g.scanExpr(o, t.Low, info)
		g.scanExpr(o, t.High, info)
	case *ast.Array:
		for _, ix := range t.Index {
			g.scanType(o, ix, info)
		}
		g.scanType(o, t.Elem, info)
	case *ast.Set:
		g.scanType(o, t.Base, info)
	case *ast.File:
		g.scanType(o, t.Elem, info)
	case *ast.Record:
		g.scanFields(o, t.Fields, info)
	}
}

func (g *gen) scanFields(o *obj, f *ast.FieldList, info *routInfo) {
	if f == nil {
		return
	}
	for _, field := range f.Fixed {
		g.scanType(o, field.Type, info)
	}
	if f.Variant == nil {
		return
	}
	for _, arm := range f.Variant.Arms {
		for _, lab := range arm.Labels {
			g.scanExpr(o, lab, info)
		}
		g.scanFields(o, arm.Fields, info)
	}
}

func (g *gen) noteName(routine *obj, e *ast.Name, info *routInfo) {
	op := g.op(e)
	o := op.obj
	if o == nil {
		return
	}
	if op.fn && o != routine {
		g.closeUntil(info, o)
		return
	}
	switch o.kind {
	case kindVar:
		if g.captures(routine, o) {
			g.closeForCapture(info, o)
		}
	case kindConst:
		if enumConst(o) {
			return
		}
		if g.captures(routine, o) {
			g.closeForCapture(info, o)
		}
	case kindProc, kindFunc:
		g.noteRoutine(o, info)
	}
}

func enumConst(o *obj) bool {
	e, ok := host(o.typ).(*enumType)
	return ok && enumMember(e, o.name)
}

// closeUntil marks routine and each parent up to, but not including, owner.
// The captured variable then lives in the function that contains the closure.
func (g *gen) closeUntil(info *routInfo, owner *obj) {
	for cur := info; cur != nil && cur.obj != owner; cur = cur.parent {
		g.closure[cur.obj] = true
	}
}

// closeUntilContainer marks routine and each parent until the function that
// declares callee. A call of that closure can then see it.
func (g *gen) closeUntilContainer(info *routInfo, callee *obj) bool {
	changed := false
	for cur := info; cur != nil; cur = cur.parent {
		if !g.closure[cur.obj] {
			g.closure[cur.obj] = true
			changed = true
		}
		if callee.block == cur.obj.block {
			return changed
		}
	}
	return changed
}

// closeForCapture marks every routine out to the one that declares v.
func (g *gen) closeForCapture(info *routInfo, v *obj) {
	for cur := info; cur != nil; cur = cur.parent {
		g.closure[cur.obj] = true
		if cur.obj.block == v.block {
			return
		}
	}
}

func (g *gen) noteRoutine(o *obj, info *routInfo) {
	if o == nil || o.param || o.predef != "" {
		return
	}
	if o.kind != kindProc && o.kind != kindFunc {
		return
	}
	info.refs = append(info.refs, o)
}

// captures reports that v belongs to an activation outside routine,
// other than the program block. Program variables are package-level.
func (g *gen) captures(routine, v *obj) bool {
	if routine == nil || v == nil || routine.block == nil || v.block == nil || v.withBase != nil {
		return false
	}
	for s := routine.block; s != nil && s != g.prog; s = s.parent {
		if s == v.block {
			return true
		}
	}
	return false
}

func (g *gen) sigParts(ps []*parm) []string {
	var parts []string
	for i := 0; i < len(ps); {
		p := ps[i]
		if p.schema == nil {
			name := g.goName(p.name)
			switch p.kind {
			case parmProc:
				parts = append(parts, name+" "+g.funcType(p.params, nil))
			case parmFunc:
				parts = append(parts, name+" "+g.funcType(p.params, p.result))
			case parmVar:
				parts = append(parts, name+" *"+g.goType(p.typ))
			default:
				parts = append(parts, name+" "+g.goType(p.typ))
			}
			i++
			continue
		}
		sec := p.section
		var grp []*parm
		for i < len(ps) && ps[i].schema != nil && ps[i].section == sec {
			grp = append(grp, ps[i])
			i++
		}
		elem := g.goType(scalarOf(grp[0].typ))
		for _, q := range grp {
			name := g.goName(q.name)
			if g.copies(q) {
				name = "_" + name
			}
			parts = append(parts, name+" *"+elem)
		}
		for _, b := range grp[0].bounds {
			parts = append(parts, g.goName(b.name)+" "+g.goType(b.typ))
		}
	}
	return parts
}

func (g *gen) funcType(ps []*parm, result Type) string {
	parts := g.sigParts(ps)
	if result == nil {
		return "func(" + strings.Join(parts, ", ") + ")"
	}
	return "func(" + strings.Join(parts, ", ") + ") " + g.resultGo(result, ps)
}

// resultGo is the Go type of a function result. A parameter can hide the
// result type's name, as in function f(t: integer): t, so a clash uses the
// type's representation instead of that name.
func (g *gen) resultGo(result Type, ps []*parm) string {
	name := g.goType(result)
	for _, p := range ps {
		if g.goName(p.name) != name {
			continue
		}
		if _, ok := result.(*subrangeType); ok {
			return g.basicGo(result)
		}
		return g.underlying(result)
	}
	return name
}

func (g *gen) prologue(ps []*parm) {
	for _, p := range ps {
		if !g.copies(p) {
			continue
		}
		g.use(runtime.PartMem)
		elem := g.goType(scalarOf(p.typ))
		n := g.sizeExpr(p.bounds)
		buf := "_buf" + g.goName(p.name)
		nn := "_n" + g.goName(p.name)
		src := "_" + g.goName(p.name)
		dst := g.goName(p.name)
		if g.fn != nil && g.fn.inner != nil {
			g.noteUse(g.fn.inner.objs[p.name])
		}
		g.line("%s := int(%s)", nn, n)
		g.line("%s := make([]%s, %s)", buf, elem, nn)
		g.line("if %s > 0 { rtMove(%s, &%s[0], %s) }", nn, src, buf, nn)
		g.line("var %s *%s", dst, elem)
		g.line("if %s > 0 { %s = &%s[0] }", nn, dst, buf)
		g.line("_ = %s", dst)
		g.line("_ = %s", buf)
	}
}

// noteConfCopy decides which conformant value parameters of r must be copied.
// A parameter is copied unless every use that can be seen only reads it.
func (g *gen) noteConfCopy(r ast.Routine) {
	o := g.c.sigOf[r]
	if o == nil || !g.c.bodyOf[r] {
		return
	}
	var routines []ast.Routine
	var body []ast.Stmt
	switch r := r.(type) {
	case *ast.Proc:
		routines, body = r.Routines, r.Body
	case *ast.Func:
		routines, body = r.Routines, r.Body
	default:
		return
	}
	if g.copyOf == nil {
		g.copyOf = map[*parm]bool{}
	}
	for _, p := range o.params {
		if p.schema == nil || p.kind != parmValue {
			continue
		}
		g.copyOf[p] = g.confWritten(p, routines, body)
	}
}

// copies reports that p is a conformant value parameter whose body may
// change it. Anything not proved read-only is copied.
func (g *gen) copies(p *parm) bool {
	if p == nil || p.schema == nil || p.kind != parmValue {
		return false
	}
	if g.copyOf == nil {
		return true
	}
	written, ok := g.copyOf[p]
	if !ok {
		return true
	}
	return written
}

func (g *gen) confWritten(p *parm, routines []ast.Routine, body []ast.Stmt) bool {
	if g.stmtsWrite(p, body) {
		return true
	}
	for _, r := range routines {
		if g.routineWrites(p, r) {
			return true
		}
	}
	return false
}

func (g *gen) routineWrites(p *parm, r ast.Routine) bool {
	switch r := r.(type) {
	case *ast.Proc:
		return g.confWritten(p, r.Routines, r.Body)
	case *ast.Func:
		return g.confWritten(p, r.Routines, r.Body)
	default:
		return true
	}
}

func (g *gen) stmtsWrite(p *parm, body []ast.Stmt) bool {
	for _, s := range body {
		if g.stmtWrites(p, s) {
			return true
		}
	}
	return false
}

func (g *gen) stmtWrites(p *parm, s ast.Stmt) bool {
	switch s := s.(type) {
	case nil:
		return false
	case *ast.Assign:
		return g.mentions(p, s.Left) || g.exprWrites(p, s.Left) || g.exprWrites(p, s.Value)
	case *ast.Read:
		for _, v := range s.Vars {
			if g.mentions(p, v) || g.exprWrites(p, v) {
				return true
			}
		}
		return false
	case *ast.Call:
		return g.callWrites(p, s, s.Args)
	case *ast.Write:
		for _, a := range s.Args {
			if g.exprWrites(p, a.Value) || g.exprWrites(p, a.Width) || g.exprWrites(p, a.Frac) {
				return true
			}
		}
		return false
	case *ast.Compound:
		return g.stmtsWrite(p, s.Body)
	case *ast.If:
		return g.exprWrites(p, s.Cond) || g.stmtWrites(p, s.Then) || g.stmtWrites(p, s.Else)
	case *ast.While:
		return g.exprWrites(p, s.Cond) || g.stmtWrites(p, s.Body)
	case *ast.Repeat:
		return g.stmtsWrite(p, s.Body) || g.exprWrites(p, s.Cond)
	case *ast.For:
		return g.exprWrites(p, s.From) || g.exprWrites(p, s.To) || g.stmtWrites(p, s.Body)
	case *ast.Case:
		if g.exprWrites(p, s.Expr) {
			return true
		}
		for _, arm := range s.Arms {
			for _, lab := range arm.Labels {
				if g.exprWrites(p, lab) {
					return true
				}
			}
			if g.stmtWrites(p, arm.Stmt) {
				return true
			}
		}
		return false
	case *ast.With:
		for _, rec := range s.Recs {
			if g.exprWrites(p, rec) {
				return true
			}
		}
		return g.stmtWrites(p, s.Body)
	case *ast.Labeled:
		return g.stmtWrites(p, s.Stmt)
	default:
		return false
	}
}

func (g *gen) exprWrites(p *parm, e ast.Expr) bool {
	if e == nil {
		return false
	}
	switch e := e.(type) {
	case *ast.CallExpr:
		return g.callWrites(p, e, e.Args)
	case *ast.Paren:
		return g.exprWrites(p, e.X)
	case *ast.Unary:
		return g.exprWrites(p, e.X)
	case *ast.Binary:
		return g.exprWrites(p, e.Left) || g.exprWrites(p, e.Right)
	case *ast.Indexed:
		if g.exprWrites(p, e.Base) {
			return true
		}
		for _, ix := range e.Indexes {
			if g.exprWrites(p, ix) {
				return true
			}
		}
		return false
	case *ast.Select:
		return g.exprWrites(p, e.Base)
	case *ast.Deref:
		return g.exprWrites(p, e.Base)
	case *ast.SetLit:
		for _, el := range e.Elems {
			if g.exprWrites(p, el.Low) || g.exprWrites(p, el.High) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (g *gen) callWrites(p *parm, node any, args []ast.Expr) bool {
	o := g.c.callOf[node]
	if o == nil || o.predef != "" {
		if g.predefWrites(p, o, args) {
			return true
		}
		for _, a := range args {
			if g.exprWrites(p, a) {
				return true
			}
		}
		return false
	}
	ai := 0
	for i := 0; i < len(o.params) && ai < len(args); {
		prm := o.params[i]
		if prm.schema == nil {
			if prm.kind == parmVar && g.mentions(p, args[ai]) || g.exprWrites(p, args[ai]) {
				return true
			}
			ai++
			i++
			continue
		}
		sec := prm.section
		for i < len(o.params) && o.params[i].schema != nil && o.params[i].section == sec {
			q := o.params[i]
			if ai < len(args) {
				if q.kind == parmVar && g.mentions(p, args[ai]) || g.exprWrites(p, args[ai]) {
					return true
				}
				ai++
			}
			i++
		}
	}
	if o != nil && o.predef == "" && g.writesOuter(o, map[*obj]bool{}) {
		return true
	}
	return false
}

// writesOuter reports that o assigns a variable that is not its own.
// A value conformant array is then copied, because that variable may be
// the actual parameter.
func (g *gen) writesOuter(o *obj, seen map[*obj]bool) bool {
	if o == nil || o.at == nil || seen[o] {
		return false
	}
	seen[o] = true
	var body []ast.Stmt
	var nested []ast.Routine
	switch r := o.at.(type) {
	case *ast.Proc:
		body, nested = r.Body, r.Routines
	case *ast.Func:
		body, nested = r.Body, r.Routines
	default:
		return true
	}
	if g.outerStmts(o, body, seen) {
		return true
	}
	for _, r := range nested {
		if g.writesOuter(g.c.sigOf[r], seen) {
			return true
		}
	}
	return false
}

func (g *gen) outerStmts(o *obj, body []ast.Stmt, seen map[*obj]bool) bool {
	for _, s := range body {
		if g.outerStmt(o, s, seen) {
			return true
		}
	}
	return false
}

func (g *gen) outerStmt(o *obj, s ast.Stmt, seen map[*obj]bool) bool {
	switch s := s.(type) {
	case nil:
		return false
	case *ast.Assign:
		return g.outerTarget(o, s.Left) || g.outerCalls(o, s.Left, seen) || g.outerCalls(o, s.Value, seen)
	case *ast.Read:
		for _, v := range s.Vars {
			if g.outerTarget(o, v) || g.outerCalls(o, v, seen) {
				return true
			}
		}
		return false
	case *ast.Call:
		return g.outerCall(o, s, s.Args, seen)
	case *ast.Write:
		if o2 := g.c.callOf[s]; o2 != nil && o2.predef == "" && g.writesOuter(o2, seen) {
			return true
		}
		for _, a := range s.Args {
			if g.outerCalls(o, a.Value, seen) || g.outerCalls(o, a.Width, seen) || g.outerCalls(o, a.Frac, seen) {
				return true
			}
		}
		return false
	case *ast.Compound:
		return g.outerStmts(o, s.Body, seen)
	case *ast.If:
		return g.outerCalls(o, s.Cond, seen) || g.outerStmt(o, s.Then, seen) || g.outerStmt(o, s.Else, seen)
	case *ast.While:
		return g.outerCalls(o, s.Cond, seen) || g.outerStmt(o, s.Body, seen)
	case *ast.Repeat:
		return g.outerStmts(o, s.Body, seen) || g.outerCalls(o, s.Cond, seen)
	case *ast.For:
		return g.outerCalls(o, s.From, seen) || g.outerCalls(o, s.To, seen) || g.outerStmt(o, s.Body, seen)
	case *ast.Case:
		if g.outerCalls(o, s.Expr, seen) {
			return true
		}
		for _, arm := range s.Arms {
			for _, lab := range arm.Labels {
				if g.outerCalls(o, lab, seen) {
					return true
				}
			}
			if g.outerStmt(o, arm.Stmt, seen) {
				return true
			}
		}
		return false
	case *ast.With:
		for _, rec := range s.Recs {
			if g.outerCalls(o, rec, seen) {
				return true
			}
		}
		return g.outerStmt(o, s.Body, seen)
	case *ast.Labeled:
		return g.outerStmt(o, s.Stmt, seen)
	default:
		return false
	}
}

func (g *gen) outerCalls(o *obj, e ast.Expr, seen map[*obj]bool) bool {
	if e == nil {
		return false
	}
	switch e := e.(type) {
	case *ast.CallExpr:
		return g.outerCall(o, e, e.Args, seen)
	case *ast.Paren:
		return g.outerCalls(o, e.X, seen)
	case *ast.Unary:
		return g.outerCalls(o, e.X, seen)
	case *ast.Binary:
		return g.outerCalls(o, e.Left, seen) || g.outerCalls(o, e.Right, seen)
	case *ast.Indexed:
		if g.outerCalls(o, e.Base, seen) {
			return true
		}
		for _, ix := range e.Indexes {
			if g.outerCalls(o, ix, seen) {
				return true
			}
		}
		return false
	case *ast.Select:
		return g.outerCalls(o, e.Base, seen)
	case *ast.Deref:
		return g.outerCalls(o, e.Base, seen)
	case *ast.SetLit:
		for _, el := range e.Elems {
			if g.outerCalls(o, el.Low, seen) || g.outerCalls(o, el.High, seen) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (g *gen) outerCall(o *obj, node any, args []ast.Expr, seen map[*obj]bool) bool {
	if callee := g.c.callOf[node]; callee != nil && callee.predef == "" && callee != o && g.writesOuter(callee, seen) {
		return true
	}
	for _, a := range args {
		if g.outerCalls(o, a, seen) {
			return true
		}
	}
	return false
}

// outerTarget reports that e designates a variable the routine does not own.
// A var parameter is included, because it may alias the caller's actual.
func (g *gen) outerTarget(o *obj, e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.Paren:
		return false
	case *ast.Name:
		op := g.c.raw[e]
		if op.obj == nil || op.obj.kind != kindVar {
			return false
		}
		if op.obj.withBase != nil {
			return g.outerTarget(o, op.obj.withBase)
		}
		if op.obj.byRef {
			return true
		}
		return !ownedBy(o, op.obj)
	case *ast.Indexed:
		return g.outerTarget(o, e.Base)
	case *ast.Select:
		return g.outerTarget(o, e.Base)
	case *ast.Deref:
		return g.outerTarget(o, e.Base)
	default:
		return false
	}
}

// ownedBy reports that v is a local or value parameter of routine or of a
// routine nested inside it.
func ownedBy(routine *obj, v *obj) bool {
	if routine == nil || v == nil || v.block == nil || routine.block == nil {
		return false
	}
	for s := v.block; s != nil; s = s.parent {
		if s.parent == routine.block {
			return true
		}
	}
	return false
}

func (g *gen) predefWrites(p *parm, o *obj, args []ast.Expr) bool {
	if o == nil {
		for _, a := range args {
			if g.mentions(p, a) {
				return true
			}
		}
		return false
	}
	switch o.predef {
	case "pack":
		return len(args) > 2 && g.mentions(p, args[2])
	case "unpack":
		return len(args) > 1 && g.mentions(p, args[1])
	case "new", "dispose", "rewrite", "reset", "get", "put":
		return len(args) > 0 && g.mentions(p, args[0])
	default:
		return false
	}
}

// mentions reports that e is p or a component of p, including a field opened
// by with from that component.
func (g *gen) mentions(p *parm, e ast.Expr) bool {
	if e == nil {
		return false
	}
	switch e := e.(type) {
	case *ast.Paren:
		return g.mentions(p, e.X)
	case *ast.Name:
		op := g.c.raw[e]
		if op.obj == nil {
			return false
		}
		if op.obj.schema == p.schema && op.obj.name == p.name && !op.obj.byRef {
			return true
		}
		if op.obj.withBase != nil {
			return g.mentions(p, op.obj.withBase)
		}
		return false
	case *ast.Indexed:
		return g.mentions(p, e.Base)
	case *ast.Select:
		return g.mentions(p, e.Base)
	case *ast.Deref:
		return g.mentions(p, e.Base)
	default:
		return false
	}
}

func (g *gen) sizeExpr(b []*obj) string {
	if len(b) < 2 {
		return "0"
	}
	var parts []string
	for i := 0; i+1 < len(b); i += 2 {
		lo := g.asIntGo(b[i].typ, g.boundGo(b[i]))
		hi := g.asIntGo(b[i+1].typ, g.boundGo(b[i+1]))
		parts = append(parts, fmt.Sprintf("((%s)-(%s)+1)", hi, lo))
	}
	return strings.Join(parts, "*")
}

func (g *gen) statements(body []ast.Stmt, labels []string) {
	if !labeled(body) {
		for _, s := range body {
			g.stmt(s)
		}
		return
	}
	segs := segments(body)
	if len(segs) == 0 {
		return
	}
	if g.frameAt >= len(g.ready) {
		g.errf("missing goto frame")
		return
	}
	f := g.ready[g.frameAt]
	g.frameAt++
	id, loop := f.pc, f.loop
	prevLoop, prevPC := g.loop, g.pc
	g.loop, g.pc = loop, id
	g.active = append(g.active, f)
	// The counter stays on the last label reached. A loop, or any other
	// path that enters this list again, has to start at the first segment.
	// A goto continues at the loop label below and does not pass this reset.
	g.line("%s = %d", id, f.first)
	g.line("%s:", loop)
	g.line("for {")
	g.ind++
	g.line("switch %s {", id)
	for i, seg := range segs {
		g.line("case %d:", seg.id)
		g.ind++
		for _, s := range seg.stmts {
			g.stmt(s)
		}
		if !endsGoto(seg.stmts) {
			if i+1 < len(segs) {
				g.line("%s = %d", id, segs[i+1].id)
				g.line("continue %s", loop)
			} else {
				g.line("break %s", loop)
			}
		}
		g.ind--
	}
	g.line("default:")
	g.ind++
	g.line("break %s", loop)
	g.ind--
	g.line("}")
	g.ind--
	g.line("}")
	g.loop, g.pc = prevLoop, prevPC
	g.active = g.active[:len(g.active)-1]
	_ = labels
}

type seg struct {
	id    int
	stmts []ast.Stmt
}

func (g *gen) collectFrames(body []ast.Stmt) []*labFrame {
	var out []*labFrame
	var walkBody func([]ast.Stmt)
	var walk func(ast.Stmt)
	walkBody = func(body []ast.Stmt) {
		if labeled(body) {
			segs := segments(body)
			if len(segs) > 0 {
				f := &labFrame{
					pc:     fmt.Sprintf("_pc%d", g.n),
					loop:   fmt.Sprintf("_L%d", g.n),
					first:  segs[0].id,
					depth:  g.depth,
					labels: map[int]bool{},
				}
				g.n++
				for _, seg := range segs {
					if seg.id >= 0 {
						f.labels[seg.id] = true
					}
				}
				out = append(out, f)
			}
		}
		for _, s := range body {
			walk(s)
		}
	}
	walk = func(s ast.Stmt) {
		if s == nil {
			return
		}
		switch s := s.(type) {
		case *ast.Labeled:
			walk(s.Stmt)
		case *ast.Compound:
			walkBody(s.Body)
		case *ast.If:
			walk(s.Then)
			walk(s.Else)
		case *ast.While:
			walk(s.Body)
		case *ast.Repeat:
			walkBody(s.Body)
		case *ast.For:
			walk(s.Body)
		case *ast.Case:
			for _, arm := range s.Arms {
				walk(arm.Stmt)
			}
		case *ast.With:
			walk(s.Body)
		}
	}
	walkBody(body)
	return out
}

func labeled(body []ast.Stmt) bool {
	for _, s := range body {
		switch s.(type) {
		case *ast.Labeled, *ast.Goto:
			return true
		}
	}
	return false
}

func endsGoto(stmts []ast.Stmt) bool {
	if len(stmts) == 0 {
		return false
	}
	_, ok := stmts[len(stmts)-1].(*ast.Goto)
	return ok
}

func segments(body []ast.Stmt) []seg {
	// The statements before the first label are not label 0. Pascal labels
	// start at 0, so the prefix uses an id no label can have.
	cur := seg{id: -1}
	var out []seg
	started := false
	for _, s := range body {
		if s == nil {
			continue
		}
		if lab, ok := s.(*ast.Labeled); ok {
			if started {
				out = append(out, cur)
			}
			n, _ := strconv.Atoi(lab.Label)
			cur = seg{id: n}
			started = true
			if lab.Stmt != nil {
				cur.stmts = append(cur.stmts, lab.Stmt)
			}
			continue
		}
		cur.stmts = append(cur.stmts, s)
		started = true
	}
	if started {
		out = append(out, cur)
	}
	if len(out) > 1 && out[0].id == 0 && len(out[0].stmts) == 0 {
		out = out[1:]
	}
	return out
}

func (g *gen) stmt(s ast.Stmt) {
	if s == nil {
		return
	}
	switch s := s.(type) {
	case *ast.Labeled:
		g.labeledStmt(s)
	case *ast.Assign:
		g.assign(s.Left, s.Value)
	case *ast.Call:
		g.callStmt(s)
	case *ast.Read:
		g.read(s)
	case *ast.Write:
		g.write(s)
	case *ast.Goto:
		g.gotoStmt(s)
	case *ast.Compound:
		g.line("{")
		g.ind++
		g.statements(s.Body, nil)
		g.ind--
		g.line("}")
	case *ast.If:
		g.line("if %s {", g.value(s.Cond))
		g.ind++
		g.stmt(s.Then)
		g.ind--
		if s.Else != nil {
			g.line("} else {")
			g.ind++
			g.stmt(s.Else)
			g.ind--
		}
		g.line("}")
	case *ast.While:
		g.line("for %s {", g.value(s.Cond))
		g.ind++
		g.stmt(s.Body)
		g.ind--
		g.line("}")
	case *ast.Repeat:
		g.line("for {")
		g.ind++
		g.statements(s.Body, nil)
		g.line("if %s {", g.value(s.Cond))
		g.ind++
		g.line("break")
		g.ind--
		g.line("}")
		g.ind--
		g.line("}")
	case *ast.For:
		g.forStmt(s)
	case *ast.Case:
		g.caseStmt(s)
	case *ast.With:
		g.withStmt(s)
	default:
		g.errf("unknown statement")
	}
}

// labeledStmt is a label on one statement, such as the else branch
// 6: case ... of false: goto 6. A statement list owns its own labels.
func (g *gen) labeledStmt(s *ast.Labeled) {
	n, err := strconv.Atoi(s.Label)
	if err != nil {
		g.stmt(s.Stmt)
		return
	}
	f := &labFrame{
		pc:     fmt.Sprintf("_pc%d", g.n),
		loop:   fmt.Sprintf("_L%d", g.n),
		first:  n,
		depth:  g.depth,
		labels: map[int]bool{n: true},
	}
	g.n++
	g.frames = append(g.frames, f)
	g.active = append(g.active, f)
	prevLoop, prevPC := g.loop, g.pc
	g.loop, g.pc = f.loop, f.pc
	g.line("var %s = %d", f.pc, n)
	g.line("%s:", f.loop)
	g.line("for {")
	g.ind++
	g.stmt(s.Stmt)
	g.line("break %s", f.loop)
	g.ind--
	g.line("}")
	g.line("_ = %s", f.pc)
	g.loop, g.pc = prevLoop, prevPC
	g.active = g.active[:len(g.active)-1]
	g.frames = g.frames[:len(g.frames)-1]
}

func (g *gen) gotoStmt(s *ast.Goto) {
	n, err := strconv.Atoi(s.Label)
	if err != nil {
		g.errf("goto %s is not in a labeled block", s.Label)
		return
	}
	var target *labFrame
	for i := len(g.frames) - 1; i >= 0; i-- {
		if g.frames[i].labels[n] {
			target = g.frames[i]
			break
		}
	}
	if target == nil {
		g.errf("goto %s is not in a labeled block", s.Label)
		return
	}
	g.line("%s = %d", target.pc, n)
	if target.depth == g.depth {
		g.line("continue %s", target.loop)
		return
	}
	g.line("_unwind = &%s", target.pc)
	g.ret()
}

func (g *gen) ret() {
	if g.fn != nil && g.fn.kind == kindFunc {
		g.line("return %s", g.resultVar(g.fn))
		return
	}
	g.line("return")
}

func (g *gen) afterCall() {
	if !g.c.farGoto {
		return
	}
	for i := len(g.active) - 1; i >= 0; i-- {
		f := g.active[i]
		g.line("if _unwind == &%s {", f.pc)
		g.ind++
		g.line("_unwind = nil")
		g.line("continue %s", f.loop)
		g.ind--
		g.line("}")
	}
	g.line("if _unwind != nil {")
	g.ind++
	g.ret()
	g.ind--
	g.line("}")
}

func (g *gen) resultVar(o *obj) string {
	if g.resName == nil {
		g.resName = map[*obj]string{}
	}
	if n, ok := g.resName[o]; ok {
		return n
	}
	n := fmt.Sprintf("_res%d", len(g.resName))
	g.resName[o] = n
	return n
}

func (g *gen) assign(left, right ast.Expr) {
	dst := g.op(left)
	if dst.fn {
		name := g.resultVar(dst.obj)
		rhs := g.checked(dst.typ, g.op(right).typ, g.value(right))
		g.line("%s = %s", name, g.cast(g.op(right).typ, dst.typ, rhs))
		return
	}
	if g.assignArray(left, right) {
		return
	}
	rhs := g.checked(dst.typ, g.op(right).typ, g.value(right))
	g.line("%s = %s", g.storeTarget(left), g.cast(g.op(right).typ, dst.typ, rhs))
}

// assignArray copies one array into another when either side is a conformant
// array. Those are pointers to their first element, so a Go assignment would
// retarget the pointer instead of copying the value.
func (g *gen) assignArray(left, right ast.Expr) bool {
	if _, ok := g.op(left).typ.(*arrayType); !ok {
		return false
	}
	lv, rv := g.viewOf(left), g.viewOf(right)
	if lv.conf == nil && rv.conf == nil {
		return false
	}
	g.use(runtime.PartMem)
	g.line("rtMove(%s, %s, int(%s))", g.elemPtr(rv), g.elemPtr(lv), g.viewCount(lv))
	return true
}

func (g *gen) elemPtr(v arrView) string {
	if v.conf != nil {
		return v.place
	}
	return "&(" + zeros(v.place, v.typ) + ")"
}

func (g *gen) viewCount(v arrView) string {
	if v.conf != nil {
		return g.sizeExpr(v.bnds)
	}
	n := 1
	t := v.typ
	for {
		a, ok := t.(*arrayType)
		if !ok {
			break
		}
		lo, hi, known := ordinalSpan(a.index)
		size, fit := 0, false
		if known {
			size, fit = fitLen(lo, hi)
		}
		if !fit {
			return "0"
		}
		n *= size
		t = a.elem
	}
	return strconv.Itoa(n)
}

// checked emits a range test for rhs when one is required, and returns the
// expression that holds the value. A subrange store and a set whose base is
// known are tested. Other values are returned unchanged.
func (g *gen) checked(dst, src Type, rhs string) string {
	if !g.opt.Range {
		return rhs
	}
	if lo, hi, ok := subrangeSpan(dst); ok {
		tmp := g.fresh()
		g.line("%s := %s", tmp, rhs)
		g.use(runtime.PartBase)
		g.line("rtInRange(%s, %s, %s)", g.asIntGo(src, tmp), g.intLit(lo), g.intLit(hi))
		return tmp
	}
	if st, ok := dst.(*setType); ok {
		lo, hi, ok := ordinalSpan(st.base)
		if !ok {
			return rhs
		}
		tmp := g.fresh()
		g.line("%s := %s", tmp, rhs)
		g.use(runtime.PartBase)
		g.line("rtSetFit(rtBits(%s), %s, %s)", tmp, g.intLit(lo), g.intLit(hi))
		return tmp
	}
	return rhs
}

func subrangeSpan(t Type) (int64, int64, bool) {
	s, ok := t.(*subrangeType)
	if !ok || s.lo == nil || s.hi == nil || !s.lo.known || !s.hi.known {
		return 0, 0, false
	}
	return s.lo.n, s.hi.n, true
}

func (g *gen) forStmt(s *ast.For) {
	o := g.c.forOf[s]
	t := integerType
	if o != nil && o.typ != nil {
		t = o.typ
	}
	name := g.goName(s.Name)
	if o != nil && o.block == g.prog && g.prog != nil {
		name = g.hoist(o.name)
	}
	g.noteUse(o)
	gt := g.goType(t)
	a, b := g.fresh(), g.fresh()
	g.line("{")
	g.ind++
	g.line("var %s %s = %s", a, gt, g.cast(g.op(s.From).typ, t, g.value(s.From)))
	g.line("var %s %s = %s", b, gt, g.cast(g.op(s.To).typ, t, g.value(s.To)))
	g.line("%s = %s", name, a)
	op := "<="
	if s.Down {
		op = ">="
	}
	g.line("if %s {", g.rel(name, b, t, op))
	g.ind++
	if lo, hi, ok := subrangeSpan(t); ok && g.opt.Range {
		g.use(runtime.PartBase)
		g.line("rtInRange(%s, %s, %s)", g.asIntGo(t, name), g.intLit(lo), g.intLit(hi))
		g.line("rtInRange(%s, %s, %s)", g.asIntGo(t, b), g.intLit(lo), g.intLit(hi))
	}
	g.line("for {")
	g.ind++
	g.stmt(s.Body)
	g.line("if %s == %s {", name, b)
	g.ind++
	g.line("break")
	g.ind--
	g.line("}")
	g.bump(name, t, s.Down)
	g.ind--
	g.line("}")
	g.ind--
	g.line("}")
	g.ind--
	g.line("}")
}

func (g *gen) rel(left, right string, t Type, op string) string {
	if host(t) != booleanType {
		return fmt.Sprintf("%s %s %s", left, op, right)
	}
	switch op {
	case "<=":
		return fmt.Sprintf("(!%s || %s)", left, right)
	case ">=":
		return fmt.Sprintf("(%s || !%s)", left, right)
	default:
		return fmt.Sprintf("%s %s %s", left, op, right)
	}
}

func (g *gen) bump(name string, t Type, down bool) {
	if host(t) == booleanType {
		g.line("%s = !%s", name, name)
		return
	}
	if down {
		g.line("%s = %s - 1", name, name)
		return
	}
	g.line("%s = %s + 1", name, name)
}

func (g *gen) caseStmt(s *ast.Case) {
	g.line("switch %s {", g.value(s.Expr))
	for _, arm := range s.Arms {
		labs := make([]string, len(arm.Labels))
		for i, lab := range arm.Labels {
			labs[i] = g.value(lab)
		}
		g.line("case %s:", strings.Join(labs, ", "))
		g.ind++
		g.stmt(arm.Stmt)
		g.ind--
	}
	if g.opt.Range {
		g.line("default:")
		g.ind++
		g.line("panic(\"case\")")
		g.ind--
	}
	g.line("}")
}

func (g *gen) withStmt(s *ast.With) {
	for _, rec := range s.Recs {
		tmp := g.fresh()
		g.line("%s := &(%s)", tmp, g.lvalue(rec))
		g.withTmp[rec] = tmp
	}
	g.withLive = append(g.withLive, s.Recs...)
	g.stmt(s.Body)
	g.withLive = g.withLive[:len(g.withLive)-len(s.Recs)]
	for _, rec := range s.Recs {
		g.line("_ = %s", g.withTmp[rec])
	}
}

// withRef reports that an active with statement holds the variable p^.
// Disposing that pointer removes a variable the with still names.
func (g *gen) withRef(p ast.Expr) bool {
	name, ok := p.(*ast.Name)
	if !ok {
		return false
	}
	for _, rec := range g.withLive {
		d, ok := rec.(*ast.Deref)
		if !ok {
			continue
		}
		n, ok := d.Base.(*ast.Name)
		if ok && n.Ident == name.Ident {
			return true
		}
	}
	return false
}

func (g *gen) callStmt(s *ast.Call) {
	o := g.c.callOf[s]
	if o != nil && o.predef != "" {
		g.predefStmt(o.predef, s.Args)
		return
	}
	g.line("%s", g.call(o, s.Args))
	g.afterCall()
}

func (g *gen) call(o *obj, args []ast.Expr) string {
	if o != nil && o.predef != "" {
		return g.predefFun(o.predef, args)
	}
	name := "missing"
	var ps []*parm
	if o != nil {
		name = g.fnName(o)
		ps = o.params
	}
	return name + "(" + strings.Join(g.callArgs(ps, args), ", ") + ")"
}

func (g *gen) callArgs(ps []*parm, args []ast.Expr) []string {
	var out []string
	ai := 0
	for i := 0; i < len(ps) && ai < len(args); {
		p := ps[i]
		if p.schema == nil {
			switch p.kind {
			case parmProc, parmFunc:
				if n, ok := args[ai].(*ast.Name); ok {
					out = append(out, g.procArg(n))
				} else {
					g.errf("procedural argument is not a name")
				}
			case parmVar:
				out = append(out, g.addr(args[ai]))
			default:
				v := g.checked(p.typ, g.op(args[ai]).typ, g.value(args[ai]))
				out = append(out, g.cast(g.op(args[ai]).typ, p.typ, v))
			}
			ai++
			i++
			continue
		}
		sec := p.section
		var grp []*parm
		for i < len(ps) && ps[i].schema != nil && ps[i].section == sec {
			grp = append(grp, ps[i])
			i++
		}
		var bounds []string
		for gi, q := range grp {
			if ai >= len(args) {
				break
			}
			ptr, dyn, nums := g.scalarPtr(args[ai])
			out = append(out, ptr)
			if gi == 0 {
				if dyn != nil {
					for bi, b := range q.bounds {
						if bi < len(dyn) {
							bounds = append(bounds, g.fromInt(b.typ, dyn[bi]))
						}
					}
				} else {
					for bi, b := range q.bounds {
						if bi < len(nums) {
							bounds = append(bounds, g.boundLit(nums[bi], b.typ))
						}
					}
				}
			}
			ai++
		}
		out = append(out, bounds...)
	}
	return out
}

func (g *gen) addr(e ast.Expr) string {
	if n, ok := e.(*ast.Name); ok {
		op := g.op(e)
		if op.obj != nil && op.obj.byRef && op.obj.withBase == nil && op.obj.schema == nil {
			g.noteUse(op.obj)
			return g.goName(n.Ident)
		}
	}
	return "&(" + g.lvalue(e) + ")"
}

func (g *gen) predefStmt(name string, args []ast.Expr) {
	switch name {
	case "rewrite", "reset", "get", "put":
		if len(args) != 1 {
			return
		}
		g.useFile(g.op(args[0]).typ)
		meth := strings.ToUpper(name[:1]) + name[1:]
		g.line("%s.%s()", g.lvalue(args[0]), meth)
	case "readln":
		g.use(runtime.PartText)
		g.flushOutput()
		g.line("rtProgramInput.ReadLn()")
	case "writeln":
		g.use(runtime.PartText)
		g.line("rtProgramOutput.Ln()")
	case "page":
		g.use(runtime.PartText)
		recv := "rtProgramOutput"
		if len(args) == 1 {
			recv = g.lvalue(args[0])
		}
		g.line("%s.Page()", recv)
	case "new":
		if len(args) < 1 {
			return
		}
		op := g.op(args[0])
		p, ok := op.typ.(*pointerType)
		if !ok || p.base == nil {
			g.errf("new requires a pointer")
			return
		}
		g.line("%s = new(%s)", g.storeTarget(args[0]), g.goType(p.base))
	case "dispose":
		if len(args) < 1 {
			return
		}
		op := g.op(args[0])
		if g.withRef(args[0]) {
			g.line("panic(\"dispose\")")
			return
		}
		if !op.vr {
			// The pointer is a temporary. Nothing remains to clear.
			g.line("_ = %s", g.value(args[0]))
			return
		}
		g.line("%s = nil", g.storeTarget(args[0]))
	case "pack":
		if len(args) != 3 {
			return
		}
		g.transfer(args[0], args[1], args[2], true)
	case "unpack":
		if len(args) != 3 {
			return
		}
		g.transfer(args[1], args[2], args[0], false)
	default:
		g.errf("unknown procedure %s", name)
	}
}

func (g *gen) transfer(unpacked, index, packed ast.Expr, pack bool) {
	g.use(runtime.PartBase)
	off := g.packOff(unpacked, index)
	dst, src := g.asSlice(packed), g.asSlice(unpacked)
	if pack {
		g.line("rtPack(%s, %s, %s, %t)", dst, src, off, g.opt.Range)
		return
	}
	g.line("rtUnpack(%s, %s, %s, %t)", dst, src, off, g.opt.Range)
}

func (g *gen) packOff(arr, index ast.Expr) string {
	v := g.viewOf(arr)
	if v.conf != nil && len(v.bnds) >= 1 {
		lo := g.asIntGo(v.bnds[0].typ, g.boundGo(v.bnds[0]))
		return fmt.Sprintf("int((%s)-(%s))", g.asIntExpr(index), lo)
	}
	return g.indexOff(g.arrayBase(arr), index)
}

// asSlice is the Go slice of an array variable. A conformant array is a
// pointer to its first element, so it cannot be sliced as a Go array.
func (g *gen) asSlice(e ast.Expr) string {
	v := g.viewOf(e)
	if v.conf == nil {
		return "(" + g.lvalue(e) + ")[:]"
	}
	g.use(runtime.PartMem)
	if len(v.bnds) < 2 {
		return "nil"
	}
	lo := g.asIntGo(v.bnds[0].typ, g.boundGo(v.bnds[0]))
	hi := g.asIntGo(v.bnds[1].typ, g.boundGo(v.bnds[1]))
	n := fmt.Sprintf("((%s)-(%s)+1)", hi, lo)
	return fmt.Sprintf("unsafe.Slice(%s, int(%s))", v.place, n)
}

func (g *gen) arrayBase(e ast.Expr) Type {
	t := g.op(e).typ
	if a, ok := t.(*arrayType); ok {
		return a.index
	}
	return integerType
}

func (g *gen) indexOff(index Type, e ast.Expr) string {
	lo, _, ok := ordinalSpan(index)
	if !ok {
		lo = 0
	}
	return fmt.Sprintf("int((%s)-(%s))", g.asIntExpr(e), g.intLit(lo))
}

// flushOutput makes a prompt written without a line marker visible
// before the program waits to read.
func (g *gen) flushOutput() {
	if g.c.usedOutput {
		g.line("rtProgramOutput.Flush()")
	}
}

func (g *gen) read(s *ast.Read) {
	vars := s.Vars
	file := "rtProgramInput"
	text := true
	var elem Type
	if len(vars) > 0 && isFile(g.op(vars[0]).typ) && g.op(vars[0]).vr {
		ft := g.op(vars[0]).typ
		file = g.filePtr(g.lvalue(vars[0]))
		_, text = ft.(textType)
		if f, ok := ft.(*fileType); ok {
			elem = f.elem
		}
		vars = vars[1:]
	}
	if text {
		g.use(runtime.PartText)
		g.flushOutput()
	} else {
		g.use(runtime.PartFile)
	}
	for _, v := range vars {
		dst := g.op(v).typ
		if text {
			switch host(dst) {
			case realType:
				g.line("%s = %s.ReadReal()", g.storeTarget(v), file)
			case charType:
				rhs := g.checked(dst, charType, file+".ReadChar()")
				g.line("%s = %s", g.storeTarget(v), rhs)
			default:
				rhs := g.checked(dst, integerType, file+".ReadInt()")
				g.line("%s = %s", g.storeTarget(v), rhs)
			}
			continue
		}
		src := elem
		if src == nil {
			src = dst
		}
		rhs := g.checked(dst, src, file+".Read()")
		g.line("%s = %s", g.storeTarget(v), g.cast(src, dst, rhs))
	}
	if s.Ln {
		g.line("%s.ReadLn()", file)
	}
}

func (g *gen) write(s *ast.Write) {
	if o := g.c.callOf[s]; o != nil && o.predef == "" {
		var args []ast.Expr
		for _, a := range s.Args {
			args = append(args, a.Value)
		}
		g.line("%s", g.call(o, args))
		g.afterCall()
		return
	}
	args := s.Args
	file := "rtProgramOutput"
	text := true
	var elem Type
	if len(args) > 0 && args[0].Width == nil && args[0].Frac == nil {
		op := g.op(args[0].Value)
		if isFile(op.typ) && op.vr {
			file = g.filePtr(g.lvalue(args[0].Value))
			_, text = op.typ.(textType)
			if f, ok := op.typ.(*fileType); ok {
				elem = f.elem
			}
			args = args[1:]
		}
	}
	if text {
		g.use(runtime.PartText)
	} else {
		g.use(runtime.PartFile)
	}
	for _, a := range args {
		if !text {
			src := g.op(a.Value).typ
			rhs := g.value(a.Value)
			if elem != nil {
				rhs = g.checked(elem, src, rhs)
				rhs = g.cast(src, elem, rhs)
			}
			g.line("%s.Write(%s)", file, rhs)
			continue
		}
		g.writeArg(file, a)
	}
	if s.Ln {
		g.line("%s.Ln()", file)
	}
}

func (g *gen) writeArg(file string, a ast.WriteArg) {
	w := "-1"
	if a.Width != nil {
		w = "int(" + g.asIntExpr(a.Width) + ")"
	}
	t := g.op(a.Value).typ
	switch {
	case host(t) == integerType:
		g.line("%s.WriteInt(%s, %s)", file, g.value(a.Value), w)
	case host(t) == realType:
		f := "-1"
		if a.Frac != nil {
			f = "int(" + g.asIntExpr(a.Frac) + ")"
		}
		g.line("%s.WriteReal(%s, %s, %s)", file, g.asReal(a.Value), w, f)
	case host(t) == booleanType:
		g.line("%s.WriteBool(%s, %s)", file, g.value(a.Value), w)
	case host(t) == charType:
		g.line("%s.WriteChar(%s, %s)", file, g.value(a.Value), w)
	default:
		if _, ok := stringLen(t); ok {
			if s, ok := a.Value.(*ast.Str); ok {
				g.line("%s.WriteStr(%s, %s)", file, strconv.Quote(s.Value), w)
				return
			}
			g.line("%s.WriteStr(string((%s)[:]), %s)", file, g.value(a.Value), w)
			return
		}
		g.errf("cannot write this value")
	}
}

func (g *gen) predefFun(name string, args []ast.Expr) string {
	one := func() string {
		if len(args) == 0 {
			return "0"
		}
		return g.value(args[0])
	}
	switch name {
	case "abs":
		if len(args) == 1 && host(g.op(args[0]).typ) == integerType {
			g.use(runtime.PartBase)
			return fmt.Sprintf("rtAbsInt(%s, %t)", one(), g.opt.Overflow)
		}
		g.use(runtime.PartReal)
		return "rtAbsReal(" + g.asReal(args[0]) + ")"
	case "sqr":
		if len(args) == 1 && host(g.op(args[0]).typ) == integerType {
			if g.opt.Overflow {
				g.use(runtime.PartBase)
				return fmt.Sprintf("rtMul(%s, %s, true)", one(), one())
			}
			return "(" + one() + " * " + one() + ")"
		}
		v := g.asReal(args[0])
		return "(" + v + " * " + v + ")"
	case "sin":
		g.use(runtime.PartReal)
		return "rtSin(" + g.asReal(args[0]) + ")"
	case "cos":
		g.use(runtime.PartReal)
		return "rtCos(" + g.asReal(args[0]) + ")"
	case "exp":
		g.use(runtime.PartReal)
		return "rtExp(" + g.asReal(args[0]) + ")"
	case "ln":
		g.use(runtime.PartReal)
		return "rtLn(" + g.asReal(args[0]) + ")"
	case "sqrt":
		g.use(runtime.PartReal)
		return "rtSqrt(" + g.asReal(args[0]) + ")"
	case "arctan":
		g.use(runtime.PartReal)
		return "rtArctan(" + g.asReal(args[0]) + ")"
	case "trunc":
		g.use(runtime.PartBase)
		return "rtTrunc(" + g.asReal(args[0]) + ")"
	case "round":
		g.use(runtime.PartReal)
		return "rtRound(" + g.asReal(args[0]) + ")"
	case "ord":
		return g.asIntExpr(args[0])
	case "chr":
		g.use(runtime.PartBase)
		return fmt.Sprintf("rtChr(%s, %t)", g.asIntExpr(args[0]), g.opt.Range)
	case "succ", "pred":
		return g.step(name, args[0])
	case "odd":
		return "((" + g.asIntExpr(args[0]) + ") & 1 != 0)"
	case "eof":
		if len(args) == 0 {
			g.use(runtime.PartText)
		} else {
			g.useFile(g.op(args[0]).typ)
		}
		return g.fileRecv(args, true) + ".Eof()"
	case "eoln":
		if len(args) == 0 {
			g.use(runtime.PartText)
		} else {
			g.useFile(g.op(args[0]).typ)
		}
		return g.fileRecv(args, true) + ".Eoln()"
	default:
		g.errf("unknown function %s", name)
		return "0"
	}
}

func (g *gen) fileRecv(args []ast.Expr, input bool) string {
	if len(args) == 0 {
		if input {
			return "rtProgramInput"
		}
		return "rtProgramOutput"
	}
	return g.lvalue(args[0])
}

// filePtr evaluates a file variable once. Later reads use that pointer,
// so an index in the file expression is not computed again.
func (g *gen) filePtr(expr string) string {
	tmp := g.fresh()
	g.line("%s := &(%s)", tmp, expr)
	return tmp
}

func (g *gen) step(name string, arg ast.Expr) string {
	dir := 1
	if name == "pred" {
		dir = -1
	}
	t := g.op(arg).typ
	lo, hi := "-rtMaxInt", "rtMaxInt"
	if l, h, ok := ordinalSpan(t); ok {
		lo, hi = g.intLit(l), g.intLit(h)
	}
	g.use(runtime.PartBase)
	inner := fmt.Sprintf("rtStep(%s, %s, %s, %d, %t)", g.asIntExpr(arg), lo, hi, dir, g.opt.Range)
	return g.fromInt(t, inner)
}

func (g *gen) value(e ast.Expr) string {
	if e == nil {
		return "0"
	}
	if p, ok := e.(*ast.Paren); ok {
		return g.value(p.X)
	}
	op := g.op(e)
	if isSetType(op.typ) || isEmptySet(op.typ) {
		return g.setBits(e)
	}
	switch e := e.(type) {
	case *ast.Int:
		if op.val != nil && op.val.known {
			return strconv.FormatInt(op.val.n, 10)
		}
		return "0"
	case *ast.Real:
		return realLit(e.Text)
	case *ast.Str:
		if len([]rune(e.Value)) == 1 {
			return charLit(int64([]rune(e.Value)[0]))
		}
		return g.strLit(e.Value)
	case *ast.Nil:
		return "nil"
	case *ast.Name:
		return g.valueName(e)
	case *ast.Unary:
		x := g.value(e.X)
		if e.Op == "not" {
			return "!(" + x + ")"
		}
		if e.Op == "-" {
			if g.opt.Overflow && host(g.op(e.X).typ) == integerType {
				g.use(runtime.PartBase)
				return fmt.Sprintf("rtSub(0, %s, true)", x)
			}
			return "(-(" + x + "))"
		}
		return "(+(" + x + "))"
	case *ast.Binary:
		return g.binary(e)
	case *ast.CallExpr:
		return g.call(g.c.callOf[e], e.Args)
	case *ast.SetLit:
		return g.setLit(e)
	case *ast.Indexed, *ast.Select, *ast.Deref:
		return g.lvalue(e)
	default:
		g.errf("unknown expression")
		return "0"
	}
}

func (g *gen) valueName(e *ast.Name) string {
	op := g.op(e)
	o := op.obj
	if o == nil {
		return g.goName(e.Ident)
	}
	switch o.kind {
	case kindConst:
		if o.block == nil && o.name == "true" {
			return "true"
		}
		if o.block == nil && o.name == "false" {
			return "false"
		}
		if o.block == nil && o.name == "maxint" {
			g.needMax = true
			return "maxint"
		}
		if e, ok := host(o.typ).(*enumType); ok && enumMember(e, o.name) {
			return g.enumGo(e, o.name)
		}
		if o.block == g.prog && g.prog != nil {
			return g.hoist(o.name)
		}
		return g.goName(o.name)
	case kindVar:
		return g.lvalue(e)
	case kindFunc:
		if op.fn {
			return g.resultVar(o)
		}
		if o.predef != "" {
			return g.predefFun(o.predef, nil)
		}
		return g.fnName(o) + "()"
	default:
		if o.kind == kindProc {
			return g.fnName(o)
		}
		return g.goName(o.name)
	}
}

func (g *gen) procArg(n *ast.Name) string {
	op := g.op(n)
	if op.obj != nil && op.obj.predef == "" && (op.obj.kind == kindProc || op.obj.kind == kindFunc) {
		return g.fnName(op.obj)
	}
	return g.goName(n.Ident)
}

func (g *gen) binary(e *ast.Binary) string {
	lt, rt := g.op(e.Left).typ, g.op(e.Right).typ
	if isSetType(lt) || isSetType(rt) || isEmptySet(lt) || isEmptySet(rt) {
		return g.setOp(e)
	}
	ln, lok := stringLen(lt)
	rn, rok := stringLen(rt)
	if lok && rok && ln == rn && (e.Op == "<" || e.Op == ">" || e.Op == "<=" || e.Op == ">=") {
		return g.strCmp(e.Op, e.Left, e.Right)
	}
	l, r := g.value(e.Left), g.value(e.Right)
	real := host(lt) == realType || host(rt) == realType
	switch e.Op {
	case "and":
		return g.boolOp("&&", l, r)
	case "or":
		return g.boolOp("||", l, r)
	case "/":
		g.use(runtime.PartBase)
		return fmt.Sprintf("rtQuo(%s, %s)", g.asReal(e.Left), g.asReal(e.Right))
	case "div":
		return "(" + l + " / " + r + ")"
	case "mod":
		g.use(runtime.PartBase)
		return fmt.Sprintf("rtMod(%s, %s)", l, r)
	case "+", "-", "*":
		if real {
			return "(" + g.asReal(e.Left) + " " + e.Op + " " + g.asReal(e.Right) + ")"
		}
		if host(lt) == integerType && (g.opt.Overflow || g.constOverflow(e.Op, e.Left, e.Right)) {
			g.use(runtime.PartBase)
			fn := map[string]string{"+": "Add", "-": "Sub", "*": "Mul"}[e.Op]
			return fmt.Sprintf("rt%s(%s, %s, %t)", fn, l, r, g.opt.Overflow)
		}
		return "(" + l + " " + e.Op + " " + r + ")"
	case "=":
		return "(" + g.cmpSide(e.Left, real) + " == " + g.cmpSide(e.Right, real) + ")"
	case "<>":
		return "(" + g.cmpSide(e.Left, real) + " != " + g.cmpSide(e.Right, real) + ")"
	case "<", ">", "<=", ">=":
		if host(lt) == booleanType || host(rt) == booleanType {
			g.use(runtime.PartBase)
			return fmt.Sprintf("(rtB2I(%s) %s rtB2I(%s))", l, e.Op, r)
		}
		return "(" + g.cmpSide(e.Left, real) + " " + e.Op + " " + g.cmpSide(e.Right, real) + ")"
	default:
		g.errf("unknown operator %s", e.Op)
		return "false"
	}
}

// constOverflow reports that both operands are known integers and the
// operation does not fit in -maxint..maxint. The call is not a Go constant,
// so the host compiler does not reject it.
func (g *gen) constOverflow(op string, left, right ast.Expr) bool {
	return intOp(op, g.op(left).val, g.op(right).val) == nil &&
		knownInt(g.op(left).val) && knownInt(g.op(right).val)
}

func knownInt(v *value) bool {
	return v != nil && v.known && !v.real
}

func (g *gen) cmpSide(e ast.Expr, real bool) string {
	if real && host(g.op(e).typ) == integerType {
		return g.asReal(e)
	}
	return g.value(e)
}

func (g *gen) boolOp(op, l, r string) string {
	if !g.opt.CompleteBool {
		return "(" + l + " " + op + " " + r + ")"
	}
	return fmt.Sprintf("func() bool { a := %s; b := %s; return a %s b }()", l, r, op)
}

func (g *gen) strCmp(op string, l, r ast.Expr) string {
	g.use(runtime.PartBase)
	neg := false
	switch op {
	case ">":
		l, r = r, l
	case "<=":
		l, r = r, l
		neg = true
	case ">=":
		neg = true
	}
	s := fmt.Sprintf("rtStrLess(%s, %s)", g.strSlice(l), g.strSlice(r))
	if neg {
		return "!(" + s + ")"
	}
	return s
}

// strSlice is the bytes of a Pascal string. A literal is not addressable,
// so it is stored in a temporary by taking its address.
func (g *gen) strSlice(e ast.Expr) string {
	for {
		p, ok := e.(*ast.Paren)
		if !ok {
			break
		}
		e = p.X
	}
	if s, ok := e.(*ast.Str); ok {
		return "(&" + g.strLit(s.Value) + ")[:]"
	}
	return "(" + g.value(e) + ")[:]"
}

func (g *gen) setOp(e *ast.Binary) string {
	g.use(runtime.PartBase)
	if e.Op == "in" {
		return fmt.Sprintf("rtIn(%s, %s)", g.asIntExpr(e.Left), g.setBits(e.Right))
	}
	l, r := g.setBits(e.Left), g.setBits(e.Right)
	switch e.Op {
	case "+":
		return fmt.Sprintf("rtUnion(%s, %s)", l, r)
	case "*":
		return fmt.Sprintf("rtInter(%s, %s)", l, r)
	case "-":
		return fmt.Sprintf("rtDiff(%s, %s)", l, r)
	case "=":
		return fmt.Sprintf("(%s == %s)", l, r)
	case "<>":
		return fmt.Sprintf("(%s != %s)", l, r)
	case "<=":
		return fmt.Sprintf("rtSubset(%s, %s)", l, r)
	case ">=":
		return fmt.Sprintf("rtSubset(%s, %s)", r, l)
	default:
		g.errf("unknown set operator %s", e.Op)
		return "false"
	}
}

func (g *gen) setBits(e ast.Expr) string {
	g.use(runtime.PartBase)
	switch e := e.(type) {
	case *ast.Paren:
		return g.setBits(e.X)
	case *ast.SetLit:
		return g.setLit(e)
	case *ast.Binary:
		if e.Op == "+" || e.Op == "-" || e.Op == "*" {
			return g.setOp(e)
		}
	}
	return "rtBits(" + g.lvalue(e) + ")"
}

func (g *gen) setLit(e *ast.SetLit) string {
	if len(e.Elems) == 0 {
		g.use(runtime.PartBase)
		return "rtBits{}"
	}
	g.use(runtime.PartBase)
	var b strings.Builder
	b.WriteString("func() rtBits { b := rtBits{}; ")
	for _, el := range e.Elems {
		if el.High == nil {
			fmt.Fprintf(&b, "b = rtIncl(b, %s); ", g.asIntExpr(el.Low))
			continue
		}
		fmt.Fprintf(&b, "b = rtInclRange(b, %s, %s); ", g.asIntExpr(el.Low), g.asIntExpr(el.High))
	}
	b.WriteString("return b }()")
	return b.String()
}

// storeTarget is the left-hand side of an assignment. Storing a whole
// variable does not count as a use; storing through it does.
func (g *gen) storeTarget(e ast.Expr) string {
	for {
		p, ok := e.(*ast.Paren)
		if !ok {
			break
		}
		e = p.X
	}
	if n, ok := e.(*ast.Name); ok {
		if name, ok := g.bareName(n); ok {
			return name
		}
	}
	return g.lvalue(e)
}

// bareName is the Go name of a whole variable whose assignment is not a use.
func (g *gen) bareName(e *ast.Name) (string, bool) {
	op := g.op(e)
	o := op.obj
	if o == nil || o.withBase != nil || (o.byRef && o.schema == nil) {
		return "", false
	}
	name := g.goName(e.Ident)
	if o.kind == kindVar && o.block == g.prog && g.prog != nil {
		name = g.hoist(o.name)
	}
	return name, true
}

// boundGo is the Go name of a conformant bound, which is a parameter.
func (g *gen) boundGo(b *obj) string {
	g.noteUse(b)
	return g.goName(b.name)
}

func (g *gen) lvalue(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.Paren:
		return g.value(e.X)
	case *ast.Name:
		op := g.op(e)
		if op.obj != nil && op.obj.withBase != nil {
			tmp := g.withTmp[op.obj.withBase]
			return tmp + "." + g.goName(op.obj.field)
		}
		g.noteUse(op.obj)
		name := g.goName(e.Ident)
		if op.obj != nil && op.obj.kind == kindVar && op.obj.withBase == nil && op.obj.block == g.prog && g.prog != nil {
			name = g.hoist(op.obj.name)
		}
		if op.obj != nil && op.obj.byRef && op.obj.schema == nil {
			return "(*" + name + ")"
		}
		return name
	case *ast.Indexed:
		return g.viewOf(e).place
	case *ast.Select:
		return g.lvalue(e.Base) + "." + g.goName(e.Field)
	case *ast.Deref:
		base := g.op(e.Base)
		switch base.typ.(type) {
		case *fileType, textType:
			return g.lvalue(e.Base) + ".Buf"
		default:
			return "(*" + g.lvalue(e.Base) + ")"
		}
	default:
		return g.value(e)
	}
}

type arrView struct {
	place string
	typ   Type
	conf  *confSchema
	bnds  []*obj
}

func (g *gen) viewOf(e ast.Expr) arrView {
	switch e := e.(type) {
	case *ast.Paren:
		return g.viewOf(e.X)
	case *ast.Indexed:
		v := g.viewOf(e.Base)
		for _, ix := range e.Indexes {
			v = g.stepIndex(v, ix)
		}
		return v
	case *ast.Name:
		op := g.op(e)
		if op.obj != nil && op.obj.schema != nil {
			g.noteUse(op.obj)
			return arrView{place: g.goName(e.Ident), typ: op.obj.typ, conf: op.obj.schema, bnds: op.obj.bounds}
		}
		return arrView{place: g.lvalue(e), typ: op.typ}
	default:
		return arrView{place: g.lvalue(e), typ: g.op(e).typ}
	}
}

func (g *gen) stepIndex(v arrView, ix ast.Expr) arrView {
	a, ok := v.typ.(*arrayType)
	if !ok {
		g.errf("indexing a non-array")
		return v
	}
	if v.conf != nil && len(v.bnds) >= 2 {
		g.use(runtime.PartBase | runtime.PartMem)
		lo := g.asIntGo(v.bnds[0].typ, g.boundGo(v.bnds[0]))
		hi := g.asIntGo(v.bnds[1].typ, g.boundGo(v.bnds[1]))
		diff := fmt.Sprintf("rtIdx(%s, %s, %s, %t)", g.asIntExpr(ix), lo, hi, g.opt.Range)
		stride := g.confStride(a.elem, v.bnds[2:])
		ptr := fmt.Sprintf("rtElem(%s, (%s)*int(%s))", v.place, diff, stride)
		if _, ok := a.elem.(*arrayType); ok {
			return arrView{place: ptr, typ: a.elem, conf: v.conf.nested, bnds: v.bnds[2:]}
		}
		return arrView{place: "(*" + ptr + ")", typ: a.elem}
	}
	lo, _, ok := ordinalSpan(a.index)
	if !ok {
		g.errf("array bounds are not static")
		lo = 0
	}
	off := fmt.Sprintf("int((%s)-(%s))", g.asIntExpr(ix), g.intLit(lo))
	return arrView{place: v.place + "[" + off + "]", typ: a.elem}
}

func (g *gen) confStride(t Type, bnds []*obj) string {
	a, ok := t.(*arrayType)
	if !ok || len(bnds) < 2 {
		return "1"
	}
	lo := g.asIntGo(bnds[0].typ, g.boundGo(bnds[0]))
	hi := g.asIntGo(bnds[1].typ, g.boundGo(bnds[1]))
	return fmt.Sprintf("(((%s)-(%s)+1)*(%s))", hi, lo, g.confStride(a.elem, bnds[2:]))
}

func (g *gen) scalarPtr(e ast.Expr) (ptr string, dyn []string, nums []int64) {
	if p, ok := e.(*ast.Paren); ok {
		return g.scalarPtr(p.X)
	}
	if s, ok := e.(*ast.Str); ok && len([]rune(s.Value)) != 1 {
		n := len([]rune(s.Value))
		tmp := g.fresh()
		g.line("%s := %s", tmp, g.strLit(s.Value))
		return "&" + tmp + "[0]", nil, []int64{1, int64(n)}
	}
	v := g.viewOf(e)
	if v.conf != nil {
		for i := 0; i+1 < len(v.bnds); i += 2 {
			dyn = append(dyn,
				g.asIntGo(v.bnds[i].typ, g.boundGo(v.bnds[i])),
				g.asIntGo(v.bnds[i+1].typ, g.boundGo(v.bnds[i+1])))
		}
		return v.place, dyn, nil
	}
	ptr = "&(" + zeros(v.place, v.typ) + ")"
	t := v.typ
	for {
		a, ok := t.(*arrayType)
		if !ok {
			break
		}
		lo, hi, ok := ordinalSpan(a.index)
		if !ok {
			g.errf("array bounds are not static")
			lo, hi = 0, 0
		}
		nums = append(nums, lo, hi)
		t = a.elem
	}
	return ptr, nil, nums
}

func zeros(place string, t Type) string {
	for {
		a, ok := t.(*arrayType)
		if !ok {
			return place
		}
		place += "[0]"
		t = a.elem
	}
}

func (g *gen) cast(src, dst Type, expr string) string {
	if isSetType(dst) {
		return g.goType(dst) + "(" + expr + ")"
	}
	if host(dst) == realType && host(src) == integerType {
		return "float64(" + expr + ")"
	}
	return expr
}

func (g *gen) asReal(e ast.Expr) string {
	v := g.value(e)
	if host(g.op(e).typ) == integerType {
		return "float64(" + v + ")"
	}
	return v
}

func (g *gen) asIntExpr(e ast.Expr) string {
	return g.asIntGo(g.op(e).typ, g.value(e))
}

func (g *gen) asIntGo(t Type, expr string) string {
	switch host(t) {
	case charType:
		return "int(" + expr + ")"
	case booleanType:
		g.use(runtime.PartBase)
		return "rtB2I(" + expr + ")"
	default:
		if _, ok := host(t).(*enumType); ok {
			return "int(" + expr + ")"
		}
		return expr
	}
}

func (g *gen) fromInt(t Type, expr string) string {
	switch host(t) {
	case charType:
		return "uint8(" + expr + ")"
	case booleanType:
		return "(" + expr + " != 0)"
	default:
		if _, ok := host(t).(*enumType); ok {
			return g.goType(host(t)) + "(" + expr + ")"
		}
		if name, ok := g.names[t]; ok && name != "" && host(t) != t {
			return expr
		}
		return expr
	}
}

func (g *gen) boundLit(n int64, t Type) string {
	return g.fromInt(t, g.intLit(n))
}

// intLit is a Go integer literal. The architecture's maxint is written as the
// name maxint so the same source is valid for a 32-bit or a 64-bit int.
func (g *gen) intLit(n int64) string {
	max := int64(^uint(0) >> 1)
	switch n {
	case max:
		g.needMax = true
		return "maxint"
	case -max:
		g.needMax = true
		return "(-maxint)"
	default:
		return strconv.FormatInt(n, 10)
	}
}

func (g *gen) strLit(s string) string {
	rs := []rune(s)
	g.strs[len(rs)] = true
	var b strings.Builder
	fmt.Fprintf(&b, "str%d{", len(rs))
	for i, r := range rs {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(charLit(int64(r)))
	}
	b.WriteByte('}')
	return b.String()
}

func realLit(text string) string {
	if strings.ContainsAny(text, ".eE") {
		return text
	}
	return text + ".0"
}

func charLit(n int64) string {
	if n >= 32 && n < 127 && n != '\\' && n != '\'' {
		return fmt.Sprintf("'%c'", rune(n))
	}
	return fmt.Sprintf("uint8(%d)", n)
}

func (g *gen) op(e ast.Expr) operand {
	if e == nil {
		return operand{typ: invalid}
	}
	if op, ok := g.c.raw[e]; ok {
		return op
	}
	return operand{typ: invalid}
}

func (g *gen) typ(t ast.Type) Type {
	if t == nil {
		return invalid
	}
	if got, ok := g.c.typeOf[t]; ok {
		return got
	}
	return invalid
}

func scalarOf(t Type) Type {
	for {
		a, ok := t.(*arrayType)
		if !ok {
			return t
		}
		t = a.elem
	}
}

func isSetType(t Type) bool {
	_, ok := t.(*setType)
	return ok
}

func isEmptySet(t Type) bool {
	_, ok := t.(emptySetType)
	return ok
}

func (g *gen) bindTypes(defs []*ast.TypeDef) {
	if g.names == nil {
		g.names = map[Type]string{}
	}
	for _, d := range defs {
		t := g.typ(d.Type)
		name := g.unique(g.goName(d.Name))
		if n, ok := stringLen(t); ok {
			g.strs[n] = true
			g.aliases = append(g.aliases, fmt.Sprintf("%s = str%d", name, n))
			g.names[t] = fmt.Sprintf("str%d", n)
			continue
		}
		if _, ok := t.(*subrangeType); ok {
			g.names[t] = name
			g.aliases = append(g.aliases, fmt.Sprintf("%s = %s", name, g.goType(host(t))))
			continue
		}
		if !nominal(t) {
			g.aliases = append(g.aliases, fmt.Sprintf("%s = %s", name, g.basicGo(t)))
			continue
		}
		if prev, ok := g.names[t]; ok {
			g.aliases = append(g.aliases, fmt.Sprintf("%s = %s", name, prev))
			continue
		}
		g.names[t] = name
		g.queue = append(g.queue, t)
	}
}

func nominal(t Type) bool {
	switch t.(type) {
	case *enumType, *arrayType, *recordType, *setType, *fileType, *pointerType:
		return true
	default:
		return false
	}
}

// declTypes writes every Pascal type at package scope. Inside a function,
// a type group cannot refer to a type declared later in that same group.
func (g *gen) declTypes() string {
	if len(g.queue) == 0 && len(g.aliases) == 0 {
		return ""
	}
	type enumBind struct {
		name string
		e    *enumType
	}
	var enums []enumBind
	var b strings.Builder
	b.WriteString("type (\n")
	for len(g.queue) > 0 {
		t := g.queue[0]
		g.queue = g.queue[1:]
		fmt.Fprintf(&b, "\t%s %s\n", g.names[t], g.underlying(t))
		if e, ok := t.(*enumType); ok {
			enums = append(enums, enumBind{g.names[t], e})
		}
	}
	for _, a := range g.aliases {
		fmt.Fprintf(&b, "\t%s\n", a)
	}
	g.aliases = nil
	b.WriteString(")\n\n")
	for _, e := range enums {
		if len(e.e.names) == 0 {
			continue
		}
		b.WriteString("const (\n")
		for i, n := range e.e.names {
			if i == 0 {
				fmt.Fprintf(&b, "\t%s %s = iota\n", g.enumGo(e.e, n), e.name)
				continue
			}
			fmt.Fprintf(&b, "\t%s\n", g.enumGo(e.e, n))
		}
		b.WriteString(")\n\n")
	}
	return b.String()
}

func (g *gen) underlying(t Type) string {
	switch t := t.(type) {
	case *enumType:
		return "int64"
	case *arrayType:
		n := 0
		if lo, hi, ok := ordinalSpan(t.index); !ok {
			g.errf("array bounds are not static")
		} else if size, fit := fitLen(lo, hi); !fit {
			g.errf("array size too large")
		} else {
			n = size
		}
		return fmt.Sprintf("[%d]%s", n, g.goType(t.elem))
	case *recordType:
		if len(t.fields) == 0 {
			return "struct{}"
		}
		var b strings.Builder
		b.WriteString("struct {\n")
		for _, f := range t.fields {
			fmt.Fprintf(&b, "%s %s\n", g.goName(f.name), g.goType(f.typ))
		}
		b.WriteString("}")
		return b.String()
	case *setType:
		g.use(runtime.PartBase)
		return "rtBits"
	case *fileType:
		g.use(runtime.PartFile)
		return fmt.Sprintf("struct{ rtFile[%s] }", g.goType(t.elem))
	case *pointerType:
		if t.base == nil {
			g.errf("unresolved pointer")
			return "*byte"
		}
		return "*" + g.goType(t.base)
	default:
		return g.basicGo(t)
	}
}

func (g *gen) goType(t Type) string {
	if t == nil || isInvalid(t) {
		return "int"
	}
	if n, ok := stringLen(t); ok {
		g.strs[n] = true
		return fmt.Sprintf("str%d", n)
	}
	if name, ok := g.names[t]; ok {
		return name
	}
	switch t.(type) {
	case *enumType, *arrayType, *recordType, *setType, *fileType, *pointerType:
		if _, ok := t.(*setType); ok {
			g.use(runtime.PartBase)
		}
		if _, ok := t.(*fileType); ok {
			g.use(runtime.PartFile)
		}
		name := fmt.Sprintf("_T%d", g.tn)
		g.tn++
		name = g.unique(name)
		g.names[t] = name
		g.queue = append(g.queue, t)
		return name
	case *subrangeType:
		return g.goType(host(t))
	default:
		return g.basicGo(t)
	}
}

func (g *gen) basicGo(t Type) string {
	switch host(t) {
	case integerType:
		return "int"
	case realType:
		return "float64"
	case booleanType:
		return "bool"
	case charType:
		return "uint8"
	}
	if _, ok := t.(textType); ok {
		g.use(runtime.PartText)
		return "rtText"
	}
	return "int"
}

// unique keeps two Pascal types that share a spelling from declaring the
// same Go name. Nested routines can reuse a type name from an outer scope.
func (g *gen) unique(name string) string {
	if g.taken == nil {
		g.taken = map[string]bool{}
	}
	n := name
	for g.taken[n] {
		n += "_"
	}
	g.taken[n] = true
	return n
}

type enumKey struct {
	e *enumType
	n string
}

func enumMember(e *enumType, name string) bool {
	for _, n := range e.names {
		if n == name {
			return true
		}
	}
	return false
}

// enumGo is the Go name of an enumeration constant. Two enumerations can
// reuse a spelling, so the name is chosen once per enumeration.
func (g *gen) enumGo(e *enumType, name string) string {
	g.noteEnum(e)
	if g.enumName == nil {
		g.enumName = map[enumKey]string{}
	}
	k := enumKey{e, name}
	if s, ok := g.enumName[k]; ok {
		return s
	}
	s := g.unique(g.goName(name))
	g.enumName[k] = s
	return s
}

// noteEnum queues an enumeration that is not a named type, such as the base
// of set of (heart, diamond, spade, club), so its constants are declared.
func (g *gen) noteEnum(e *enumType) {
	if e == nil {
		return
	}
	if g.names == nil {
		g.names = map[Type]string{}
	}
	if _, ok := g.names[e]; ok {
		return
	}
	g.names[e] = g.unique("_enum")
	g.queue = append(g.queue, e)
}

func (g *gen) goName(s string) string {
	n := s
	for reserved[n] {
		n += "_"
	}
	return n
}

var reserved = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
	"any": true, "bool": true, "byte": true, "comparable": true,
	"complex64": true, "complex128": true, "error": true, "false": true,
	"float32": true, "float64": true, "int": true, "int8": true, "int16": true,
	"int32": true, "int64": true, "rune": true, "string": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
	"uintptr": true, "true": true,
	"append": true, "cap": true, "clear": true, "close": true, "complex": true,
	"copy": true, "delete": true, "imag": true, "len": true, "make": true,
	"max": true, "min": true, "new": true, "panic": true, "print": true,
	"println": true, "real": true, "recover": true, "nil": true, "iota": true,
	"rt": true,
}
