package main

import (
	"bytes"
	"context"
	"gopas/internal/check"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// prtCase is one file from testdata/p5/prt, judged against the P5 listing.
// The file number is the test number. The code is P5's compiler error number,
// which is a different sequence (14 is "';' expected", not test 14).
type prtCase struct {
	kind string
	file string
	line int
	col  int
	code int
	msg  string
	our  int
	want string
	// ours is the caret gopas reports when that place is the one a reader
	// should see. P5's line and column stay in line and col.
	oursLine int
	oursCol  int
}

func TestPRT(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "p5", "prt")
	cases := loadPRT(t, filepath.Join(dir, "..", "prt.gold"))
	seen := map[string]bool{}
	for _, c := range cases {
		seen[c.file] = true
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		name := e.Name()
		if strings.HasPrefix(name, "iso7185prt") && strings.HasSuffix(name, ".pas") && !seen[name] {
			t.Errorf("no expected result for %s", name)
		}
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			path := filepath.Join(dir, c.file)
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			switch c.kind {
			case "compile":
				checkCompile(t, path, src, c)
			case "runtime":
				checkRuntime(t, path, c, nil)
			case "range":
				checkRuntime(t, path, c, &check.Options{Range: true})
			case "overflow":
				checkRuntime(t, path, c, &check.Options{Overflow: true})
			case "complete":
				checkRuntime(t, path, c, &check.Options{CompleteBool: true})
			case "accept":
				checkAccept(t, path)
			case "skip":
				t.Skip("ISO 7185 error left unreported")
			default:
				t.Fatalf("unknown kind %q", c.kind)
			}
		})
	}
}

func checkCompile(t *testing.T, path string, src []byte, c prtCase) {
	t.Helper()
	_, err := translate(path, nil)
	if err == nil {
		t.Fatal("compiled a program P5 rejects")
	}
	line, col, code, msg, ok := firstDiag(err.Error())
	if !ok {
		t.Fatalf("no source location in:\n%s", err)
	}
	if code == 0 {
		t.Errorf("diagnostic has no error code: %s", msg)
	}
	wantLine, wantCol := c.line, c.col
	if c.oursLine != 0 {
		wantLine, wantCol = c.oursLine, c.oursCol
	}
	if !samePlace(src, line, col, wantLine, wantCol) {
		if c.oursLine != 0 {
			t.Errorf("at %d:%d, gold marks %d:%d", line, col, wantLine, wantCol)
		} else {
			t.Errorf("at %d:%d, P5 marks %d:%d", line, col, wantLine, wantCol)
		}
		return
	}
	if c.oursLine == 0 {
		if tok := p5Token(c.code); tok != "" && !strings.Contains(msg, tok) {
			t.Errorf("message %q does not mention %s", msg, tok)
		}
	}
	if c.our != 0 && (code != c.our || msg != c.want) {
		t.Errorf("got %d: %s, want %d: %s", code, msg, c.our, c.want)
	}
}

func checkRuntime(t *testing.T, path string, c prtCase, opt *check.Options) {
	t.Helper()
	prog, err := translate(path, opt)
	if err != nil {
		t.Fatalf("P5 accepts this program and rejects it at run time, compile failed:\n%s", err)
	}
	out, err := runLimited(prog, 5*time.Second)
	if err == nil {
		t.Fatalf("ran to completion; this program must stop at run time\n%s", out)
	}
}

func checkAccept(t *testing.T, path string) {
	t.Helper()
	prog, err := translate(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runLimited(prog, 5*time.Second); err != nil {
		t.Fatal(err)
	}
}

func loadPRT(t *testing.T, path string) []prtCase {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []prtCase
	for _, line := range strings.Split(string(b), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 6 && len(f) != 8 && len(f) != 10 {
			t.Fatalf("bad gold line %q", line)
		}
		lineN, err1 := strconv.Atoi(f[2])
		col, err2 := strconv.Atoi(f[3])
		code, err3 := strconv.Atoi(f[4])
		if err1 != nil || err2 != nil || err3 != nil {
			t.Fatalf("bad gold line %q", line)
		}
		c := prtCase{kind: f[0], file: f[1], line: lineN, col: col, code: code, msg: f[5]}
		if len(f) >= 8 {
			c.our, err1 = strconv.Atoi(f[6])
			c.want = f[7]
			if err1 != nil {
				t.Fatalf("bad gold line %q", line)
			}
		}
		if len(f) == 10 {
			c.oursLine, err1 = strconv.Atoi(f[8])
			c.oursCol, err2 = strconv.Atoi(f[9])
			if err1 != nil || err2 != nil {
				t.Fatalf("bad gold line %q", line)
			}
		}
		out = append(out, c)
	}
	return out
}

var diagLine = regexp.MustCompile(`^(.+):(\d+):(\d+): (\d+): (.*)$`)

func firstDiag(msg string) (line, col, code int, text string, ok bool) {
	head, _, _ := strings.Cut(msg, "\n")
	m := diagLine.FindStringSubmatch(head)
	if m == nil {
		return 0, 0, 0, "", false
	}
	var err1, err2, err3 error
	line, err1 = strconv.Atoi(m[2])
	col, err2 = strconv.Atoi(m[3])
	code, err3 = strconv.Atoi(m[4])
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, "", false
	}
	return line, col, code, m[5], true
}

// p5Token is the token a Pascal-P "X expected" error is asking for.
// Our message may name other tokens that are also legal at that point.
func p5Token(code int) string {
	switch code {
	case 3:
		return "'program'"
	case 4:
		return "')'"
	case 5:
		return "':'"
	case 8:
		return "'of'"
	case 9:
		return "'('"
	case 11:
		return "'['"
	case 12:
		return "']'"
	case 13:
		return "'end'"
	case 14:
		return "';'"
	case 16:
		return "'='"
	case 17:
		return "'begin'"
	case 20:
		return "','"
	case 21:
		return "'.'"
	case 51:
		return "':='"
	case 52:
		return "'then'"
	case 53:
		return "'until'"
	case 54:
		return "'do'"
	default:
		return ""
	}
}

// samePlace reports whether the diagnostic and P5's caret are on the same
// token. P5's column is the character it was reading, often the last character
// of that token. One column either way on that same line is still the same
// place. Column 0 is the end of the file.
func samePlace(src []byte, line, col, wantLine, wantCol int) bool {
	lines := strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
	if wantCol < 1 {
		if line == wantLine {
			return true
		}
		// The missing token is at the end of the previous line.
		if line == wantLine-1 && line >= 1 && line <= len(lines) {
			return col >= len(lines[line-1])
		}
		// At end of input, P5's line count can run past the last line.
		return atEnd(lines, line, col) && wantLine > len(lines)
	}
	if line != wantLine || line < 1 || line > len(lines) {
		return false
	}
	if col < 1 {
		col = 1
	}
	if col-wantCol <= 1 && wantCol-col <= 1 {
		return true
	}
	text := lines[line-1]
	return onToken(text, col, wantCol)
}

func atEnd(lines []string, line, col int) bool {
	last := len(lines)
	for last > 1 && lines[last-1] == "" {
		last--
	}
	if line != last || line < 1 {
		return false
	}
	return col >= len(lines[line-1])
}

func onToken(line string, a, b int) bool {
	if a == b {
		return true
	}
	if a < 1 || b < 1 || a > len(line) || b > len(line) {
		return false
	}
	s, e := tokenAt(line, b)
	return a >= s && a <= e
}

func tokenAt(line string, col int) (start, end int) {
	i := col - 1
	if i < 0 || i >= len(line) {
		return col, col
	}
	if isIdentByte(line[i]) {
		s := i
		for s > 0 && isIdentByte(line[s-1]) {
			s--
		}
		e := i + 1
		for e < len(line) && isIdentByte(line[e]) {
			e++
		}
		return s + 1, e
	}
	return col, col
}

func isIdentByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}

func runLimited(src string, limit time.Duration) (string, error) {
	ver, err := goVersion()
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "gopas")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	mod := "module example\n\ngo " + ver + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", ".")
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err = cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return out.String(), ctx.Err()
	}
	return out.String(), err
}
