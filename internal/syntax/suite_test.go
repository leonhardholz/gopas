package syntax

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The Pascal Validation Suite and the Pascal Acceptance Test are programs,
// not executions. These tests cover the files whose result is fixed before
// a program runs: a conforming program parses, and an extension does not.
// Deviance tests and the rejection test mix syntax errors with programs
// that are syntactically valid and fail later, so they are kept in testdata
// and not given a single parse result.

func TestValidationSuiteParses(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "pvs")
	for _, dir := range []string{"CONFORM", "ERROR", "IMPDEF", "IMPDEP", "IMPDEFB"} {
		parseDir(t, filepath.Join(root, dir), true)
	}
	level1 := filepath.Join(root, "LEVEL1")
	n := 0
	eachPascal(t, level1, func(path string) {
		switch pvsClass(t, path) {
		case "CONFORMANCE", "ERRORHANDLING", "IMPLEMENTATIONDEFINED", "IMPLEMENTATIONDEFINEDBEHAVIOUR":
			n++
			if err := parseProgram(path); err != nil {
				t.Errorf("%s: %v", path, err)
			}
		}
	})
	if n == 0 {
		t.Fatal("LEVEL1: no programs")
	}
}

func TestValidationSuiteExtensions(t *testing.T) {
	// Each program is rejected at the first character of its extension.
	// A failure earlier in the file is a different error.
	cases := []struct {
		file      string
		line, col int
		token     string
	}{
		{"EXTEND1.PAS", 26, 8, "_"},         // t_1: an identifier cannot contain an underscore
		{"EXTEND2.PAS", 31, 10, "GT"},       // GT is not a relational operator
		{"EXTEND3.PAS", 25, 6, "%"},         // % is not a colon
		{"EXTEND4.PAS", 30, 7, "otherwise"}, // a case has no otherwise clause
		{"EXTEND5.PAS", 19, 21, "_"},        // an underscore does not separate tokens
		{"EXTEND6.PAS", 20, 5, "_"},         // a__a: adjacent underscores are not an identifier
		{"EXTEND7.PAS", 20, 4, "_"},         // _aa: an identifier cannot start with an underscore
		{"EXTEND8.PAS", 19, 6, "_"},         // aa_: an identifier cannot end with an underscore
		{"EXTEND9.PAS", 23, 5, "_"},         // a_a: an identifier cannot contain an underscore
	}
	dir := filepath.Join("..", "..", "testdata", "pvs", "EXTEND")
	for _, tt := range cases {
		path := filepath.Join(dir, tt.file)
		err := parseProgram(path)
		if err == nil {
			t.Errorf("%s: accepted %s", tt.file, tt.token)
			continue
		}
		at := fmt.Sprintf(":%d:%d (", tt.line, tt.col)
		if !strings.Contains(err.Error(), at) {
			t.Errorf("%s: error %v, want it at %s", tt.file, err, at)
		}
		if got := tokenAt(t, path, tt.line, tt.col); !strings.HasPrefix(got, tt.token) {
			t.Errorf("%s:%d:%d is %q, want %q", tt.file, tt.line, tt.col, got, tt.token)
		}
	}
}

func tokenAt(t *testing.T, path string, line, col int) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(b), "\n")
	if line < 1 || line > len(lines) || col < 1 || col > len(lines[line-1]) {
		t.Fatalf("%s: no character at %d:%d", path, line, col)
	}
	return lines[line-1][col-1:]
}

func TestPascalAcceptance(t *testing.T) {
	for _, name := range []string{"iso7185pat.pas", "iso7185pat0001.pas"} {
		path := filepath.Join("..", "..", "testdata", "p5", name)
		if err := parseProgram(path); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func parseDir(t *testing.T, dir string, accept bool) {
	t.Helper()
	n := 0
	eachPascal(t, dir, func(path string) {
		n++
		err := parseProgram(path)
		if accept && err != nil {
			t.Errorf("%s: %v", filepath.Base(path), err)
		}
		if !accept && err == nil {
			t.Errorf("%s: parsed an extension", filepath.Base(path))
		}
	})
	if n == 0 {
		t.Fatalf("%s: no programs", dir)
	}
}

func eachPascal(t *testing.T, dir string, visit func(string)) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range entries {
		if ent.IsDir() || !strings.EqualFold(filepath.Ext(ent.Name()), ".pas") {
			continue
		}
		visit(filepath.Join(dir, ent.Name()))
	}
}

func parseProgram(path string) error {
	_, err := ParseFile(path)
	return err
}

func pvsClass(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	const mark = "CLASS="
	i := strings.Index(text, mark)
	if i < 0 {
		t.Fatalf("%s: no CLASS", filepath.Base(path))
	}
	rest := text[i+len(mark):]
	end := strings.IndexAny(rest, ", }\r\n")
	if end < 0 {
		t.Fatalf("%s: bad CLASS", filepath.Base(path))
	}
	return rest[:end]
}
