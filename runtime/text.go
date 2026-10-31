package runtime

import (
	"bufio"
	"io"
	"os"
	"strconv"
	"strings"
)

// store is the disk file behind a Pascal file. A local file is temporary and
// removed by Cleanup. out is set only for a text file that writes to an
// external writer, such as standard output.
type store struct {
	f    *os.File
	bw   *bufio.Writer
	tmp  bool
	own  bool
	done bool
}

func remember(s *store) {
	live = append(live, s)
}

// openStore returns s connected to its disk file. trunc replaces the contents.
// An empty name creates a temporary file. foreign is the panic used when s
// already refers to a file this package does not own, such as standard input.
func openStore(s *store, name string, trunc bool, foreign string) *store {
	if s != nil && s.f != nil {
		if !s.own && foreign != "" {
			panic(foreign)
		}
		if trunc {
			if err := s.f.Truncate(0); err != nil {
				panic(err)
			}
			if _, err := s.f.Seek(0, io.SeekStart); err != nil {
				panic(err)
			}
		}
		s.bw = bufio.NewWriter(s.f)
		return s
	}
	var (
		f   *os.File
		err error
		tmp bool
	)
	if name != "" {
		flag := os.O_RDWR
		if trunc {
			flag |= os.O_CREATE | os.O_TRUNC
		}
		f, err = os.OpenFile(name, flag, 0o644)
	} else {
		f, err = os.CreateTemp("", "gopas-")
		tmp = true
	}
	if err != nil {
		panic(err)
	}
	s = &store{f: f, bw: bufio.NewWriter(f), tmp: tmp, own: true}
	remember(s)
	return s
}

var live []*store

// Cleanup flushes and closes every disk file opened by this package.
func Cleanup() {
	for _, s := range live {
		s.close()
	}
	live = nil
}

func (s *store) close() {
	if s == nil || s.done {
		return
	}
	s.done = true
	if s.bw != nil {
		s.bw.Flush()
		s.bw = nil
	}
	if s.f != nil && s.own {
		name := s.f.Name()
		s.f.Close()
		if s.tmp {
			os.Remove(name)
		}
	}
}

// Text is a Pascal text file. Output is standard output and Input is standard input.
// Any other text file is a disk file. Rewrite truncates it and Reset reads it
// from the first byte. Reads and writes go through a buffer.
type Text struct {
	Buf      byte
	name     string
	out      io.Writer
	store    *store
	br       *bufio.Reader
	eof      bool
	gen      bool
	ready    bool
	line     bool
	implicit bool
	past     bool
	saw      bool
	endedNL  bool
}

// Output is the program parameter output. Writes are buffered and flushed
// by Ln, Page, and Flush.
func Output() Text { return Text{out: bufio.NewWriter(os.Stdout)} }

// Input is the program parameter input.
func Input() Text {
	t := Text{store: &store{f: os.Stdin}}
	t.br = bufio.NewReader(os.Stdin)
	return t
}

// Bind records the external name of a program parameter.
func (t *Text) Bind(name string) { t.name = name }

// Rewrite empties the file and leaves it ready to write.
// A file connected to an external writer, such as output, stays there.
func (t *Text) Rewrite() {
	t.Buf = 0
	t.eof = true
	t.ready = false
	t.line = false
	t.implicit = false
	t.past = false
	t.saw = false
	t.endedNL = false
	t.br = nil
	if t.out != nil {
		t.gen = false
		return
	}
	t.open(true)
	t.gen = true
}

// Reset reads the file from the first character.
func (t *Text) Reset() {
	t.ready = false
	t.implicit = false
	t.past = false
	t.line = false
	t.saw = false
	t.endedNL = false
	t.Buf = 0
	t.gen = false
	if t.out != nil {
		t.eof = false
		return
	}
	if t.store == nil {
		if t.name == "" {
			t.eof = true
			t.ready = true
			return
		}
		t.open(false)
	}
	if t.store.bw != nil {
		t.store.bw.Flush()
		t.store.bw = nil
	}
	if _, err := t.store.f.Seek(0, io.SeekStart); err != nil {
		panic(err)
	}
	t.br = bufio.NewReader(t.store.f)
	t.eof = false
	t.prepare()
}

// open connects t to its disk file. trunc replaces the contents.
func (t *Text) open(trunc bool) {
	t.store = openStore(t.store, t.name, trunc, "rewrite")
}

// Put appends the buffer character. eof must be true.
func (t *Text) Put() {
	if !t.eof {
		panic("put when not eof")
	}
	t.write(string([]byte{t.Buf}))
}

// Get advances to the next character. The buffer variable then holds it,
// or a space when the file is at the end of a line.
func (t *Text) Get() {
	t.prepare()
	if t.eof {
		panic("get past eof")
	}
	t.advance()
	t.prepare()
}

// prepare loads the buffer variable. A line marker is presented as a space.
// A file that does not end with a line marker has one implied line ending
// before end of file.
func (t *Text) prepare() {
	if t.ready || t.gen {
		return
	}
	t.ready = true
	if t.br == nil {
		t.eof = true
		t.line = false
		t.Buf = 0
		return
	}
	b, err := t.br.ReadByte()
	if err != nil {
		if t.saw && !t.endedNL && !t.past {
			t.eof = false
			t.line = true
			t.Buf = ' '
			t.implicit = true
			return
		}
		t.eof = true
		t.line = false
		t.Buf = 0
		t.implicit = false
		return
	}
	t.saw = true
	t.endedNL = b == '\n'
	t.eof = false
	t.implicit = false
	if b == '\n' {
		t.line = true
		t.Buf = ' '
		return
	}
	t.line = false
	t.Buf = b
}

func (t *Text) advance() {
	if t.implicit {
		t.implicit = false
		t.past = true
	}
	t.ready = false
}

func (t *Text) write(s string) {
	if t.out != nil {
		io.WriteString(t.out, s)
		return
	}
	if t.store == nil || t.store.bw == nil {
		t.open(t.store == nil)
	}
	t.store.bw.WriteString(s)
	t.gen = true
	t.eof = true
}

// Ln ends the current line. An external writer is flushed so the line appears.
func (t *Text) Ln() {
	t.write("\n")
	if t.out != nil {
		t.Flush()
	}
}

// Page writes a form feed. An external writer is flushed.
func (t *Text) Page() {
	t.write("\f")
	if t.out != nil {
		t.Flush()
	}
}

// Flush writes any buffered output.
func (t *Text) Flush() {
	if f, ok := t.out.(interface{ Flush() error }); ok {
		f.Flush()
	}
	if t.store != nil && t.store.bw != nil {
		t.store.bw.Flush()
	}
}

// Eof reports whether the text file is exhausted.
func (t *Text) Eof() bool {
	if !t.gen {
		t.prepare()
	}
	return t.eof
}

// Eoln reports whether the file is at the end of a line.
// End of file is also end of line, so a read loop stops instead of waiting.
func (t *Text) Eoln() bool {
	if t.gen {
		return t.eof
	}
	t.prepare()
	return t.eof || t.line
}

// WriteInt writes n in a field of width columns. Width 0 or less selects the
// default of 11. A value longer than width is written in full.
func (t *Text) WriteInt(n int, width int) {
	if width < 0 {
		width = 11
	}
	t.write(pad(strconv.FormatInt(int64(n), 10), width, false))
}

// WriteReal writes x. A negative frac selects scientific notation and a
// default width of 29. A non-negative frac selects fixed point with that
// many digits after the point.
//
// Scientific form uses 17 significant digits. When a width is given and that
// form is longer, fewer digits are used so the text fits, down to one digit
// on each side of the point. A width too small for that shortest form is
// ignored. Fixed point keeps the requested fraction digits and grows the
// field when the integer part needs more room.
func (t *Text) WriteReal(x float64, width, frac int) {
	if frac >= 0 {
		s := strconv.FormatFloat(x, 'f', frac, 64)
		if width < 0 {
			width = len(s)
		}
		t.write(pad(s, width, false))
		return
	}
	if width < 0 {
		width = 29
	}
	t.write(pad(fitScientific(x, width), width, false))
}

// fitScientific prints x in floating form. A non-negative value begins with
// a blank where a minus sign would be. At most 17 significant digits are
// used, and fewer when that is what fits in width. A width too small for
// one digit on each side of the point is ignored.
func fitScientific(x float64, width int) string {
	const full = 16
	sign := byte(' ')
	ax := x
	if x < 0 {
		sign = '-'
		ax = -x
	}
	best := signedSci(ax, 1, sign)
	for prec := 2; prec <= full; prec++ {
		s := signedSci(ax, prec, sign)
		if len(s) > width {
			break
		}
		best = s
	}
	return best
}

func signedSci(ax float64, prec int, sign byte) string {
	return string(sign) + strconv.FormatFloat(ax, 'E', prec, 64)
}

// WriteBool writes TRUE or FALSE. Width 0 or less selects the default of 5.
// The field is exactly width columns, padded on the left or cut on the right.
func (t *Text) WriteBool(b bool, width int) {
	s := "FALSE"
	if b {
		s = "TRUE"
	}
	if width < 0 {
		width = 5
	}
	t.write(pad(s, width, true))
}

// WriteChar writes c. Width 0 or less selects the default of 1.
// The field is exactly width columns.
func (t *Text) WriteChar(c byte, width int) {
	if width < 0 {
		width = 1
	}
	t.write(pad(string([]byte{c}), width, true))
}

// WriteStr writes s. Width 0 or less selects the length of s.
// The field is exactly width columns.
func (t *Text) WriteStr(s string, width int) {
	if width < 0 {
		width = len(s)
	}
	t.write(pad(s, width, true))
}

func pad(s string, width int, exact bool) string {
	if width < 0 {
		return s
	}
	if len(s) >= width {
		if exact {
			return s[:width]
		}
		return s
	}
	return strings.Repeat(" ", width-len(s)) + s
}

// ReadChar returns the current character and advances.
// At the end of a line the character is a space, not the line marker.
func (t *Text) ReadChar() byte {
	t.prepare()
	if t.eof {
		panic("read past eof")
	}
	c := t.Buf
	t.advance()
	return c
}

// ReadInt reads an integer, skipping leading spaces and line breaks.
func (t *Text) ReadInt() int {
	t.skipBlank()
	t.prepare()
	if t.eof {
		return 0
	}
	sign := 1
	if t.Buf == '+' || t.Buf == '-' {
		if t.Buf == '-' {
			sign = -1
		}
		t.advance()
		t.prepare()
	}
	var n int
	digits := false
	for !t.eof && t.Buf >= '0' && t.Buf <= '9' {
		digits = true
		n = n*10 + int(t.Buf-'0')
		t.advance()
		t.prepare()
	}
	if !digits {
		return 0
	}
	return sign * n
}

// ReadReal reads a real, skipping leading spaces and line breaks.
func (t *Text) ReadReal() float64 {
	t.skipBlank()
	t.prepare()
	if t.eof {
		return 0
	}
	var b strings.Builder
	if t.Buf == '+' || t.Buf == '-' {
		b.WriteByte(t.Buf)
		t.advance()
		t.prepare()
	}
	sawDigit := false
	sawDot := false
	for !t.eof {
		c := t.Buf
		switch {
		case c >= '0' && c <= '9':
			sawDigit = true
		case c == '.' && !sawDot:
			sawDot = true
		case (c == 'e' || c == 'E') && sawDigit:
			b.WriteByte(c)
			t.advance()
			t.prepare()
			if !t.eof && (t.Buf == '+' || t.Buf == '-') {
				b.WriteByte(t.Buf)
				t.advance()
				t.prepare()
			}
			for !t.eof && t.Buf >= '0' && t.Buf <= '9' {
				b.WriteByte(t.Buf)
				t.advance()
				t.prepare()
			}
			goto parsed
		default:
			goto parsed
		}
		b.WriteByte(c)
		t.advance()
		t.prepare()
	}
parsed:
	if !sawDigit {
		return 0
	}
	x, err := strconv.ParseFloat(b.String(), 64)
	if err != nil {
		return 0
	}
	return x
}

func (t *Text) skipBlank() {
	for {
		t.prepare()
		if t.eof || (!t.line && t.Buf != ' ' && t.Buf != '\t') {
			return
		}
		t.advance()
	}
}

// ReadLn skips the rest of the current line, including its line marker.
func (t *Text) ReadLn() {
	if t.gen {
		return
	}
	for {
		t.prepare()
		if t.eof {
			return
		}
		marker := t.line
		t.advance()
		if marker {
			return
		}
	}
}
