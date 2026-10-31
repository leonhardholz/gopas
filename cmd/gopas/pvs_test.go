package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The Pascal Validation Suite is a set of independent programs. Each file
// states its class. Conformance programs must run and report PASS, except
// the empty program CONF024 and the page check CONF207. Deviance and
// extension programs must be rejected before they finish. An error-handling
// pretest must run; the error itself may be left undetected. The other
// classes report implementation facts and must run.

func TestPVS(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "pvs")
	var files []string
	ents, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		subs, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range subs {
			if s.IsDir() || !strings.EqualFold(filepath.Ext(s.Name()), ".pas") {
				continue
			}
			files = append(files, filepath.Join(dir, s.Name()))
		}
	}
	if len(files) == 0 {
		t.Fatal("no programs")
	}
	for _, path := range files {
		path := path
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(rel, func(t *testing.T) {
			t.Parallel()
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			class, pretest := pvsHeader(src)
			if class == "" {
				class = dirClass(rel)
			}
			got := runPVS(t, path)
			switch class {
			case "CONFORMANCE":
				checkConformance(t, got, path)
			case "DEVIANCE", "EXTENSION":
				checkStops(t, got)
			case "ERRORHANDLING":
				if pretest {
					checkRuns(t, got)
				} else {
					checkError(t, got)
				}
			case "IMPLEMENTATIONDEFINED", "IMPLEMENTATIONDEPENDENT", "IMPLEMENTATIONDEFINEDBEHAVIOUR":
				checkRuns(t, got)
			default:
				t.Fatalf("unknown class %q", class)
			}
		})
	}
}

type pvsResult struct {
	kind string
	out  string
}

func runPVS(t *testing.T, path string) pvsResult {
	t.Helper()
	prog, err := translate(path, nil)
	if err != nil {
		return pvsResult{kind: "compile", out: err.Error()}
	}
	out, err := runLimited(prog, 10*time.Second)
	if badGo(out) {
		return pvsResult{kind: "badgo", out: out}
	}
	if err != nil {
		return pvsResult{kind: "stop", out: out}
	}
	return pvsResult{kind: "ran", out: out}
}

func checkConformance(t *testing.T, got pvsResult, path string) {
	t.Helper()
	out := checkRuns(t, got)
	name := strings.ToUpper(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	switch name {
	case "CONF024":
		if strings.TrimSpace(out) != "" {
			t.Fatalf("the empty program wrote:\n%s", out)
		}
	case "CONF207":
		if !strings.Contains(out, "\f") {
			t.Fatal("page did not write a form feed")
		}
	default:
		if !strings.Contains(out, "PASS") || strings.Contains(out, "FAIL") {
			t.Fatalf("output:\n%s", out)
		}
	}
}

func checkRuns(t *testing.T, got pvsResult) string {
	t.Helper()
	if got.kind != "ran" {
		t.Fatalf("%s\n%s", got.kind, got.out)
	}
	return got.out
}

func checkStops(t *testing.T, got pvsResult) {
	t.Helper()
	if got.kind == "badgo" {
		t.Fatalf("generated Go did not compile:\n%s", got.out)
	}
	if got.kind == "ran" {
		t.Fatalf("ran to completion\n%s", got.out)
	}
}

func checkError(t *testing.T, got pvsResult) {
	t.Helper()
	if got.kind == "badgo" {
		t.Fatalf("generated Go did not compile:\n%s", got.out)
	}
	// A designated error may go unreported. A run that stops, diverges,
	// or finishes is acceptable. A Go compile error is not.
}

func badGo(out string) bool {
	return strings.Contains(out, "# example\n") || strings.Contains(out, "# example\r\n")
}

func pvsHeader(src []byte) (class string, pretest bool) {
	text := string(src)
	const mark = "CLASS="
	if i := strings.Index(text, mark); i >= 0 {
		rest := text[i+len(mark):]
		end := strings.IndexAny(rest, ", }\r\n")
		if end >= 0 {
			class = rest[:end]
		}
	}
	head := text
	if j := strings.Index(strings.ToLower(text), "program "); j >= 0 {
		head = text[:j]
	}
	pretest = strings.Contains(head, "PRETEST")
	return class, pretest
}

func dirClass(rel string) string {
	dir := rel
	if i := strings.IndexAny(rel, `/\`); i >= 0 {
		dir = rel[:i]
	}
	switch strings.ToUpper(dir) {
	case "CONFORM":
		return "CONFORMANCE"
	case "DEVIANCE":
		return "DEVIANCE"
	case "ERROR":
		return "ERRORHANDLING"
	case "EXTEND":
		return "EXTENSION"
	case "IMPDEF":
		return "IMPLEMENTATIONDEFINED"
	case "IMPDEP":
		return "IMPLEMENTATIONDEPENDENT"
	case "IMPDEFB":
		return "IMPLEMENTATIONDEFINEDBEHAVIOUR"
	default:
		return ""
	}
}
