package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Whetstone and Dhrystone are the Pascal sources in testdata/bench.
// Each test translates and builds once, then times a single run of
// about a second. The run is long enough that starting the process
// is only a small part of the measurement.
//
// whetstoneInner and whetstoneOuter match the constants in whetstone.pas.
// Kilowhetstones per second are 100 * outer * inner / seconds. MWIPS is
// that figure divided by 1000.

const (
	dhrystoneRuns  = 16500000
	whetstoneInner = 100
	whetstoneOuter = 210
)

func TestWhetstone(t *testing.T) {
	bin := buildBench(t, "whetstone.pas")
	got, seconds := runBench(t, bin, "")
	if !strings.Contains(got, "9.9724988856150087E-01") || !strings.Contains(got, "-8.6630136653807888E-01") {
		t.Fatalf("unexpected output:\n%s", got)
	}
	mwips := float64(whetstoneOuter*whetstoneInner) / (10 * seconds)
	t.Logf("%.2fs  %.0f MWIPS", seconds, mwips)
}

func TestDhrystone(t *testing.T) {
	bin := buildBench(t, "dhry.pas")
	in := strconv.Itoa(dhrystoneRuns) + "\n"
	got, seconds := runBench(t, bin, in)
	if !strings.Contains(got, "Execution ends") || !strings.Contains(got, strconv.Itoa(dhrystoneRuns+10)) {
		t.Fatalf("unexpected output:\n%s", got)
	}
	t.Logf("%.2fs  %.0f dhrystones/s", seconds, float64(dhrystoneRuns)/seconds)
}

func buildBench(t testing.TB, name string) string {
	t.Helper()
	src, err := translate(filepath.Join("..", "..", "testdata", "bench", name), nil)
	if err != nil {
		t.Fatal(err)
	}
	ver, err := goVersion()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module bench\n\ngo "+ver+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", "bench", ".")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return filepath.Join(dir, "bench")
}

func runBench(t testing.TB, bin, in string) (string, float64) {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Stdin = strings.NewReader(in)
	start := time.Now()
	out, err := cmd.CombinedOutput()
	seconds := time.Since(start).Seconds()
	if err != nil {
		t.Fatalf("%s: %v\n%s", bin, err, out)
	}
	return string(out), seconds
}
