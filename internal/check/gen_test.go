package check

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRuntimeImports(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		has     []string
		missing []string
	}{
		{
			name:    "text",
			src:     "program hello(output); begin writeln('Hello') end.",
			missing: []string{`"math"`, `"encoding/gob"`, `"encoding/binary"`, `"unsafe"`},
		},
		{
			name:    "set",
			src:     "program s(output); var a: set of 0..9; begin a := [1]; writeln(1 in a) end.",
			missing: []string{`"math"`, `"encoding/gob"`, `"encoding/binary"`, `"unsafe"`},
		},
		{
			name:    "real",
			src:     "program r(output); begin writeln(sin(0)) end.",
			has:     []string{`"math"`},
			missing: []string{`"encoding/gob"`, `"encoding/binary"`, `"unsafe"`},
		},
		{
			name:    "file",
			src:     "program f(output); var g: file of integer; begin rewrite(g); write(g, 1) end.",
			has:     []string{`"encoding/binary"`, `"unsafe"`},
			missing: []string{`"math"`, `"encoding/gob"`},
		},
	}
	for _, tt := range cases {
		src, errs := Generate(parseSource(t, tt.src), nil)
		if len(errs) > 0 {
			t.Fatalf("%s: %v", tt.name, errs)
		}
		for _, s := range tt.has {
			if !strings.Contains(src, s) {
				t.Errorf("%s: missing %s", tt.name, s)
			}
		}
		for _, s := range tt.missing {
			if strings.Contains(src, s) {
				t.Errorf("%s: includes %s", tt.name, s)
			}
		}
	}
}

func TestDisposeExpr(t *testing.T) {
	src := `
program p(output);
type iptr = ^integer;
var n: iptr;
function frp: iptr;
begin
  frp := n
end;
begin
  dispose(frp)
end.
`
	prog := parseSource(t, src)
	if errs := Check(prog); len(errs) != 0 {
		t.Fatal(errs)
	}
	got, errs := Generate(prog, nil)
	if len(errs) > 0 {
		t.Fatal(errs, got)
	}
	if !strings.Contains(got, "_ = frp()") {
		t.Fatalf("dispose of an expression missing from\n%s", got)
	}
}

func TestISO7185PAT(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	prog := parseFile(t, filepath.Join(root, "testdata/p5/iso7185pat.pas"))
	src, errs := Generate(prog, &Options{DisposeExpr: true})
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if !strings.Contains(src, "_ = frp()") {
		t.Fatal("dispose(frp) was not translated")
	}
	dir := t.TempDir()
	mod := "module example\n\ngo 1.18\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", filepath.Join(dir, "pat"), ".")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	inPath := filepath.Join(root, "testdata/p5/iso7185pat.inp")
	in, err := os.Open(inPath)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	run := exec.CommandContext(ctx, filepath.Join(dir, "pat"))
	run.Stdin = in
	out, err := run.CombinedOutput()
	if err != nil {
		tail := out
		if len(tail) > 2000 {
			tail = tail[len(tail)-2000:]
		}
		t.Fatalf("run: %v\n%s", err, tail)
	}
	raw, err := os.ReadFile(inPath)
	if err != nil {
		t.Fatal(err)
	}
	if msgs := checkPATOutput(string(out), string(raw)); len(msgs) > 0 {
		n := len(msgs)
		if n > 40 {
			msgs = msgs[:40]
		}
		t.Fatalf("%d output mismatches\n%s", n, strings.Join(msgs, "\n"))
	}
}

func TestConformantReadOnly(t *testing.T) {
	src := `program p(output);
type rec = record n: integer end;
var r: array [1..3] of integer;
procedure look(a: array [i..j: integer] of integer);
begin
  writeln(a[i]:1)
end;
procedure poke(var a: array [i..j: integer] of integer);
begin
  a[i] := 9
end;
procedure wrap(a: array [i..j: integer] of integer);
begin
  poke(a)
end;
procedure outer(a: array [i..j: integer] of integer);
  procedure inner;
  begin
    a[i] := 9
  end;
begin
  inner
end;
procedure setf(a: array [i..j: integer] of rec);
begin
  with a[i] do n := 9
end;
begin
  r[1] := 1;
  look(r);
  writeln(r[1]:1);
  wrap(r);
  writeln(r[1]:1);
  outer(r);
  writeln(r[1]:1)
end.
`
	prog := parseSource(t, src)
	out, errs := Generate(prog, nil)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if !strings.Contains(out, "func look(a *int, i int, j int)") {
		t.Fatal("read-only parameter was copied\n", out)
	}
	for _, name := range []string{"wrap", "outer"} {
		if !strings.Contains(out, "func "+name+"(_a *int, i int, j int)") {
			t.Fatalf("%s was not copied\n%s", name, out)
		}
	}
	if !strings.Contains(out, "func setf(_a *rec, i int, j int)") {
		t.Fatal("with assignment was not treated as a write\n", out)
	}
	if got := runGenerated(t, src); got != "1\n1\n1\n1\n" {
		t.Fatalf("got %q", got)
	}
}

func TestStringOrder(t *testing.T) {
	src := `program p(output);
var s, t: packed array [1..4] of char;
begin
  s := 'abxd';
  t := 'abyd';
  writeln(s < t, s > t, s <= t, s >= t, s = t);
  s := t;
  writeln(s < t, s <= t, 'abxd' < t)
end.
`
	out, errs := Generate(parseSource(t, src), nil)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if !strings.Contains(out, "rtStrLess(") || strings.Contains(out, "func() bool") {
		t.Fatalf("string comparison was not a call:\n%s", out)
	}
	if got := runGenerated(t, src); got != " TRUEFALSE TRUEFALSEFALSE\nFALSE TRUE TRUE\n" {
		t.Fatalf("got %q", got)
	}
}

func TestPromptVisibleBeforeRead(t *testing.T) {
	src := `program p(input, output);
begin
  write('go');
  readln
end.
`
	prog := parseSource(t, src)
	out, errs := Generate(prog, nil)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example\n\ngo 1.18\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "p")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = dir
	if bout, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, bout)
	}
	cmd := exec.Command(bin)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	got := make([]byte, 2)
	done := make(chan error, 1)
	go func() {
		_, err := io.ReadFull(stdout, got)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("prompt was not written before readln")
	}
	if string(got) != "go" {
		t.Fatalf("got %q", got)
	}
	if _, err := io.WriteString(stdin, "\n"); err != nil {
		t.Fatal(err)
	}
	stdin.Close()
	cmd.Wait()
}

func TestFlushPartialLine(t *testing.T) {
	src := `program p(output);
begin
  write('x')
end.
`
	if got := runGenerated(t, src); got != "x" {
		t.Fatalf("got %q", got)
	}
}

func TestDirectFunc(t *testing.T) {
	src := `program p(output);
var n: integer;
procedure inc;
begin
  n := n + 1
end;
procedure outer;
var m: integer;
  procedure inner;
  begin
    m := 4
  end;
begin
  m := 1;
  inner;
  n := m
end;
begin
  outer;
  inc;
  writeln(n:1)
end.
`
	prog := parseSource(t, src)
	out, errs := Generate(prog, nil)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if !strings.Contains(out, "func inc(") || !strings.Contains(out, "func outer(") {
		t.Fatalf("routine was not a function\n%s", out)
	}
	if strings.Contains(out, "var inc func") || strings.Contains(out, "var outer func") {
		t.Fatalf("routine is still a function variable\n%s", out)
	}
	if !strings.Contains(out, "inner = func(") {
		t.Fatalf("capturing routine was not a closure\n%s", out)
	}
	if got := runGenerated(t, src); got != "5\n" {
		t.Fatalf("got %q", got)
	}
	same := `program p(output);
procedure a;
  procedure q;
  begin
    writeln(1:1)
  end;
begin
  q
end;
procedure b;
  procedure q;
  begin
    writeln(2:1)
  end;
begin
  q
end;
begin
  a;
  b
end.
`
	if got := runGenerated(t, same); got != "1\n2\n" {
		t.Fatalf("same name: got %q", got)
	}
	mutual := `program p(output);
procedure a(k: integer); forward;
procedure b(k: integer);
begin
  if k > 0 then a(k - 1)
end;
procedure a;
begin
  writeln(k:1);
  if k > 0 then b(k - 1)
end;
begin
  a(2)
end.
`
	if got := runGenerated(t, mutual); got != "2\n0\n" {
		t.Fatalf("mutual: got %q", got)
	}
}

func TestDiscardUnread(t *testing.T) {
	src := `program p(output);
procedure q(a, b: integer);
var c: integer;
begin
  c := a;
  writeln(c:1)
end;
procedure sink;
var n: integer;
begin
  n := 1
end;
procedure outer(i: integer);
  procedure inner(i: integer);
  begin
    writeln(i:1)
  end;
begin
  inner(7)
end;
begin
  q(3, 4);
  sink;
  outer(9)
end.
`
	prog := parseSource(t, src)
	out, errs := Generate(prog, nil)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if strings.Contains(out, "_ = &") {
		t.Fatalf("address taken to silence an unused name\n%s", out)
	}
	if !strings.Contains(out, "_ = b") {
		t.Fatalf("unused parameter was not discarded\n%s", out)
	}
	if strings.Contains(out, "_ = a") || strings.Contains(out, "_ = c") {
		t.Fatalf("a read name was discarded\n%s", out)
	}
	if got := runGenerated(t, src); got != "3\n7\n" {
		t.Fatalf("got %q", got)
	}
}

func TestNestedResultAndFarGoto(t *testing.T) {
	cases := []struct {
		src, want string
	}{
		{`program p(output);
function junk20: integer;
var i: integer;
function inner: integer;
begin
  inner := 12;
  junk20 := 37
end;
begin
  i := inner
end;
begin
  writeln(junk20:1)
end.
`, "37\n"},
		{`program p(output);
label 1;
procedure j;
begin
  goto 1
end;
begin
  j;
  writeln('bad');
  1: writeln('ok')
end.
`, "ok\n"},
	}
	for _, tc := range cases {
		if got := runGenerated(t, tc.src); got != tc.want {
			t.Fatalf("got %q want %q", got, tc.want)
		}
	}
}

func TestLoopRestartsAfterGoto(t *testing.T) {
	// A goto to a label in a loop body must not decide where the next
	// pass starts. The P5 compiler does this in new and dispose:
	// while sy = comma, with goto 1 at the end of the body.
	// n counts the statement the goto skips. k counts arrivals at the label.
	cases := []struct {
		src, want string
	}{
		{`program p(output);
label 1;
var i, n, k: integer;
begin
  i := 0;
  while i < 2 do
    begin
      i := i+1;
      goto 1;
      n := n+1;
      1: k := k+1
    end;
  writeln(i:1);
  writeln(n:1);
  writeln(k:1)
end.
`, "2\n0\n2\n"},
		{`program p(output);
label 1;
var i, n, k: integer;
begin
  i := 0;
  repeat
    i := i+1;
    goto 1;
    n := n+1;
    1: k := k+1
  until i = 2;
  writeln(i:1);
  writeln(n:1);
  writeln(k:1)
end.
`, "2\n0\n2\n"},
		{`program p(output);
label 1;
var i, n, k: integer;
begin
  for i := 1 to 2 do
    begin
      n := n+1;
      goto 1;
      n := n+10;
      1: k := k+1
    end;
  writeln(n:1);
  writeln(k:1)
end.
`, "2\n2\n"},
	}
	for _, tc := range cases {
		if got := runLimited(t, tc.src); got != tc.want {
			t.Fatalf("got %q want %q\n%s", got, tc.want, tc.src)
		}
	}
}

func runLimited(t *testing.T, src string) string {
	t.Helper()
	prog := parseSource(t, src)
	out, errs := Generate(prog, nil)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	dir := t.TempDir()
	mod := "module example\n\ngo 1.18\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	build := exec.Command("go", "build", "-o", filepath.Join(dir, "p"), ".")
	build.Dir = dir
	if bout, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s\n%s", err, bout, out)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(dir, "p"))
	got, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("program did not finish\n%s", out)
	}
	if err != nil {
		t.Fatalf("run: %v\n%s\n%s", err, got, out)
	}
	return string(got)
}

func runGenerated(t *testing.T, src string) string {
	t.Helper()
	prog := parseSource(t, src)
	out, errs := Generate(prog, nil)
	if len(errs) > 0 {
		t.Fatal(errs, out)
	}
	dir := t.TempDir()
	mod := "module example\n\ngo 1.18\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s\n%s", err, got, out)
	}
	return string(got)
}

func TestRangeChecks(t *testing.T) {
	stops := []string{
		`program p; var i: 1..9; begin for i := 0 to 10 do begin end end.`,
		`program p; var a: set of 1..5; begin a := [3, 6] end.`,
		`program p; var a: array [1..4] of integer; z: packed array [1..2] of integer; i: integer;
begin i := 4; pack(a, i, z) end.`,
		`program p; type e = (one, two, three); var f: file of two..three; begin rewrite(f); write(f, one) end.`,
	}
	for _, src := range stops {
		if err := runOpt(t, src, &Options{Range: true}); err == nil {
			t.Fatalf("completed with range checks:\n%s", src)
		}
	}
	open := []string{
		`program p; var i: 1..9; begin for i := 10 to 0 do begin end end.`,
		`program p; var a: array [1..4] of integer; z: packed array [1..2] of integer; i: integer;
begin i := 4; pack(a, i, z) end.`,
	}
	for _, src := range open {
		if err := runOpt(t, src, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := runOpt(t, open[0], &Options{Range: true}); err != nil {
		t.Fatal(err)
	}
}

func runOpt(t *testing.T, src string, opt *Options) error {
	t.Helper()
	prog := parseSource(t, src)
	out, errs := Generate(prog, opt)
	if len(errs) > 0 {
		t.Fatal(errs, out)
	}
	dir := t.TempDir()
	mod := "module example\n\ngo 1.18\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	got, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v\n%s", err, got)
	}
	return nil
}

func TestConstOverflow(t *testing.T) {
	pos := `program p(output);
var i: integer;
begin
  i := (maxint - (maxint div 2)) * 2 + 2;
  if i > 0 then writeln('pos') else writeln('neg')
end.`
	out, errs := Generate(parseSource(t, pos), nil)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if !strings.Contains(out, "rtMul(") {
		t.Fatalf("overflow stayed a constant:\n%s", out)
	}
	if got := runGenerated(t, pos); got != "neg\n" {
		t.Fatalf("got %q", got)
	}
	if err := runOpt(t, pos, &Options{Overflow: true}); err == nil {
		t.Fatal("overflow check did not stop")
	}
	neg := `program p(output);
var i: integer;
begin
  i := (-maxint - (-maxint div 2)) * 2 - 2;
  writeln('done')
end.`
	out, errs = Generate(parseSource(t, neg), nil)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if !strings.Contains(out, "rtSub(") {
		t.Fatalf("overflow stayed a constant:\n%s", out)
	}
	if err := runOpt(t, neg, nil); err != nil {
		t.Fatal(err)
	}
	fit := `program p(output);
var i: integer;
begin
  i := 1 + 2
end.`
	out, errs = Generate(parseSource(t, fit), nil)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if strings.Contains(out, "rtAdd(") {
		t.Fatalf("in-range addition was a call:\n%s", out)
	}
}

func TestGenerateSwitches(t *testing.T) {
	src := "program p(output); var n: 1..2; begin n := 1; case n of 1: n := 2 end end."
	got, errs := Generate(parseSource(t, src), &Options{Range: true})
	if len(errs) > 0 {
		t.Fatal(errs, got)
	}
	if !strings.Contains(got, `panic("case")`) || !strings.Contains(got, "rtInRange") {
		t.Fatalf("range switch missing from\n%s", got)
	}
	got, errs = Generate(parseSource(t, "program p(output); var b: boolean; begin b := true and false end."), &Options{CompleteBool: true})
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if !strings.Contains(got, "func() bool") {
		t.Fatalf("complete evaluation missing from\n%s", got)
	}
}
