package check

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// checkPATOutput compares a Pascal acceptance test listing with the
// expectations that listing prints. A result and its expectation are split
// by "s/b". Boolean letter case is ignored. A real matches when it is within
// one unit of the last digit of the printed expectation, because the
// characters used for a real are implementation-defined, and the
// transcendental expectations are rounded to about four digits. File23 is checked
// against the input file, because the expectation block shipped with the
// test names different lines than iso7185pat.inp.
func checkPATOutput(out, input string) []string {
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	var msgs []string
	for _, line := range lines {
		if strings.Contains(line, "*** bad") {
			msgs = append(msgs, "failure mark: "+line)
		}
	}
	for i := 0; i < len(lines); i++ {
		if !strings.Contains(lines[i], "s/b") {
			continue
		}
		left, right := splitSB(lines[i])
		if sbBlock(right) {
			n, block := checkSBBlock(lines, i, left, input)
			msgs = append(msgs, block...)
			i += n
			continue
		}
		got := stripLabel(left)
		if got == "" && i > 0 {
			got = stripLabel(lines[i-1])
		}
		if !valuesMatch(got, right) {
			msgs = append(msgs, fmt.Sprintf("line %d: got %q want %q", i+1, got, right))
		}
	}
	return msgs
}

func splitSB(line string) (left, right string) {
	i := strings.Index(line, "s/b")
	left = strings.TrimRight(line[:i], " ")
	right = line[i+len("s/b"):]
	right = strings.TrimPrefix(right, ":")
	right = strings.TrimPrefix(right, " ")
	return left, right
}

// sbBlock is an expectation that occupies the following lines.
func sbBlock(right string) bool {
	return right == "" || strings.HasPrefix(right, "(")
}

var (
	labelOnly = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*:[ \t]*$`)
	labelHead = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*:[ \t]*`)
	bareName  = regexp.MustCompile(`^[A-Za-z]+[0-9]+[ \t]+`)
	boolToken = regexp.MustCompile(`(?i)^(true|tru|tr|t|false|fals|fal|fa|f)$`)
)

func stripLabel(s string) string {
	for {
		next := labelHead.ReplaceAllString(s, "")
		if next == s {
			break
		}
		s = next
	}
	if m := bareName.FindString(s); m != "" {
		s = s[len(m):]
	}
	return s
}

func checkSBBlock(lines []string, at int, left, input string) (int, []string) {
	start := at
	for start > 0 && !labelOnly.MatchString(lines[start-1]) {
		start--
		if strings.Contains(lines[start], "s/b") {
			start++
			break
		}
	}
	got := append([]string(nil), lines[start:at]...)
	end := at + 1
	for end < len(lines) {
		line := lines[end]
		if labelOnly.MatchString(line) || strings.HasPrefix(line, "*") || strings.Contains(line, "s/b") {
			break
		}
		end++
	}
	want := append([]string(nil), lines[at+1:end]...)
	got = trimBlank(got)
	want = trimBlank(want)
	if len(got) > len(want) {
		got = got[len(got)-len(want):]
	}
	var msgs []string
	if start > 0 && strings.TrimSpace(stripLabel(left)) == "" && strings.HasPrefix(strings.TrimSpace(lines[start-1]), "File23:") {
		return end - at - 1, checkFile23(got, want, input)
	}
	if len(got) != len(want) {
		msgs = append(msgs, fmt.Sprintf("line %d: %d result lines, %d expected", at+1, len(got), len(want)))
		return end - at - 1, msgs
	}
	for i := range want {
		if !valuesMatch(got[i], want[i]) {
			msgs = append(msgs, fmt.Sprintf("line %d: got %q want %q", start+i+1, got[i], want[i]))
		}
	}
	return end - at - 1, msgs
}

func checkFile23(got, want []string, input string) []string {
	in := trimBlank(strings.Split(strings.TrimRight(input, "\n"), "\n"))
	var msgs []string
	if len(got) != 5 || len(in) < 5 || len(want) < 5 {
		return []string{fmt.Sprintf("File23: got %d lines, input %d, expected %d", len(got), len(in), len(want))}
	}
	for i := 0; i < 3; i++ {
		if got[i] != in[i] {
			msgs = append(msgs, fmt.Sprintf("File23 line %d: got %q input %q", i+1, got[i], in[i]))
		}
	}
	for i := 3; i < 5; i++ {
		if !valuesMatch(got[i], want[i]) {
			msgs = append(msgs, fmt.Sprintf("File23 line %d: got %q want %q", i+1, got[i], want[i]))
		}
	}
	return msgs
}

func trimBlank(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	return lines
}

func valuesMatch(got, want string) bool {
	if norm(got) == norm(want) {
		return true
	}
	// Control17 writes one extra copy of its first word before "s/b".
	gt := strings.Fields(got)
	wt := strings.Fields(want)
	if len(gt) == len(wt)+1 && len(gt) > 1 && gt[len(gt)-1] == gt[0] {
		gt = gt[:len(gt)-1]
	}
	if len(gt) != len(wt) {
		return false
	}
	for i := range gt {
		if norm(gt[i]) == norm(wt[i]) {
			continue
		}
		if !looksReal(gt[i]) || !looksReal(wt[i]) || !realsClose(gt[i], wt[i]) {
			return false
		}
	}
	return true
}

func norm(s string) string {
	parts := strings.Fields(s)
	for i, p := range parts {
		if boolToken.MatchString(p) {
			parts[i] = strings.ToLower(p)
		}
	}
	return strings.Join(parts, " ")
}

func looksReal(s string) bool {
	return strings.ContainsAny(s, ".eE")
}

func realsClose(got, want string) bool {
	g, err1 := strconv.ParseFloat(got, 64)
	w, err2 := strconv.ParseFloat(want, 64)
	if err1 != nil || err2 != nil {
		return false
	}
	tol := printedULP(want)
	if rel := 1.5e-4 * math.Abs(w); rel > tol {
		tol = rel
	}
	return math.Abs(g-w) <= tol
}

// printedULP is one unit of the last digit in a printed real.
func printedULP(s string) float64 {
	s = strings.TrimSpace(s)
	exp := 0
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		exp, _ = strconv.Atoi(s[i+1:])
		s = s[:i]
	}
	frac := 0
	if i := strings.IndexByte(s, '.'); i >= 0 {
		frac = len(s) - i - 1
	}
	return math.Pow(10, float64(exp-frac))
}

func TestPATOutputCheck(t *testing.T) {
	out := strings.Join([]string{
		"Integer1:   121 s/b 121",
		"Integer11:   TRUE s/b true",
		"Boolean45:",
		"     FALSE",
		"FALS",
		"Boolean45: s/b:",
		"     false",
		"fals",
		"Real1:   1.5540000000000000E+00 s/b  1.554000e+00",
		"Real10: 4.2346 s/b 4.2345",
		"Real32: -3.4430594170275586E-01 s/b -3.44290e-01",
		"Control17: start good start s/b start good",
		"File23:",
		"Test line 1",
		"Test line 3",
		"Test line 4",
		"4567",
		"-1.2345678900000001E-10",
		"s/b",
		"Test line 1",
		"Test line 2",
		"Test line 3",
		"4567",
		"-1.234567890000000e-10",
	}, "\n")
	input := "Test line 1\nTest line 3\nTest line 4\n4567\n-1.234567890e-10;\n"
	if msgs := checkPATOutput(out, input); len(msgs) > 0 {
		t.Fatal(msgs)
	}
	bad := "Integer1:   120 s/b 121\n"
	if msgs := checkPATOutput(bad, ""); len(msgs) != 1 {
		t.Fatalf("mismatch count %d: %v", len(msgs), msgs)
	}
}
