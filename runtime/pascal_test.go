package runtime

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestStrLess(t *testing.T) {
	if !StrLess([]byte("abxd"), []byte("abyd")) {
		t.Fatal("abxd < abyd")
	}
	if StrLess([]byte("abyd"), []byte("abxd")) || StrLess([]byte("abyd"), []byte("abyd")) {
		t.Fatal("order")
	}
}

func TestBundleParts(t *testing.T) {
	prog := "package main\nfunc main() {}\n"
	text, err := Bundle(prog, PartText)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "encoding/gob") || strings.Contains(text, "\"math\"") {
		t.Fatal("text runtime includes an unused import")
	}
	if !strings.Contains(text, "func rtOutput()") || !strings.Contains(text, "defer rtCleanup()") {
		t.Fatal("text runtime is incomplete")
	}
	files, err := Bundle(prog, PartFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(files, "encoding/binary") || strings.Contains(files, "\"math\"") || strings.Contains(files, "encoding/gob") {
		t.Fatal("typed-file runtime has the wrong imports")
	}
	real, err := Bundle(prog, PartReal)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(real, "\"math\"") || strings.Contains(real, "rtCleanup") || strings.Contains(real, "encoding/binary") {
		t.Fatal("real runtime has the wrong imports")
	}
}

func TestWriteWidths(t *testing.T) {
	var b bytes.Buffer
	w := Text{out: &b}
	w.WriteInt(1, -1)
	w.WriteBool(true, -1)
	w.WriteBool(false, -1)
	w.WriteChar('X', -1)
	w.WriteStr("Hello", -1)
	w.WriteChar('X', 0)
	w.Ln()
	got := b.String()
	want := "          1 TRUEFALSEXHello\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestWriteRealWidth(t *testing.T) {
	cases := []struct {
		x     float64
		width int
		frac  int
		want  string
	}{
		{1.2345678901234567, -1, -1, "       1.2345678901234567E+00"},
		{1.554, 15, -1, " 1.55400000E+00"},
		{1.2345678901234567, 8, -1, " 1.2E+00"},
		{1.2345678901234567, 1, -1, " 1.2E+00"},
		{-5.565, 15, -1, "-5.56500000E+00"},
		{9.99, 7, -1, " 1.0E+01"},
		{1.25, 1, 2, "1.25"},
	}
	for _, tc := range cases {
		var b bytes.Buffer
		w := Text{out: &b}
		w.WriteReal(tc.x, tc.width, tc.frac)
		if b.String() != tc.want {
			t.Errorf("WriteReal(%v, %d, %d) = %q want %q", tc.x, tc.width, tc.frac, b.String(), tc.want)
		}
	}
}

func TestLineFlush(t *testing.T) {
	var b bytes.Buffer
	w := Text{out: bufio.NewWriter(&b)}
	w.WriteStr("hi", -1)
	if b.Len() != 0 {
		t.Fatalf("visible before newline: %q", b.String())
	}
	w.Ln()
	if b.String() != "hi\n" {
		t.Fatalf("after newline %q", b.String())
	}
	w.WriteChar('z', -1)
	w.Flush()
	if b.String() != "hi\nz" {
		t.Fatalf("after flush %q", b.String())
	}
}

func TestReadCharBuffer(t *testing.T) {
	defer Cleanup()
	f := textFrom("4.0")
	if c := f.ReadChar(); c != '4' || f.Buf != '.' {
		t.Fatalf("after 4: char %q buffer %q", c, f.Buf)
	}
	if c := f.ReadChar(); c != '.' || f.Buf != '0' {
		t.Fatalf("after dot: char %q buffer %q", c, f.Buf)
	}
	lines := textFrom("a\nb")
	if c := lines.ReadChar(); c != 'a' || lines.Buf != ' ' || !lines.Eoln() {
		t.Fatalf("after a: char %q buffer %q eoln %v", c, lines.Buf, lines.Eoln())
	}
	if c := lines.ReadChar(); c != ' ' || lines.Buf != 'b' || lines.Eoln() {
		t.Fatalf("after end of line: char %q buffer %q eoln %v", c, lines.Buf, lines.Eoln())
	}
}

func TestReadChunk(t *testing.T) {
	defer Cleanup()
	f := textFrom("A")
	if c := f.ReadChar(); c != 'A' {
		t.Fatalf("char %q", c)
	}
	g := textFrom("abcdef")
	var b strings.Builder
	for !g.Eof() {
		if g.Eoln() {
			g.ReadChar()
			continue
		}
		b.WriteByte(g.ReadChar())
	}
	if b.String() != "abcdef" {
		t.Fatalf("%q", b.String())
	}
}

func textFrom(s string) Text {
	f, err := os.CreateTemp("", "gopas-test")
	if err != nil {
		panic(err)
	}
	if _, err := f.WriteString(s); err != nil {
		panic(err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		panic(err)
	}
	st := &store{f: f, own: true, tmp: true}
	remember(st)
	return Text{store: st, br: bufio.NewReader(f)}
}

func TestTextFile(t *testing.T) {
	defer Cleanup()
	var f Text
	f.Rewrite()
	f.WriteInt(42, -1)
	f.Ln()
	f.Reset()
	if f.Eoln() {
		t.Fatal("eoln at the first digit")
	}
	var n int
	got := 0
	for !f.Eoln() {
		c := f.ReadChar()
		got++
		if c != ' ' && (c < '0' || c > '9') {
			t.Fatalf("char %q", c)
		}
	}
	if got != 11 {
		t.Fatalf("line length %d", got)
	}
	f.Reset()
	if n = int(f.ReadInt()); n != 42 {
		t.Fatalf("read %d", n)
	}
}

func TestTextPut(t *testing.T) {
	defer Cleanup()
	var f Text
	f.Rewrite()
	f.Buf = 'G'
	f.Put()
	f.Buf = 'H'
	f.Put()
	f.Reset()
	if c := f.ReadChar(); c != 'G' {
		t.Fatalf("first %q", c)
	}
	if c := f.ReadChar(); c != 'H' {
		t.Fatalf("second %q", c)
	}
	f.Reset()
	defer func() {
		if recover() == nil {
			t.Fatal("put when not eof did not stop")
		}
	}()
	f.Put()
}

func TestTextLines(t *testing.T) {
	defer Cleanup()
	var f Text
	f.Rewrite()
	f.WriteStr("how now", -1)
	f.Ln()
	f.WriteStr("brown cow", -1)
	f.Ln()
	f.Reset()
	var b strings.Builder
	for !f.Eof() {
		if f.Eoln() {
			b.WriteString("<eoln>")
		}
		b.WriteByte(f.ReadChar())
	}
	if b.String() != "how now<eoln> brown cow<eoln> " {
		t.Fatalf("%q", b.String())
	}
}

func TestInputEnds(t *testing.T) {
	defer Cleanup()
	f := textFrom("Test line 1\n4567\n-1.5e-10\n")
	var b strings.Builder
	for !f.Eoln() {
		b.WriteByte(f.ReadChar())
	}
	f.ReadLn()
	if b.String() != "Test line 1" {
		t.Fatalf("line %q", b.String())
	}
	if n := f.ReadInt(); n != 4567 {
		t.Fatalf("int %d", n)
	}
	f.ReadLn()
	if x := f.ReadReal(); x != -1.5e-10 {
		t.Fatalf("real %g", x)
	}
	f.ReadLn()
	if !f.Eof() {
		t.Fatal("expected end of file")
	}
	empty := textFrom("")
	if !empty.Eoln() || !empty.Eof() {
		t.Fatal("empty input should end the read loop")
	}
}

func TestMod(t *testing.T) {
	if got := Mod(7, 5); got != 2 {
		t.Fatalf("7 mod 5 = %d", got)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("mod by a negative divisor did not stop")
		}
	}()
	Mod(1, -1)
}

func TestStepBounds(t *testing.T) {
	if got := Step(1, -MaxInt, MaxInt, 1, true); got != 2 {
		t.Fatalf("succ(1) = %d", got)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("succ(maxint) did not stop")
		}
	}()
	Step(MaxInt, -MaxInt, MaxInt, 1, true)
}

func TestStepBelow(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("pred(-maxint) did not stop")
		}
	}()
	Step(-MaxInt, -MaxInt, MaxInt, -1, true)
}

func TestSetFit(t *testing.T) {
	ok := Incl(Incl(Bits{}, 1), 2)
	SetFit(ok, 1, 5)
	defer func() {
		if recover() == nil {
			t.Fatal("element 6 did not stop")
		}
	}()
	SetFit(Incl(ok, 6), 1, 5)
}

func TestPackCheck(t *testing.T) {
	src := []int{1, 2, 3, 4}
	dst := []int{0, 0}
	Pack(dst, src, 3, false)
	if dst[0] != 4 || dst[1] != 0 {
		t.Fatalf("partial pack: %v", dst)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("pack past the end did not stop")
		}
	}()
	Pack(dst, src, 3, true)
}

func TestFileRoundTrip(t *testing.T) {
	defer Cleanup()
	var f File[int]
	f.Rewrite()
	for _, n := range []int{1, 2, 3} {
		f.Write(n)
	}
	f.Reset()
	var got []int
	for !f.Eof() {
		got = append(got, f.Read())
	}
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("%v", got)
	}
}

func TestFileRecord(t *testing.T) {
	defer Cleanup()
	type rec struct {
		n int
		b bool
	}
	var f File[rec]
	f.Rewrite()
	f.Write(rec{n: 7, b: true})
	f.Write(rec{n: 8, b: false})
	f.Reset()
	first := f.Read()
	second := f.Read()
	if first.n != 7 || !first.b || second.n != 8 || second.b || !f.Eof() {
		t.Fatalf("%+v %+v eof %v", first, second, f.Eof())
	}
}
