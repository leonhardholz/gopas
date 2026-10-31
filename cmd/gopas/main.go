// Command gopas translates a Standard Pascal program into Go and runs it.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopas/internal/ast"
	"gopas/internal/check"
	"gopas/internal/syntax"
)

func main() {
	if err := run(os.Args[1:], stdio{os.Stdin, os.Stdout, os.Stderr}); err != nil {
		var status exitStatus
		if errors.As(err, &status) {
			os.Exit(status.code)
		}
		fmt.Fprintln(os.Stderr, "gopas:", err)
		os.Exit(1)
	}
}

type stdio struct {
	in          io.Reader
	out, errOut io.Writer
}

// exitStatus is the status of the translated program. It is not printed.
type exitStatus struct{ code int }

func (e exitStatus) Error() string { return fmt.Sprintf("exit status %d", e.code) }

func run(args []string, io stdio) error {
	fs := flag.NewFlagSet("gopas", flag.ContinueOnError)
	fs.SetOutput(io.errOut)
	var (
		outPath     string
		complete    bool
		overflow    bool
		rangeCheck  bool
		disposeExpr bool
	)
	fs.StringVar(&outPath, "o", "", "write the Go source to this path instead of running the program; use - for stdout")
	fs.BoolVar(&complete, "complete", false, "evaluate both operands of and and or")
	fs.BoolVar(&overflow, "overflow", false, "stop the program when integer arithmetic overflows")
	fs.BoolVar(&rangeCheck, "range", false, "stop when a value is outside its range, or a case has no matching label")
	fs.BoolVar(&disposeExpr, "disposeexpr", false, "allow dispose of a pointer expression")
	fs.Usage = func() {
		fmt.Fprintln(io.errOut, "usage: gopas [options] file.pas")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("expected one Pascal file")
	}
	src, err := translate(fs.Arg(0), &check.Options{
		CompleteBool: complete,
		Overflow:     overflow,
		Range:        rangeCheck,
		DisposeExpr:  disposeExpr,
	})
	if err != nil {
		return err
	}
	if outPath != "" {
		return emit(src, outPath, io.out)
	}
	return execute(src, io)
}

func translate(path string, opt *check.Options) (string, error) {
	srcBytes, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	got, err := syntax.ParseFile(path)
	if err != nil {
		return "", errors.New(formatParse(path, srcBytes, err))
	}
	prog, ok := got.(*ast.Program)
	if !ok {
		return "", fmt.Errorf("%s: got %T, want a program", path, got)
	}
	if errs := check.Check(prog); check.PreferPlain(prog, errs) {
		got, err = syntax.ParseFile(path, syntax.WithRecover(false))
		if err != nil {
			return "", errors.New(formatParse(path, srcBytes, err))
		}
		prog, ok = got.(*ast.Program)
		if !ok {
			return "", fmt.Errorf("%s: got %T, want a program", path, got)
		}
	}
	src, errs := check.Generate(prog, opt)
	if len(errs) > 0 {
		return "", errors.New(formatErrs(path, srcBytes, errs))
	}
	return src, nil
}

var pigeonErr = regexp.MustCompile(`^.*:(\d+):(\d+) \(\d+\): (?:rule \S+: )?(.*)$`)

func formatParse(path string, src []byte, err error) string {
	var b strings.Builder
	n := 0
	for _, line := range strings.Split(err.Error(), "\n") {
		m := pigeonErr.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ln, _ := strconv.Atoi(m[1])
		col, _ := strconv.Atoi(m[2])
		code, msg := check.Syntax(m[3])
		if code != 0 {
			msg = fmt.Sprintf("%d: %s", code, msg)
		}
		if n > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(formatAt(path, ln, col, msg, src))
		n++
	}
	if n == 0 {
		return path + ": " + err.Error()
	}
	return b.String()
}

func formatErrs(path string, src []byte, errs []check.Error) string {
	var b strings.Builder
	for i, e := range errs {
		if i > 0 {
			b.WriteByte('\n')
		}
		msg := e.Msg
		if e.Code != 0 {
			msg = fmt.Sprintf("%d: %s", e.Code, msg)
		}
		if e.Line == 0 {
			fmt.Fprintf(&b, "%s: %s", path, msg)
			continue
		}
		b.WriteString(formatAt(path, e.Line, e.Col, msg, src))
	}
	return b.String()
}

func formatAt(path string, line, col int, msg string, src []byte) string {
	line, col = afterBreak(line, col, src)
	var b strings.Builder
	fmt.Fprintf(&b, "%s:%d:%d: %s", path, line, col, msg)
	lines := strings.Split(string(src), "\n")
	if line < 1 || line > len(lines) {
		return b.String()
	}
	shown, mark := caret(strings.TrimRight(lines[line-1], "\r"), col)
	b.WriteByte('\n')
	b.WriteString(shown)
	b.WriteByte('\n')
	b.WriteString(mark)
	return b.String()
}

// afterBreak moves a failure that sits on a line break back onto the last
// character of the line that ended. Pigeon reports that spot as column 0 of
// the next line, or as column 1 of an empty line.
func afterBreak(line, col int, src []byte) (int, int) {
	lines := strings.Split(string(src), "\n")
	if line < 1 {
		return 1, 1
	}
	if line > len(lines) {
		line = len(lines)
		col = 0
	}
	text := ""
	if line >= 1 && line <= len(lines) {
		text = strings.TrimRight(lines[line-1], "\r")
	}
	if col >= 1 && text != "" {
		return line, col
	}
	for line > 1 {
		line--
		text = strings.TrimRight(lines[line-1], "\r")
		if text != "" {
			return line, len(text)
		}
	}
	return 1, 1
}

func caret(line string, col int) (shown, mark string) {
	if col < 1 {
		col = 1
	}
	var b strings.Builder
	markAt := 0
	seen := false
	for i := 0; i < len(line); i++ {
		if i == col-1 {
			markAt = b.Len()
			seen = true
		}
		if line[i] == '\t' {
			n := 8 - b.Len()%8
			b.WriteString(strings.Repeat(" ", n))
			continue
		}
		b.WriteByte(line[i])
	}
	if !seen {
		markAt = b.Len()
	}
	return b.String(), strings.Repeat(" ", markAt) + "^"
}

func emit(src, path string, stdout io.Writer) error {
	if path == "-" {
		_, err := io.WriteString(stdout, src)
		return err
	}
	return os.WriteFile(path, []byte(src), 0o644)
}

func execute(src string, io stdio) error {
	ver, err := goVersion()
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "gopas")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	mod := "module example\n\ngo " + ver + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		return err
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	cmd.Stdin = io.in
	cmd.Stdout = io.out
	cmd.Stderr = io.errOut
	err = cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exitStatus{code: exit.ExitCode()}
	}
	return err
}

// goVersion is the language version of the go command that will build the program.
func goVersion() (string, error) {
	out, err := exec.Command("go", "env", "GOVERSION").Output()
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(out))
	v = strings.TrimPrefix(v, "go")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	if v == "" || v[0] < '0' || v[0] > '9' {
		return "", fmt.Errorf("unrecognized go version %q", strings.TrimSpace(string(out)))
	}
	return v, nil
}
