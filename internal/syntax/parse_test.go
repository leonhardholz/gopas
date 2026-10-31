package syntax

import (
	"reflect"
	"testing"

	"gopas/internal/ast"
)

func TestEmptyProgram(t *testing.T) {
	got, err := ParseFile("../../testdata/ok/empty.pas")
	if err != nil {
		t.Fatal(err)
	}
	prog, ok := got.(*ast.Program)
	if !ok {
		t.Fatalf("got %T, want *ast.Program", got)
	}
	if prog.Name != "empty" || !reflect.DeepEqual(prog.Files, []string{"output"}) {
		t.Fatalf("got program %s(%v), want empty(output)", prog.Name, prog.Files)
	}
	if prog.Vars != nil || len(prog.Body) != 0 {
		t.Fatalf("got vars %#v body %#v, want empty", prog.Vars, prog.Body)
	}
}

func TestHelloProgram(t *testing.T) {
	got, err := ParseFile("../../testdata/ok/hello.pas")
	if err != nil {
		t.Fatal(err)
	}
	prog, ok := got.(*ast.Program)
	if !ok {
		t.Fatalf("got %T, want *ast.Program", got)
	}
	if prog.Name != "hello" || !reflect.DeepEqual(prog.Files, []string{"output"}) {
		t.Fatalf("got program %s(%v), want hello(output)", prog.Name, prog.Files)
	}
	write, ok := onlyStmt(t, prog).(*ast.Write)
	if !ok || !write.Ln || len(write.Args) != 1 {
		t.Fatalf("got %#v, want writeln('Hello')", prog.Body[0])
	}
	arg, ok := write.Args[0].Value.(*ast.Str)
	if !ok || arg.Value != "Hello" || write.Args[0].Width != nil {
		t.Fatalf("got argument %#v, want %q", write.Args[0], "Hello")
	}
}

func TestPrimitiveVariables(t *testing.T) {
	prog := parseFile(t, "../../testdata/ok/vars.pas")
	if prog.Name != "count" || !reflect.DeepEqual(prog.Files, []string{"output"}) {
		t.Fatalf("got program %s(%v), want count(output)", prog.Name, prog.Files)
	}
	wantVars := []*ast.Decl{
		{Names: []string{"n"}, Type: typeName("integer")},
		{Names: []string{"x", "y"}, Type: typeName("real")},
		{Names: []string{"c"}, Type: typeName("char")},
		{Names: []string{"b"}, Type: typeName("boolean")},
	}
	if !reflect.DeepEqual(prog.Vars, wantVars) {
		t.Fatalf("got vars %#v", prog.Vars)
	}
	assign, ok := onlyStmt(t, prog).(*ast.Assign)
	if !ok {
		t.Fatalf("got body %T, want *ast.Assign", prog.Body[0])
	}
	if !reflect.DeepEqual(assign.Left, &ast.Name{Ident: "n"}) || !reflect.DeepEqual(assign.Value, &ast.Int{Text: "1"}) {
		t.Fatalf("got %#v := %#v", assign.Left, assign.Value)
	}
}

func TestStatementSequence(t *testing.T) {
	prog := parseFile(t, "../../testdata/ok/seq.pas")
	if len(prog.Body) != 3 {
		t.Fatalf("got %d statements, want 3", len(prog.Body))
	}
	first, ok := prog.Body[0].(*ast.Assign)
	if !ok || !reflect.DeepEqual(first.Left, &ast.Name{Ident: "n"}) || !reflect.DeepEqual(first.Value, &ast.Int{Text: "1"}) {
		t.Fatalf("first statement: %#v", prog.Body[0])
	}
	second, ok := prog.Body[1].(*ast.Assign)
	if !ok || !reflect.DeepEqual(second.Left, &ast.Name{Ident: "n"}) || !reflect.DeepEqual(second.Value, &ast.Name{Ident: "n"}) {
		t.Fatalf("second statement: %#v", prog.Body[1])
	}
	third, ok := prog.Body[2].(*ast.Write)
	if !ok || !third.Ln || len(third.Args) != 1 {
		t.Fatalf("third statement: %#v", prog.Body[2])
	}
	if arg, ok := third.Args[0].Value.(*ast.Str); !ok || arg.Value != "Hello" {
		t.Fatalf("third argument: %#v", third.Args[0])
	}

	trailing := parseSource(t, "program p(output);\nbegin\n ;\nend.\n")
	if len(trailing.Body) != 0 {
		t.Fatalf("trailing empty statements: %#v", trailing.Body)
	}
}

func TestAssignmentValues(t *testing.T) {
	tests := []struct {
		rhs  string
		want ast.Expr
	}{
		{"1", &ast.Int{Text: "1"}},
		{"1.5", &ast.Real{Text: "1.5"}},
		{"5E-3", &ast.Real{Text: "5E-3"}},
		{"'a'", &ast.Str{Value: "a"}},
		{"true", &ast.Name{Ident: "true"}},
		{"m", &ast.Name{Ident: "m"}},
	}
	for _, tt := range tests {
		src := "program p(output);\nvar n: integer;\nbegin\n n := " + tt.rhs + "\nend.\n"
		prog := parseSource(t, src)
		assign, ok := onlyStmt(t, prog).(*ast.Assign)
		if !ok {
			t.Fatalf("%s: got body %T", tt.rhs, prog.Body[0])
		}
		if !reflect.DeepEqual(assign.Value, tt.want) {
			t.Fatalf("%s: got %#v", tt.rhs, assign.Value)
		}
	}
}

func TestControlFlow(t *testing.T) {
	n := &ast.Name{Ident: "n"}
	hello := &ast.Write{Ln: true, Args: []ast.WriteArg{{Value: &ast.Str{Value: "Hello"}}}}
	want := []ast.Stmt{
		&ast.If{
			Cond: &ast.Binary{Op: "<", Left: n, Right: &ast.Int{Text: "10"}},
			Then: &ast.Assign{Left: &ast.Name{Ident: "n"}, Value: &ast.Int{Text: "1"}},
			Else: hello,
		},
		&ast.While{
			Cond: &ast.Binary{Op: "<", Left: n, Right: &ast.Int{Text: "10"}},
			Body: &ast.Assign{Left: n, Value: &ast.Binary{Op: "+", Left: n, Right: &ast.Int{Text: "1"}}},
		},
		&ast.Repeat{
			Body: []ast.Stmt{
				&ast.Assign{Left: &ast.Name{Ident: "n"}, Value: &ast.Int{Text: "1"}},
				hello,
			},
			Cond: &ast.Binary{Op: "=", Left: n, Right: &ast.Int{Text: "1"}},
		},
		&ast.For{
			Name: "n",
			From: &ast.Int{Text: "1"},
			To:   &ast.Int{Text: "10"},
			Body: hello,
		},
		&ast.For{
			Name: "n",
			From: &ast.Int{Text: "10"},
			Down: true,
			To:   &ast.Int{Text: "1"},
			Body: &ast.Compound{Body: []ast.Stmt{
				&ast.Assign{Left: &ast.Name{Ident: "m"}, Value: n},
			}},
		},
		&ast.Case{
			Expr: n,
			Arms: []*ast.Arm{
				{Labels: []ast.Expr{&ast.Int{Text: "1"}}, Stmt: hello},
				{Labels: []ast.Expr{&ast.Int{Text: "2"}, &ast.Int{Text: "3"}}, Stmt: &ast.Assign{Left: &ast.Name{Ident: "n"}, Value: &ast.Int{Text: "1"}}},
			},
		},
	}
	prog := parseFile(t, "../../testdata/ok/flow.pas")
	if !reflect.DeepEqual(prog.Body, want) {
		t.Fatalf("got %#v", prog.Body)
	}
}

func TestDanglingElse(t *testing.T) {
	prog := parseSource(t, "program p(output);\nvar n: integer;\nbegin\n if n < 10 then if n = 0 then n := 1 else n := 2\nend.\n")
	outer, ok := onlyStmt(t, prog).(*ast.If)
	if !ok || outer.Else != nil {
		t.Fatalf("got %#v", prog.Body)
	}
	inner, ok := outer.Then.(*ast.If)
	if !ok || inner.Else == nil {
		t.Fatalf("got then %#v", outer.Then)
	}
}

func TestSemicolonBeforeElse(t *testing.T) {
	src := "program p(output);\nvar n: integer;\nbegin\n if n < 10 then n := 1;\n else n := 2\nend.\n"
	if _, err := Parse("", []byte(src)); err == nil {
		t.Fatal("got parse of a semicolon before else")
	}
}

func TestProcedures(t *testing.T) {
	n := &ast.Name{Ident: "n"}
	prog := parseFile(t, "../../testdata/ok/procs.pas")
	want := []ast.Routine{
		&ast.Proc{
			Name: "reset",
			Body: []ast.Stmt{&ast.Assign{Left: &ast.Name{Ident: "n"}, Value: &ast.Int{Text: "0"}}},
		},
		&ast.Proc{
			Name: "add",
			Params: []ast.Formal{
				&ast.Param{Names: []string{"x"}, Type: "integer"},
				&ast.Param{Names: []string{"y"}, ByRef: true, Type: "integer"},
			},
			Vars: []*ast.Decl{{Names: []string{"t"}, Type: typeName("integer")}},
			Routines: []ast.Routine{&ast.Proc{
				Name:   "touch",
				Params: []ast.Formal{&ast.Param{Names: []string{"z"}, ByRef: true, Type: "integer"}},
				Body:   []ast.Stmt{&ast.Assign{Left: &ast.Name{Ident: "z"}, Value: &ast.Int{Text: "1"}}},
			}},
			Body: []ast.Stmt{
				&ast.Call{Name: "touch", Args: []ast.Expr{&ast.Name{Ident: "y"}}},
				&ast.Assign{Left: &ast.Name{Ident: "t"}, Value: &ast.Name{Ident: "x"}},
			},
		},
	}
	if !reflect.DeepEqual(prog.Routines, want) {
		t.Fatalf("routines: %#v", prog.Routines)
	}
	wantBody := []ast.Stmt{
		&ast.Call{Name: "reset"},
		&ast.Call{Name: "add", Args: []ast.Expr{&ast.Int{Text: "1"}, n}},
		&ast.Write{Ln: true, Args: []ast.WriteArg{{Value: &ast.Str{Value: "Hello"}}}},
	}
	if !reflect.DeepEqual(prog.Body, wantBody) {
		t.Fatalf("body: %#v", prog.Body)
	}
}

func TestFunctions(t *testing.T) {
	prog := parseFile(t, "../../testdata/ok/funcs.pas")
	want := []ast.Routine{
		&ast.Func{
			Name:   "next",
			Params: []ast.Formal{&ast.Param{Names: []string{"x"}, Type: "integer"}},
			Result: "integer",
			Body:   []ast.Stmt{&ast.Assign{Left: &ast.Name{Ident: "next"}, Value: &ast.Name{Ident: "x"}}},
		},
		&ast.Func{
			Name:   "current",
			Result: "integer",
			Body:   []ast.Stmt{&ast.Assign{Left: &ast.Name{Ident: "current"}, Value: &ast.Name{Ident: "n"}}},
		},
	}
	if !reflect.DeepEqual(prog.Routines, want) {
		t.Fatalf("routines: %#v", prog.Routines)
	}
	wantBody := []ast.Stmt{
		&ast.Assign{Left: &ast.Name{Ident: "n"}, Value: &ast.CallExpr{Name: "next", Args: []ast.Expr{&ast.Name{Ident: "current"}}}},
		&ast.Assign{Left: &ast.Name{Ident: "n"}, Value: &ast.CallExpr{Name: "next", Args: []ast.Expr{&ast.Int{Text: "1"}}}},
	}
	if !reflect.DeepEqual(prog.Body, wantBody) {
		t.Fatalf("body: %#v", prog.Body)
	}
}

func TestEmptyParameterList(t *testing.T) {
	src := "program p(output);\nprocedure q();\nbegin\nend;\nbegin\nend.\n"
	if _, err := Parse("", []byte(src)); err == nil {
		t.Fatal("got parse of an empty parameter list")
	}
}

func TestTypeDefinitions(t *testing.T) {
	prog := parseFile(t, "../../testdata/ok/types.pas")
	wantTypes := []*ast.TypeDef{
		{Name: "color", Type: &ast.Enum{Names: []string{"red", "green", "blue"}}},
		{Name: "index", Type: &ast.Subrange{Low: &ast.Int{Text: "1"}, High: &ast.Int{Text: "10"}}},
		{Name: "letter", Type: &ast.Subrange{Low: &ast.Str{Value: "a"}, High: &ast.Str{Value: "z"}}},
		{Name: "number", Type: typeName("integer")},
	}
	if !reflect.DeepEqual(prog.Types, wantTypes) {
		t.Fatalf("types: %#v", prog.Types)
	}
	wantVars := []*ast.Decl{
		{Names: []string{"c"}, Type: typeName("color")},
		{Names: []string{"i"}, Type: typeName("index")},
		{Names: []string{"ch"}, Type: typeName("letter")},
		{Names: []string{"n"}, Type: typeName("number")},
		{Names: []string{"k"}, Type: &ast.Subrange{
			Low:  &ast.Unary{Op: "-", X: &ast.Int{Text: "1"}},
			High: &ast.Int{Text: "1"},
		}},
	}
	if !reflect.DeepEqual(prog.Vars, wantVars) {
		t.Fatalf("vars: %#v", prog.Vars)
	}
	if len(prog.Routines) != 1 {
		t.Fatalf("routines: %#v", prog.Routines)
	}
	paint, ok := prog.Routines[0].(*ast.Proc)
	if !ok || paint.Name != "paint" {
		t.Fatalf("routines: %#v", prog.Routines)
	}
	wantLocal := []*ast.TypeDef{{Name: "shade", Type: typeName("color")}}
	if !reflect.DeepEqual(paint.Types, wantLocal) {
		t.Fatalf("local types: %#v", paint.Types)
	}
	if paint.Params[0].(*ast.Param).Type != "color" {
		t.Fatalf("parameter type %q", paint.Params[0].(*ast.Param).Type)
	}
}

func TestArrays(t *testing.T) {
	n := &ast.Name{Ident: "n"}
	r1 := &ast.Indexed{Base: &ast.Name{Ident: "r"}, Indexes: []ast.Expr{&ast.Int{Text: "1"}}}
	rn := &ast.Indexed{Base: &ast.Name{Ident: "r"}, Indexes: []ast.Expr{n}}
	prog := parseFile(t, "../../testdata/ok/arrays.pas")
	row := &ast.Array{Index: []ast.Type{typeName("index")}, Elem: typeName("integer")}
	wantTypes := []*ast.TypeDef{
		{Name: "index", Type: &ast.Subrange{Low: &ast.Int{Text: "1"}, High: &ast.Int{Text: "10"}}},
		{Name: "row", Type: row},
		{Name: "matrix", Type: &ast.Array{
			Index: []ast.Type{
				&ast.Subrange{Low: &ast.Int{Text: "1"}, High: &ast.Int{Text: "10"}},
				&ast.Subrange{Low: &ast.Int{Text: "1"}, High: &ast.Int{Text: "10"}},
			},
			Elem: typeName("integer"),
		}},
		{Name: "name", Type: &ast.Array{
			Packed: true,
			Index:  []ast.Type{&ast.Subrange{Low: &ast.Int{Text: "1"}, High: &ast.Int{Text: "8"}}},
			Elem:   typeName("char"),
		}},
	}
	if !reflect.DeepEqual(prog.Types, wantTypes) {
		t.Fatalf("types: %#v", prog.Types)
	}
	grid := &ast.Array{
		Index: []ast.Type{&ast.Subrange{Low: &ast.Int{Text: "1"}, High: &ast.Int{Text: "2"}}},
		Elem: &ast.Array{
			Index: []ast.Type{&ast.Subrange{Low: &ast.Int{Text: "1"}, High: &ast.Int{Text: "3"}}},
			Elem:  typeName("integer"),
		},
	}
	if !reflect.DeepEqual(prog.Vars[3].Type, grid) {
		t.Fatalf("grid: %#v", prog.Vars[3].Type)
	}
	wantBody := []ast.Stmt{
		&ast.Assign{Left: r1, Value: &ast.Int{Text: "1"}},
		&ast.Assign{Left: n, Value: r1},
		&ast.Assign{Left: rn, Value: &ast.Int{Text: "1"}},
		&ast.Assign{Left: &ast.Indexed{Base: &ast.Name{Ident: "m"}, Indexes: []ast.Expr{&ast.Int{Text: "1"}, n}}, Value: rn},
		&ast.Assign{Left: n, Value: &ast.Indexed{
			Base:    &ast.Indexed{Base: &ast.Name{Ident: "grid"}, Indexes: []ast.Expr{&ast.Int{Text: "1"}}},
			Indexes: []ast.Expr{&ast.Int{Text: "2"}},
		}},
		&ast.Assign{Left: &ast.Indexed{Base: &ast.Name{Ident: "s"}, Indexes: []ast.Expr{&ast.Int{Text: "1"}}}, Value: &ast.Str{Value: "a"}},
	}
	if !reflect.DeepEqual(prog.Body, wantBody) {
		t.Fatalf("body: %#v", prog.Body)
	}
}

func TestSets(t *testing.T) {
	h := &ast.Name{Ident: "h"}
	g := &ast.Name{Ident: "g"}
	red := &ast.Name{Ident: "red"}
	prog := parseFile(t, "../../testdata/ok/sets.pas")
	wantTypes := []*ast.TypeDef{
		{Name: "color", Type: &ast.Enum{Names: []string{"red", "green", "blue"}}},
		{Name: "hue", Type: &ast.Set{Base: typeName("color")}},
		{Name: "small", Type: &ast.Set{Base: &ast.Subrange{Low: &ast.Int{Text: "1"}, High: &ast.Int{Text: "10"}}}},
		{Name: "bits", Type: &ast.Set{Packed: true, Base: &ast.Subrange{Low: &ast.Int{Text: "0"}, High: &ast.Int{Text: "7"}}}},
	}
	if !reflect.DeepEqual(prog.Types, wantTypes) {
		t.Fatalf("types: %#v", prog.Types)
	}
	wantBody := []ast.Stmt{
		&ast.Assign{Left: h, Value: &ast.SetLit{}},
		&ast.Assign{Left: h, Value: &ast.SetLit{Elems: []*ast.SetElem{
			{Low: red},
			{Low: &ast.Name{Ident: "green"}},
		}}},
		&ast.Assign{Left: h, Value: &ast.SetLit{Elems: []*ast.SetElem{
			{Low: red, High: &ast.Name{Ident: "blue"}},
		}}},
		&ast.Assign{Left: h, Value: &ast.Binary{
			Op:    "+",
			Left:  h,
			Right: &ast.Binary{Op: "*", Left: g, Right: h},
		}},
		&ast.Assign{Left: h, Value: &ast.Binary{Op: "-", Left: h, Right: g}},
		&ast.Assign{Left: &ast.Name{Ident: "b"}, Value: &ast.Binary{
			Op:    "in",
			Left:  red,
			Right: &ast.Binary{Op: "+", Left: h, Right: g},
		}},
		&ast.Assign{Left: &ast.Name{Ident: "s"}, Value: &ast.SetLit{Elems: []*ast.SetElem{
			{Low: &ast.Int{Text: "1"}, High: &ast.Int{Text: "10"}},
		}}},
	}
	if !reflect.DeepEqual(prog.Body, wantBody) {
		t.Fatalf("body: %#v", prog.Body)
	}
}

func TestOperators(t *testing.T) {
	i := &ast.Name{Ident: "i"}
	j := &ast.Name{Ident: "j"}
	k := &ast.Name{Ident: "k"}
	x := &ast.Name{Ident: "x"}
	p := &ast.Name{Ident: "p"}
	q := &ast.Name{Ident: "q"}
	prog := parseFile(t, "../../testdata/ok/ops.pas")
	nilVal := &ast.Nil{}
	notP := &ast.Unary{Op: "not", X: p}
	one := &ast.Int{Text: "1"}
	want := []ast.Stmt{
		&ast.Assign{Left: j, Value: one},
		&ast.Assign{Left: k, Value: one},
		&ast.Assign{Left: &ast.Name{Ident: "y"}, Value: &ast.Real{Text: "1.0"}},
		&ast.Assign{Left: i, Value: &ast.Binary{
			Op:    "mod",
			Left:  &ast.Binary{Op: "div", Left: i, Right: j},
			Right: k,
		}},
		&ast.Assign{Left: i, Value: &ast.Binary{
			Op:    "-",
			Left:  &ast.Binary{Op: "+", Left: i, Right: j},
			Right: k,
		}},
		&ast.Assign{Left: p, Value: &ast.Binary{Op: "or", Left: p, Right: q}},
		&ast.Assign{Left: x, Value: &ast.Binary{
			Op:    "*",
			Left:  &ast.Binary{Op: "/", Left: x, Right: &ast.Name{Ident: "y"}},
			Right: x,
		}},
		&ast.Assign{Left: p, Value: &ast.Binary{
			Op:    "or",
			Left:  &ast.Binary{Op: "and", Left: p, Right: q},
			Right: notP,
		}},
		&ast.Assign{Left: p, Value: &ast.Binary{Op: "and", Left: notP, Right: q}},
		&ast.Assign{Left: p, Value: &ast.Unary{Op: "not", X: &ast.Unary{Op: "not", X: q}}},
		&ast.Assign{Left: p, Value: &ast.Binary{Op: "=", Left: &ast.Name{Ident: "a"}, Right: nilVal}},
	}
	if !reflect.DeepEqual(prog.Body, want) {
		t.Fatalf("body: %#v", prog.Body)
	}
	mixed := parseSource(t, "program p(output);\nvar i: integer;\nbegin\n i := i + j or k\nend.\n")
	or, ok := mixed.Body[0].(*ast.Assign).Value.(*ast.Binary)
	if !ok || or.Op != "or" || or.Left.(*ast.Binary).Op != "+" {
		t.Fatalf("or precedence: %#v", mixed.Body)
	}
}

func TestRecords(t *testing.T) {
	date := &ast.Record{Fields: &ast.FieldList{Fixed: []*ast.Field{
		{Names: []string{"day"}, Type: &ast.Subrange{Low: &ast.Int{Text: "1"}, High: &ast.Int{Text: "31"}}},
		{Names: []string{"month"}, Type: &ast.Subrange{Low: &ast.Int{Text: "1"}, High: &ast.Int{Text: "12"}}},
		{Names: []string{"year"}, Type: typeName("integer")},
	}}}
	person := &ast.Record{Fields: &ast.FieldList{
		Fixed: []*ast.Field{
			{Names: []string{"name"}, Type: &ast.Array{
				Packed: true,
				Index:  []ast.Type{&ast.Subrange{Low: &ast.Int{Text: "1"}, High: &ast.Int{Text: "8"}}},
				Elem:   typeName("char"),
			}},
			{Names: []string{"age"}, Type: typeName("integer")},
		},
		Variant: &ast.Variant{
			Tag:  "married",
			Type: "boolean",
			Arms: []*ast.VariantArm{
				{Labels: []ast.Expr{&ast.Name{Ident: "true"}}, Fields: &ast.FieldList{Fixed: []*ast.Field{
					{Names: []string{"spouse"}, Type: typeName("integer")},
				}}},
				{Labels: []ast.Expr{&ast.Name{Ident: "false"}}},
			},
		},
	}}
	shape := &ast.Record{Fields: &ast.FieldList{Variant: &ast.Variant{
		Type: "color",
		Arms: []*ast.VariantArm{
			{Labels: []ast.Expr{&ast.Name{Ident: "red"}}, Fields: &ast.FieldList{Fixed: []*ast.Field{
				{Names: []string{"x"}, Type: typeName("integer")},
			}}},
			{Labels: []ast.Expr{&ast.Name{Ident: "blue"}, &ast.Name{Ident: "green"}}},
		},
	}}}
	prog := parseFile(t, "../../testdata/ok/records.pas")
	wantTypes := []*ast.TypeDef{
		{Name: "color", Type: &ast.Enum{Names: []string{"red", "green", "blue"}}},
		{Name: "empty", Type: &ast.Record{}},
		{Name: "date", Type: date},
		{Name: "person", Type: person},
		{Name: "shape", Type: shape},
	}
	if !reflect.DeepEqual(prog.Types, wantTypes) {
		t.Fatalf("types: %#v", prog.Types)
	}
	d := &ast.Name{Ident: "d"}
	wantBody := []ast.Stmt{
		&ast.Assign{Left: &ast.Select{Base: d, Field: "year"}, Value: &ast.Int{Text: "2000"}},
		&ast.Assign{Left: &ast.Indexed{
			Base:    &ast.Select{Base: &ast.Name{Ident: "p"}, Field: "name"},
			Indexes: []ast.Expr{&ast.Int{Text: "1"}},
		}, Value: &ast.Str{Value: "a"}},
		&ast.Assign{Left: &ast.Select{
			Base:  &ast.Indexed{Base: &ast.Name{Ident: "days"}, Indexes: []ast.Expr{&ast.Int{Text: "1"}}},
			Field: "year",
		}, Value: &ast.Int{Text: "1"}},
		&ast.With{
			Recs: []ast.Expr{d},
			Body: &ast.Assign{Left: &ast.Name{Ident: "year"}, Value: &ast.Name{Ident: "month"}},
		},
	}
	if !reflect.DeepEqual(prog.Body, wantBody) {
		t.Fatalf("body: %#v", prog.Body)
	}
}

func TestParameterNeedsTypeName(t *testing.T) {
	src := "program p(output);\nprocedure q(x: 1..10);\nbegin\nend;\nbegin\nend.\n"
	if _, err := Parse("", []byte(src)); err == nil {
		t.Fatal("got parse of a subrange parameter")
	}
}

func typeName(name string) ast.Type {
	return &ast.TypeName{Name: name}
}

func TestConstants(t *testing.T) {
	prog := parseFile(t, "../../testdata/ok/consts.pas")
	want := []*ast.ConstDef{
		{Name: "ten", Value: &ast.Int{Text: "10"}},
		{Name: "neg", Value: &ast.Unary{Op: "-", X: &ast.Int{Text: "1"}}},
		{Name: "letter", Value: &ast.Str{Value: "a"}},
		{Name: "also", Value: &ast.Name{Ident: "ten"}},
	}
	if !reflect.DeepEqual(prog.Consts, want) {
		t.Fatalf("consts: %#v", prog.Consts)
	}
	reject(t, "program p(output);\ntype t = integer;\nconst n = 1;\nbegin end.")
}

func TestLabels(t *testing.T) {
	n := &ast.Name{Ident: "n"}
	prog := parseFile(t, "../../testdata/ok/labels.pas")
	if !reflect.DeepEqual(prog.Labels, []string{"1", "20"}) {
		t.Fatalf("labels: %#v", prog.Labels)
	}
	want := []ast.Stmt{
		&ast.Labeled{Label: "1", Stmt: &ast.Assign{Left: n, Value: &ast.Int{Text: "1"}}},
		&ast.Goto{Label: "20"},
		&ast.Labeled{Label: "20"},
	}
	if !reflect.DeepEqual(prog.Body, want) {
		t.Fatalf("body: %#v", prog.Body)
	}
	reject(t, "program p(output);\nlabel 10000;\nbegin end.")
}

func TestPointers(t *testing.T) {
	node := &ast.Record{Fields: &ast.FieldList{Fixed: []*ast.Field{
		{Names: []string{"value"}, Type: typeName("integer")},
		{Names: []string{"next"}, Type: &ast.Pointer{To: "node"}},
	}}}
	prog := parseFile(t, "../../testdata/ok/pointers.pas")
	wantTypes := []*ast.TypeDef{
		{Name: "node", Type: node},
		{Name: "link", Type: &ast.Pointer{To: "node"}},
	}
	if !reflect.DeepEqual(prog.Types, wantTypes) {
		t.Fatalf("types: %#v", prog.Types)
	}
	p := &ast.Name{Ident: "p"}
	value := &ast.Select{Base: &ast.Deref{Base: p}, Field: "value"}
	next := &ast.Select{Base: &ast.Deref{Base: p}, Field: "next"}
	wantBody := []ast.Stmt{
		&ast.Assign{Left: p, Value: &ast.Nil{}},
		&ast.Call{Name: "new", Args: []ast.Expr{p}},
		&ast.Assign{Left: value, Value: &ast.Int{Text: "1"}},
		&ast.Assign{Left: next, Value: &ast.Nil{}},
	}
	if !reflect.DeepEqual(prog.Body, wantBody) {
		t.Fatalf("body: %#v", prog.Body)
	}
	up := parseSource(t, "program p(output);\nvar q: ↑integer;\nbegin\n q↑ := 1\nend.\n")
	if _, ok := up.Vars[0].Type.(*ast.Pointer); !ok {
		t.Fatalf("up-arrow: %#v", up.Vars[0].Type)
	}
	if _, ok := up.Body[0].(*ast.Assign).Left.(*ast.Deref); !ok {
		t.Fatalf("deref: %#v", up.Body[0])
	}
	at := parseSource(t, "program p(output);\nvar q: @integer;\nbegin\n q@ := 1\nend.\n")
	if _, ok := at.Vars[0].Type.(*ast.Pointer); !ok {
		t.Fatalf("at: %#v", at.Vars[0].Type)
	}
}

func TestFiles(t *testing.T) {
	prog := parseFile(t, "../../testdata/ok/files.pas")
	wantTypes := []*ast.TypeDef{
		{Name: "numfile", Type: &ast.File{Elem: typeName("integer")}},
		{Name: "textpack", Type: &ast.File{Packed: true, Elem: typeName("char")}},
	}
	if !reflect.DeepEqual(prog.Types, wantTypes) {
		t.Fatalf("types: %#v", prog.Types)
	}
	f := &ast.Name{Ident: "f"}
	g := &ast.Name{Ident: "g"}
	wantBody := []ast.Stmt{
		&ast.Assign{Left: &ast.Deref{Base: f}, Value: &ast.Int{Text: "1"}},
		&ast.Assign{
			Left:  &ast.Indexed{Base: &ast.Deref{Base: g}, Indexes: []ast.Expr{&ast.Int{Text: "1"}}},
			Value: &ast.Deref{Base: f},
		},
	}
	if !reflect.DeepEqual(prog.Body, wantBody) {
		t.Fatalf("body: %#v", prog.Body)
	}
}

func TestForward(t *testing.T) {
	x := &ast.Param{Names: []string{"x"}, Type: "integer"}
	n := &ast.Param{Names: []string{"n"}, Type: "integer"}
	prog := parseFile(t, "../../testdata/ok/forward.pas")
	want := []ast.Routine{
		&ast.Proc{Name: "p", Params: []ast.Formal{x}, Forward: true},
		&ast.Func{Name: "f", Params: []ast.Formal{x}, Result: "integer", Forward: true},
		&ast.Proc{Name: "apply", Params: []ast.Formal{
			&ast.ProcParam{Name: "q", Params: []ast.Formal{n}},
			&ast.FuncParam{Name: "g", Params: []ast.Formal{n}, Result: "integer"},
		}},
		&ast.Proc{Name: "p"},
		&ast.Func{Name: "f", Body: []ast.Stmt{
			&ast.Assign{Left: &ast.Name{Ident: "f"}, Value: &ast.Name{Ident: "x"}},
		}},
	}
	if !reflect.DeepEqual(prog.Routines, want) {
		t.Fatalf("routines: %#v", prog.Routines)
	}
	reject(t, "program p(output);\nfunction f; forward;\nbegin end.")
}

func TestComments(t *testing.T) {
	prog := parseFile(t, "../../testdata/ok/comments.pas")
	want := []*ast.ConstDef{{Name: "n", Value: &ast.Int{Text: "10"}}}
	if !reflect.DeepEqual(prog.Consts, want) || len(prog.Body) != 0 {
		t.Fatalf("program: %#v", prog)
	}
	parseSource(t, "program p(output); (* hello }\nbegin end.")
	reject(t, "program p(output);\nbegin (*) end.")
}

func TestReadWrite(t *testing.T) {
	i := &ast.Name{Ident: "i"}
	f := &ast.Name{Ident: "f"}
	prog := parseFile(t, "../../testdata/ok/io.pas")
	x := &ast.Name{Ident: "x"}
	want := []ast.Stmt{
		&ast.Read{Vars: []ast.Expr{i}},
		&ast.Read{Ln: true, Vars: []ast.Expr{i}},
		&ast.Write{Args: []ast.WriteArg{{
			Value: x,
			Width: &ast.Int{Text: "1"},
			Frac:  &ast.Int{Text: "2"},
		}}},
		&ast.Write{Ln: true, Args: []ast.WriteArg{
			{Value: f},
			{Value: &ast.Binary{Op: "+", Left: i, Right: &ast.Int{Text: "1"}}, Width: &ast.Int{Text: "8"}},
		}},
		&ast.Call{Name: "writeln"},
	}
	if !reflect.DeepEqual(prog.Body, want) {
		t.Fatalf("body: %#v", prog.Body)
	}
	reject(t, "program p(output);\nvar i: integer;\nbegin\n read(1)\nend.")
	reject(t, "program p(output);\nbegin\n writeln()\nend.")
}

func TestLevel0Syntax(t *testing.T) {
	prog := parseSource(t, "program P;\nbegin end.\n")
	if prog.Name != "p" || prog.Files != nil {
		t.Fatalf("heading: %s %v", prog.Name, prog.Files)
	}
	prog = parseSource(t, "program Copy(Input, Output);\nbegin end.\n")
	if prog.Name != "copy" || !reflect.DeepEqual(prog.Files, []string{"input", "output"}) {
		t.Fatalf("heading: %s %v", prog.Name, prog.Files)
	}
	prog = parseSource(t, "program p(output);\nvar N: Integer;\nbegin\n n := N\nend.\n")
	assign, ok := onlyStmt(t, prog).(*ast.Assign)
	if !ok || !reflect.DeepEqual(assign.Left, &ast.Name{Ident: "n"}) || !reflect.DeepEqual(assign.Value, &ast.Name{Ident: "n"}) {
		t.Fatalf("case fold: %#v", prog.Body)
	}
	if prog.Vars[0].Type.(*ast.TypeName).Name != "integer" {
		t.Fatalf("type: %#v", prog.Vars[0].Type)
	}

	prog = parseSource(t, `
program p(output);
var n: integer;
begin
  case n of
    -1, +2: n := 1;
    'a': n := 2;
  end
end.
`)
	want := &ast.Case{
		Expr: &ast.Name{Ident: "n"},
		Arms: []*ast.Arm{
			{Labels: []ast.Expr{
				&ast.Unary{Op: "-", X: &ast.Int{Text: "1"}},
				&ast.Unary{Op: "+", X: &ast.Int{Text: "2"}},
			}, Stmt: &ast.Assign{Left: &ast.Name{Ident: "n"}, Value: &ast.Int{Text: "1"}}},
			{Labels: []ast.Expr{&ast.Str{Value: "a"}}, Stmt: &ast.Assign{Left: &ast.Name{Ident: "n"}, Value: &ast.Int{Text: "2"}}},
		},
	}
	if !reflect.DeepEqual(onlyStmt(t, prog), want) {
		t.Fatalf("case: %#v", prog.Body[0])
	}

	prog = parseSource(t, `
program p(output);
type row = array (.1..2.) of integer;
var x: row; s: set of 1..2;
begin
  x(.1.) := 1;
  s := (.1, 2.)
end.
`)
	if _, ok := prog.Types[0].Type.(*ast.Array); !ok {
		t.Fatalf("array: %#v", prog.Types[0].Type)
	}
	if _, ok := prog.Body[0].(*ast.Assign).Left.(*ast.Indexed); !ok {
		t.Fatalf("index: %#v", prog.Body[0])
	}
	if _, ok := prog.Body[1].(*ast.Assign).Value.(*ast.SetLit); !ok {
		t.Fatalf("set: %#v", prog.Body[1])
	}

	reject(t, "program p();\nbegin end.")
	reject(t, "program p(output);\nvar n: integer;\nbegin\n case n of end\nend.")
	reject(t, "program p(output);\nvar n, a: integer;\nbegin\n case n of a[1]: n := 1 end\nend.")
	reject(t, "program p(output);\nvar n: integer;\nbegin\n case n of 1: n := 1;; end\nend.")
	reject(t, "program p(output);\ntype e = record ; end;\nbegin end.")
	reject(t, "program p(output);\ntype e = record case boolean of true: ();; end;\nbegin end.")
	for _, word := range []string{"and", "const", "div", "file", "goto", "label", "mod", "nil", "not", "or"} {
		reject(t, "program p(output);\nvar "+word+": integer;\nbegin end.")
	}
}

func TestConformantArray(t *testing.T) {
	prog := parseSource(t, "program p;\nprocedure q(var a, b: array [i..j: integer; k..l: color] of packed array [u..v: integer] of char);\nbegin end;\nbegin end.")
	got := prog.Routines[0].(*ast.Proc).Params[0].(*ast.ConfParam)
	want := &ast.ConfParam{
		Names: []string{"a", "b"},
		ByRef: true,
		Schema: &ast.ConfSchema{
			Indexes: []ast.ConfIndex{
				{Low: "i", High: "j", Type: "integer"},
				{Low: "k", High: "l", Type: "color"},
			},
			Nested: &ast.ConfSchema{
				Packed:  true,
				Indexes: []ast.ConfIndex{{Low: "u", High: "v", Type: "integer"}},
				Elem:    "char",
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("param: %#v", got)
	}
}

func reject(t *testing.T, src string) {
	t.Helper()
	if _, err := Parse("", []byte(src)); err == nil {
		t.Fatalf("accepted %q", src)
	}
}

func onlyStmt(t *testing.T, prog *ast.Program) ast.Stmt {
	t.Helper()
	if len(prog.Body) != 1 {
		t.Fatalf("got %d statements, want 1", len(prog.Body))
	}
	return prog.Body[0]
}

func parseFile(t *testing.T, path string) *ast.Program {
	t.Helper()
	got, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	prog, ok := got.(*ast.Program)
	if !ok {
		t.Fatalf("got %T, want *ast.Program", got)
	}
	return prog
}

func parseSource(t *testing.T, src string) *ast.Program {
	t.Helper()
	got, err := Parse("", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	prog, ok := got.(*ast.Program)
	if !ok {
		t.Fatalf("got %T, want *ast.Program", got)
	}
	return prog
}
