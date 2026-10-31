package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTranslateHello(t *testing.T) {
	src, err := translate(filepath.Join("..", "..", "testdata", "ok", "hello.pas"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(src, "package main") || !strings.Contains(src, "Hello") || strings.Contains(src, "gopas/runtime") {
		t.Fatalf("generated source:\n%s", src)
	}
}

func TestTranslateRejects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.pas")
	if err := os.WriteFile(path, []byte("program p(output); begin n := 1 end."), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := translate(path, nil)
	if err == nil {
		t.Fatal("expected errors")
	}
	got := err.Error()
	want := path + ":1:26: 104: identifier 'n' not declared\nprogram p(output); begin n := 1 end.\n                         ^"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestTranslateSyntax(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.pas")
	src := "program p(output); begin $ end."
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := translate(path, nil)
	if err == nil {
		t.Fatal("expected a syntax error")
	}
	got := err.Error()
	if !strings.Contains(got, path+":1:26: 6: expected") {
		t.Fatalf("got\n%s", got)
	}
	if !strings.Contains(got, "\nprogram p(output); begin $ end.\n                         ^") {
		t.Fatalf("got\n%s", got)
	}
}

func TestEmit(t *testing.T) {
	out := filepath.Join(t.TempDir(), "main.go")
	hello := filepath.Join("..", "..", "testdata", "ok", "hello.pas")
	if err := run([]string{"-o", out, hello}, stdio{}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "package main") {
		t.Fatalf("wrote:\n%s", b)
	}
}

func TestBuildHello(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "hello")
	hello := filepath.Join("..", "..", "testdata", "ok", "hello.pas")
	if err := run([]string{"-b", bin, hello}, stdio{}); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if string(out) != "Hello\n" {
		t.Fatalf("output %q", out)
	}
}

func TestRunHello(t *testing.T) {
	var out bytes.Buffer
	hello := filepath.Join("..", "..", "testdata", "ok", "hello.pas")
	err := run([]string{hello}, stdio{in: strings.NewReader(""), out: &out, errOut: &out})
	if err != nil {
		t.Fatal(err, out.String())
	}
	if out.String() != "Hello\n" {
		t.Fatalf("output %q", out.String())
	}
}
