package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopas/internal/ast"
	"gopas/internal/syntax"
)

func TestOKPrograms(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "ok")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range entries {
		name := ent.Name()
		prog := parseFile(t, filepath.Join(dir, name))
		if errs := Check(prog); len(errs) > 0 {
			t.Errorf("%s: %v", name, errs)
		}
	}
}

func TestViolations(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"program p(output); begin n := 1 end.", "not declared"},
		{"program p(output); var n, n: integer; begin end.", "declared twice"},
		{"program p(output); var c: char; begin c := 1 end.", "type conflict"},
		{"program p(output); var i: integer; begin i := i or i end.", "boolean"},
		{"program p(output); procedure q; forward; begin end.", "not completed"},
		{"program p(output); label 1; begin goto 2 end.", "not declared"},
		{"program p(output); var f: file of integer; begin f := f end.", "type conflict"},
		{"program p(f); var f: integer; begin end.", "not a file"},
		{"program p(output); type a = (red, red); begin end.", "declared twice"},
		{"program p(output); type r = record case boolean of true: () end; begin end.", "cover"},
		{"program p(output); const s = 'abc'; begin if s[1] = 'a' then end.", "index requires a variable"},
		{"program p(output); const red = 1; procedure q; const m = red; var colour: (red, blue); begin end; begin end.", "used before its definition"},
	}
	for _, tt := range cases {
		errs := Check(parseSource(t, tt.src))
		if !has(errs, tt.want) {
			t.Errorf("%s: %v, want %q", tt.src, errs, tt.want)
		}
	}
}

func TestVariantSubrange(t *testing.T) {
	ok := `
program p(output);
type
  color = (red, green, blue);
  shade = green..blue;
  r = record
    case s: shade of
      green: ();
      blue: ()
  end;
begin
end.
`
	if errs := Check(parseSource(t, ok)); len(errs) > 0 {
		t.Fatal(errs)
	}
	missing := `
program p(output);
type
  color = (red, green, blue);
  shade = green..blue;
  r = record
    case s: shade of
      green: ()
  end;
begin
end.
`
	if errs := Check(parseSource(t, missing)); !has(errs, "cover") {
		t.Fatalf("%v, want a coverage error", errs)
	}
}

func TestPackUnpack(t *testing.T) {
	ok := `
program p;
type number = integer;
var
  a: array [1..10] of integer;
  b: array [1..10] of number;
  m: array [1..3] of array [1..10] of integer;
  z: packed array [0..3] of integer;
begin
  pack(a, 1, z);
  pack(b, 2, z);
  pack(m[1], 1, z);
  unpack(z, a, 1)
end.
`
	if errs := Check(parseSource(t, ok)); len(errs) > 0 {
		t.Fatal(errs)
	}
	cases := []struct {
		src  string
		want string
	}{
		{`program p; var a: array [1..4] of integer; z: packed array [1..2] of integer;
begin pack(z, 1, a) end.`, "unpacked"},
		{`program p; var a: array [1..4] of integer; z: packed array [1..2] of char;
begin pack(a, 1, z) end.`, "component"},
		{`program p; var a: array [1..4] of integer; z: packed array [1..2] of integer;
begin pack(a, 'x', z) end.`, "index"},
		{`program p; var a: array [1..4] of integer; z: packed array [1..2] of integer;
begin pack(a, 0, z) end.`, "out of range"},
		{`program p; var a: array [1..4] of integer; z: packed array [1..2] of integer;
begin pack(a, 4, z) end.`, "past the end"},
		{`program p; var a: array [1..4] of integer; z: packed array [1..2] of integer;
begin unpack(z, a, 4) end.`, "past the end"},
		{`program p; var a: array [1..4] of 1..10; z: packed array [1..2] of integer;
begin pack(a, 1, z) end.`, "component"},
		{`program p; var a: array [1..4] of integer; z: packed array [1..2] of integer;
begin unpack(a, z, 1) end.`, "a packed array"},
		{`program p; var z: packed array [1..2] of integer; begin pack(1, 1, z) end.`, "variable"},
	}
	for _, tt := range cases {
		errs := Check(parseSource(t, tt.src))
		if !has(errs, tt.want) {
			t.Errorf("%s: %v, want %q", tt.src, errs, tt.want)
		}
	}
}

func TestNewDispose(t *testing.T) {
	ok := `
program p;
type
  color = (red, green, blue);
  node = record
    case married: boolean of
      true: (case color of
        red: (x: integer);
        green, blue: ());
      false: ()
  end;
  link = ^node;
  plain = ^integer;
var q: link; n: plain;
begin
  new(q);
  new(n);
  new(q, true);
  new(q, true, red);
  new(q, true, green);
  new(q, false);
  dispose(q, true, blue);
  dispose(q, false);
  dispose(n)
end.
`
	if errs := Check(parseSource(t, ok)); len(errs) > 0 {
		t.Fatal(errs)
	}
	cases := []struct {
		src  string
		want string
	}{
		{`program p; type n = record case boolean of true: (); false: () end; var q: ^n;
begin new(q, 1) end.`, "tag type"},
		{`program p; type color = (red, green, blue); n = record case boolean of true: (case color of red, green, blue: ()); false: () end; var q: ^n;
begin new(q, true, true) end.`, "tag type"},
		{`program p; type n = record case boolean of true: (); false: () end; var q: ^n;
begin new(q, false, true) end.`, "too many"},
		{`program p; type n = record year: integer end; var q: ^n;
begin dispose(q, 1) end.`, "variant"},
		{`program p; type t = 1..3; n = record case t of 1: (); 2: (); 3: () end; var q: ^n;
begin new(q, 4) end.`, "out of range"},
	}
	for _, tt := range cases {
		errs := Check(parseSource(t, tt.src))
		if !has(errs, tt.want) {
			t.Errorf("%s: %v, want %q", tt.src, errs, tt.want)
		}
	}
}

func TestConformant(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`program p; var a: array [1..2] of char;
procedure q(x: array [i..j: integer] of integer); begin end;
begin q(a) end.`, "conformant"},
		{`program p; var a: array [boolean] of integer;
procedure q(x: array [i..j: integer] of integer); begin end;
begin q(a) end.`, "conformant"},
		{`program p; procedure q(var s: packed array [i..j: integer] of char); begin end;
begin q('ab') end.`, "variable"},
		{`program p; var a: array [1..2] of integer; b: array [1..3] of integer;
procedure q(x, y: array [i..j: integer] of integer); begin end;
begin q(a, b) end.`, "bounds differ"},
		{`program p; type small = 1..3; var a: array [1..5] of integer;
procedure q(x: array [i..j: small] of integer); begin end;
begin q(a) end.`, "out of range"},
		{`program p; procedure q(a: array [i..j: integer] of integer); begin i := 1 end; begin end.`, "requires a variable"},
		{`program p; procedure q(a: array [i..i: integer] of integer); begin end; begin end.`, "declared twice"},
		{`program p; type f = file of integer;
procedure q(a: array [i..j: integer] of f); begin end; begin end.`, "file"},
		{`program p; procedure q(a: array [integer..j: integer] of char); begin end; begin end.`, "not a type"},
		{`program p; procedure a(x: array [i..j: integer] of integer); begin end;
procedure b(x: array [m..n: integer] of char); begin end;
procedure apply(procedure r(x: array [u..v: integer] of integer)); begin end;
begin apply(a); apply(b) end.`, "procedural"},
	}
	for _, tt := range cases {
		errs := Check(parseSource(t, tt.src))
		if !has(errs, tt.want) {
			t.Errorf("%s: %v, want %q", tt.src, errs, tt.want)
		}
	}
}

func TestPointerAndForward(t *testing.T) {
	src := `
program p(output);
type link = ^node;
  node = record n: integer; next: link end;
var q: link;
procedure a(x: integer); forward;
procedure a;
begin a(1) end;
begin
  q := nil;
  new(q);
  q^.n := 1
end.
`
	if errs := Check(parseSource(t, src)); len(errs) > 0 {
		t.Fatal(errs)
	}
}

func TestBoundConstant(t *testing.T) {
	ok := `
program p(output);
var a: array [1..2] of char;
procedure p(var x: array [l..u: integer] of char);
const m = 1;
begin
  x[m] := 'a';
  x[l] := 'b'
end;
begin
end.
`
	if errs := Check(parseSource(t, ok)); len(errs) > 0 {
		t.Fatal(errs)
	}
	bad := `
program p(output);
var a: array [1..2] of char;
procedure p(var x: array [l..u: integer] of char);
const m = l;
begin
  x[l] := 'a'
end;
begin
end.
`
	if errs := Check(parseSource(t, bad)); !has(errs, "is not a constant") {
		t.Fatalf("%v", errs)
	}
}

func TestFunctionPointer(t *testing.T) {
	ok := `
program p(output);
type ptr = ^integer;
var p: ptr;
function f: ptr;
var q: ptr;
begin
  new(q);
  f := q;
  q^ := 10
end;
begin
end.
`
	if errs := Check(parseSource(t, ok)); len(errs) > 0 {
		t.Fatal(errs)
	}
	bad := `
program p(output);
type ptr = ^integer;
var p: ptr;
function f: ptr;
begin
  new(p);
  f := p;
  f^ := 10
end;
begin
end.
`
	if errs := Check(parseSource(t, bad)); !has(errs, "requires a variable") {
		t.Fatalf("%v", errs)
	}
}

func TestPointerEquality(t *testing.T) {
	ok := `
program p(output);
type one = ^integer;
var x, y: one;
begin
  if (x = y) or (x <> nil) or (nil = y) then
     writeln('ok')
end.
`
	if errs := Check(parseSource(t, ok)); len(errs) > 0 {
		t.Fatal(errs)
	}
	bad := `
program p(output);
type
   one = ^integer;
   two = ^integer;
var
   x: one;
   y: two;
begin
  if (x <> y) or (x = y) then
     writeln('no')
end.
`
	if errs := Check(parseSource(t, bad)); !has(errs, "type conflict") {
		t.Fatalf("%v", errs)
	}
}

func TestTruncRound(t *testing.T) {
	ok := `
program p(output);
var i: integer; x: real;
begin
  x := 1.5;
  i := trunc(x) + round(x)
end.
`
	if errs := Check(parseSource(t, ok)); len(errs) > 0 {
		t.Fatal(errs)
	}
	bad := `
program p(output);
var i: integer; x: real;
begin
  i := 1001;
  x := trunc(i) + round(i + 1)
end.
`
	if errs := Check(parseSource(t, bad)); !has(errs, "requires a real") {
		t.Fatalf("%v", errs)
	}
}

func TestConformantActuals(t *testing.T) {
	same := `
program p(output);
var x, y: array [1..2] of boolean;
procedure byValue(a, b: array [l..u: integer] of boolean);
begin
end;
procedure byVar(var a, b: array [l..u: integer] of boolean);
begin
end;
begin
  byValue(x, y);
  byVar(x, y)
end.
`
	if errs := Check(parseSource(t, same)); len(errs) > 0 {
		t.Fatal(errs)
	}
	distinct := `
program p(output);
var
   x: array [1..2] of boolean;
   y: array [1..2] of boolean;
procedure byValue(a, b: array [l..u: integer] of boolean);
begin
end;
procedure byVar(var a, b: array [l..u: integer] of boolean);
begin
end;
begin
  byValue(x, y);
  byVar(x, y)
end.
`
	errs := Check(parseSource(t, distinct))
	if !has(errs, "same type") {
		t.Fatalf("%v", errs)
	}
	passed := `
program p(output);
var x: array [boolean] of real;
function f(var a: array [l..u: boolean] of real): boolean;
begin
  f := a[l] = 1.0
end;
procedure r(y: real);
begin
end;
procedure p1(c: array [l..u: boolean] of real);
begin
  r(c[l]);
  if f(c) then
     r(c[u])
end;
begin
  p1(x)
end.
`
	if errs := Check(parseSource(t, passed)); len(errs) > 0 {
		t.Fatal(errs)
	}
	valueAgain := `
program p(output);
var x: array [boolean] of char;
function fact(c: array [l..u: boolean] of char; n: integer): integer;
begin
  fact := n * fact(c, n - 1)
end;
procedure q(b: array [l..u: boolean] of char);
begin
end;
procedure p1(var a: array [l..u: boolean] of char);
begin
  q(a)
end;
begin
  fact(x, 5);
  p1(x)
end.
`
	if errs := Check(parseSource(t, valueAgain)); !has(errs, "array type") {
		t.Fatalf("%v", errs)
	}
}

func TestParamSections(t *testing.T) {
	ok := `
program p(output);
var aa, bb: integer;
procedure p1(procedure formal(var a, b: integer));
begin
  formal(aa, bb)
end;
procedure same(var a, b: integer);
begin
end;
begin
  p1(same)
end.
`
	if errs := Check(parseSource(t, ok)); len(errs) > 0 {
		t.Fatal(errs)
	}
	bad := `
program p(output);
var aa, bb: integer;
procedure p1(procedure formal(var a, b: integer));
begin
  formal(aa, bb)
end;
procedure actual(var a: integer; var b: integer);
begin
end;
begin
  p1(actual)
end.
`
	if errs := Check(parseSource(t, bad)); !has(errs, "procedural parameter mismatch") {
		t.Fatalf("%v", errs)
	}
}

func TestParenVariable(t *testing.T) {
	ok := `
program p(output);
var x: integer;
procedure r(y: integer);
begin
end;
begin
  r((x));
  r((1 + 2))
end.
`
	if errs := Check(parseSource(t, ok)); len(errs) > 0 {
		t.Fatal(errs)
	}
	bad := `
program p(output);
var x: integer;
procedure p2(var y: integer);
begin
  y := y
end;
begin
  p2((x))
end.
`
	if errs := Check(parseSource(t, bad)); !has(errs, "must be a variable") {
		t.Fatalf("%v", errs)
	}
}

func TestPackedCharParam(t *testing.T) {
	readOK := `
program p(output);
var
  f: text;
  image: packed array [1..4] of char;
begin
  rewrite(f);
  reset(f);
  read(f, image[1])
end.
`
	if errs := Check(parseSource(t, readOK)); len(errs) > 0 {
		t.Fatal(errs)
	}
	bad := `
program p(output);
var
  image: packed array [1..4] of char;
function header(var col1: char): boolean;
begin
  header := col1 = 'H'
end;
begin
  if header(image[1]) then
end.
`
	if errs := Check(parseSource(t, bad)); !has(errs, "component of a packed variable") {
		t.Fatalf("%v", errs)
	}
}

func TestStringIdentity(t *testing.T) {
	ok := `
program p(output);
type
  a = packed array [1..4] of char;
  b = packed array [1..4] of char;
var
  x: a;
  y: b;
procedure q(s: a);
begin
end;
procedure r(var s: a);
begin
end;
begin
  x := y;
  x := 'abcd';
  q(y);
  q('abcd');
  r(x)
end.
`
	if errs := Check(parseSource(t, ok)); len(errs) > 0 {
		t.Fatal(errs)
	}
	bad := `
program p(output);
type
  a = packed array [1..4] of char;
  b = packed array [1..4] of char;
var
  y: b;
procedure r(var s: a);
begin
end;
begin
  r(y)
end.
`
	if errs := Check(parseSource(t, bad)); !has(errs, "variable parameter type mismatch") {
		t.Fatalf("%v", errs)
	}
}

func TestSetPackedRelation(t *testing.T) {
	ok := `
program p(output);
type
  plain = set of boolean;
  pack = packed set of false..true;
var
  b: plain;
  pset: pack;
  flag: boolean;
begin
  b := [true];
  pset := [true];
  flag := b >= [false];
  flag := pset = [true]
end.
`
	if errs := Check(parseSource(t, ok)); len(errs) > 0 {
		t.Fatal(errs)
	}
	bad := `
program p(output);
type
  plain = set of boolean;
  pack = packed set of false..true;
var
  b: plain;
  pset: pack;
  flag: boolean;
begin
  flag := b >= pset
end.
`
	if errs := Check(parseSource(t, bad)); !has(errs, "type conflict") {
		t.Fatalf("%v", errs)
	}
}

func TestParamScope(t *testing.T) {
	ok := `
program p(output);
type
  a = char;
  b = boolean;
procedure x2(c: a; d: b);
begin
end;
procedure y(a: b);
begin
end;
begin
end.
`
	if errs := Check(parseSource(t, ok)); len(errs) > 0 {
		t.Fatal(errs)
	}
	before := `
program p(output);
type
  a = char;
  b = boolean;
procedure x2(c: a; a: b);
begin
end;
begin
end.
`
	if errs := Check(parseSource(t, before)); !has(errs, "used before its definition") {
		t.Fatalf("%v", errs)
	}
	after := `
program p(output);
type
  a = char;
procedure x2(a: boolean; c: a);
begin
end;
begin
end.
`
	if errs := Check(parseSource(t, after)); !has(errs, "not a type") {
		t.Fatalf("%v", errs)
	}
}

func TestRecordFieldScope(t *testing.T) {
	ok := `
program p(output);
type
  rec = record
    ptr: ^fred
  end;
  fred = rec;
  node = record
    link: ^node
  end;
  outer = record
    ptr: ^item;
    inner: record
      item: integer
    end
  end;
  item = integer;
begin
end.
`
	if errs := Check(parseSource(t, ok)); len(errs) > 0 {
		t.Fatal(errs)
	}
	before := `
program p(output);
type
  rec = record
    ptr: ^fred;
    fred: integer
  end;
  fred = rec;
begin
end.
`
	if errs := Check(parseSource(t, before)); !has(errs, "used before its definition") {
		t.Fatalf("%v", errs)
	}
	after := `
program p(output);
type
  rec = record
    fred: integer;
    ptr: ^fred
  end;
begin
end.
`
	if errs := Check(parseSource(t, after)); !has(errs, "not a type") {
		t.Fatalf("%v", errs)
	}
}

func TestEnumInVariable(t *testing.T) {
	src := `program p(output);
const red = 1;
procedure q;
var colour: (red, blue);
begin
  colour := red
end;
begin
  if red = 1 then
end.
`
	if errs := Check(parseSource(t, src)); len(errs) > 0 {
		t.Fatal(errs)
	}
}

func TestStringVariableIndex(t *testing.T) {
	src := `program p(output); var s: packed array [1..3] of char; begin s[1] := 'a' end.`
	if errs := Check(parseSource(t, src)); len(errs) > 0 {
		t.Fatal(errs)
	}
}

func has(errs []Error, sub string) bool {
	for _, e := range errs {
		if strings.Contains(e.Msg, sub) {
			return true
		}
	}
	return false
}

func parseSource(t *testing.T, src string) *ast.Program {
	t.Helper()
	got, err := syntax.Parse("", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	prog, ok := got.(*ast.Program)
	if !ok {
		t.Fatalf("got %T", got)
	}
	return prog
}

func TestSetLimit(t *testing.T) {
	bad := []string{
		"program p(output); type s = set of integer; begin end.",
		"program p(output); type s = set of -1..5; begin end.",
	}
	for _, src := range bad {
		if errs := Check(parseSource(t, src)); !has(errs, "set base") {
			t.Fatal(src, errs)
		}
	}
	ok := "program p(output); type s = set of char; var x: s; begin x := [] end."
	if errs := Check(parseSource(t, ok)); len(errs) > 0 {
		t.Fatal(errs)
	}
}

func TestCharSet(t *testing.T) {
	src := "program p(output); const c = '\u0100'; begin end."
	prog := parseSource(t, src)
	if errs := Check(prog); !has(errs, "character outside the set") {
		t.Fatal(errs)
	}
}

func parseFile(t *testing.T, path string) *ast.Program {
	t.Helper()
	got, err := syntax.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	prog, ok := got.(*ast.Program)
	if !ok {
		t.Fatalf("got %T", got)
	}
	return prog
}
