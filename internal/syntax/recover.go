package syntax

import (
	"errors"
	"sync"
)

// active is the parser inside parse. Parses do not overlap.
var (
	parseMu sync.Mutex
	active  *parser
)

// WithRecover turns the misspelling rules on or off. They are on when the
// option is omitted. A second parse with them off is used when the first
// parse attached the error to a name that is already declared.
func WithRecover(on bool) Option {
	return func(p *parser) Option {
		old, _ := p.cur.globalStore["recover"].(bool)
		_, had := p.cur.globalStore["recover"]
		p.cur.globalStore["recover"] = on
		if !had {
			return func(p *parser) Option {
				delete(p.cur.globalStore, "recover")
				return nil
			}
		}
		return WithRecover(old)
	}
}

// notKeyword is true when the text ahead is an ordinary name. A keyword is
// rejected without scanning it, so the failure stays on the keyword itself.
// joinedWord reports a number that runs straight into a word, as in 42div.
func joinedWord() {
	p := active
	if p == nil || p.pt.offset >= len(p.data) || !isLetter(p.data[p.pt.offset]) {
		return
	}
	p.addErrAt(errors.New("number and word are not separated"), p.pt.position, nil)
}

func notKeyword() bool {
	p := active
	if p == nil {
		return true
	}
	word, _ := identAt(p.data, p.pt.offset)
	if word == "" {
		return true
	}
	return !keyword(word)
}

// variantLabel is true when a constant list is followed by ':' and '('.
// A field after the variant part (name ':' type) is not another arm.
func variantLabel() bool {
	p := active
	if p == nil {
		return false
	}
	i := skipSpace(p.data, p.pt.offset)
	for {
		j, ok := constAt(p.data, i)
		if !ok {
			return false
		}
		i = skipSpace(p.data, j)
		if i < len(p.data) && p.data[i] == ',' {
			i = skipSpace(p.data, i+1)
			continue
		}
		break
	}
	if i >= len(p.data) || p.data[i] != ':' || i+1 < len(p.data) && p.data[i+1] == '=' {
		return false
	}
	i = skipSpace(p.data, i+1)
	return i < len(p.data) && p.data[i] == '('
}

// emptyQuotes is a string of no characters: '' not followed by another quote.
// ''' is one quote character, and 'it''s' is a longer string.
func emptyQuotes() bool {
	p := active
	if p == nil {
		return false
	}
	i := p.pt.offset
	if i+1 >= len(p.data) || p.data[i] != '\'' || p.data[i+1] != '\'' {
		return false
	}
	return i+2 >= len(p.data) || p.data[i+2] != '\''
}

func constAt(data []byte, i int) (int, bool) {
	i = skipSpace(data, i)
	if i >= len(data) {
		return i, false
	}
	if data[i] == '+' || data[i] == '-' {
		if j, ok := numberAt(data, skipSpace(data, i+1)); ok {
			return j, true
		}
		return nameAt(data, skipSpace(data, i+1))
	}
	if data[i] == '\'' {
		return stringAt(data, i)
	}
	if j, ok := numberAt(data, i); ok {
		return j, true
	}
	return nameAt(data, i)
}

func nameAt(data []byte, i int) (int, bool) {
	word, j := identAt(data, i)
	if word == "" || keyword(word) {
		return i, false
	}
	return j, true
}

func numberAt(data []byte, i int) (int, bool) {
	if i >= len(data) || data[i] < '0' || data[i] > '9' {
		return i, false
	}
	j := i + 1
	for j < len(data) && data[j] >= '0' && data[j] <= '9' {
		j++
	}
	if j+1 < len(data) && data[j] == '.' && data[j+1] >= '0' && data[j+1] <= '9' {
		j += 2
		for j < len(data) && data[j] >= '0' && data[j] <= '9' {
			j++
		}
	}
	if j < len(data) && (data[j] == 'e' || data[j] == 'E') {
		k := j + 1
		if k < len(data) && (data[k] == '+' || data[k] == '-') {
			k++
		}
		if k < len(data) && data[k] >= '0' && data[k] <= '9' {
			j = k + 1
			for j < len(data) && data[j] >= '0' && data[j] <= '9' {
				j++
			}
		}
	}
	return j, true
}

func stringAt(data []byte, i int) (int, bool) {
	if i >= len(data) || data[i] != '\'' {
		return i, false
	}
	j := i + 1
	for j < len(data) {
		if data[j] == '\n' || data[j] == '\r' {
			return i, false
		}
		if data[j] == '\'' {
			if j+1 < len(data) && data[j+1] == '\'' {
				j += 2
				continue
			}
			return j + 1, true
		}
		j++
	}
	return i, false
}

func recovering(c *current) bool {
	v, ok := c.globalStore["recover"].(bool)
	if !ok {
		return true
	}
	return v
}

func badStmtStart() bool {
	p := active
	if p == nil {
		return false
	}
	i := skipSpace(p.data, p.pt.offset)
	word, j := identAt(p.data, i)
	if word == "" || keyword(word) {
		return false
	}
	j = skipSpace(p.data, j)
	if j >= len(p.data) || p.data[j] == ';' || p.data[j] == '(' {
		return false
	}
	if j+1 < len(p.data) && p.data[j] == ':' && p.data[j+1] == '=' {
		return false
	}
	if startsWord(p.data, j, "end") || startsWord(p.data, j, "else") || startsWord(p.data, j, "until") {
		return false
	}
	return true
}

func twoPlainIdents() bool {
	p := active
	if p == nil {
		return false
	}
	word, j := identAt(p.data, skipSpace(p.data, p.pt.offset))
	if word == "" || keyword(word) {
		return false
	}
	word, j = identAt(p.data, skipSpace(p.data, j))
	if word == "" || keyword(word) {
		return false
	}
	return stmtBoundary(p.data, j)
}

func stmtBoundary(data []byte, i int) bool {
	i = skipSpace(data, i)
	if i >= len(data) {
		return false
	}
	if data[i] == ';' || data[i] == '.' {
		return true
	}
	return startsWord(data, i, "end") || startsWord(data, i, "else") || startsWord(data, i, "until")
}

func badTypeTail() bool {
	p := active
	if p == nil {
		return false
	}
	i := skipSpace(p.data, p.pt.offset)
	if word, j := identAt(p.data, i); word != "" {
		if keyword(word) {
			return true
		}
		j = skipSpace(p.data, j)
		return j >= len(p.data) || p.data[j] != ':'
	}
	return false
}

func atFinalEnd() bool {
	p := active
	if p == nil {
		return false
	}
	i := skipSpace(p.data, p.pt.offset)
	if !startsWord(p.data, i, "end") {
		return false
	}
	i = skipSpace(p.data, i+3)
	return i < len(p.data) && p.data[i] == '.'
}

func nextIsDot() bool {
	p := active
	if p == nil {
		return false
	}
	i := skipSpace(p.data, p.pt.offset)
	if i >= len(p.data) || p.data[i] != '.' {
		return false
	}
	return skipSpace(p.data, i+1) >= len(p.data)
}

func skipSpace(data []byte, i int) int {
	for i < len(data) {
		switch data[i] {
		case ' ', '\t', '\r', '\n':
			i++
		case '{':
			j := i + 1
			for j < len(data) && data[j] != '}' {
				j++
			}
			if j >= len(data) {
				return i
			}
			i = j + 1
		case '(':
			if i+1 < len(data) && data[i+1] == '*' {
				j := i + 2
				for j+1 < len(data) && !(data[j] == '*' && data[j+1] == ')') {
					j++
				}
				if j+1 >= len(data) {
					return i
				}
				i = j + 2
				continue
			}
			return i
		default:
			return i
		}
	}
	return i
}

func identAt(data []byte, i int) (string, int) {
	if i >= len(data) || !isLetter(data[i]) {
		return "", i
	}
	j := i + 1
	for j < len(data) && isIdent(data[j]) {
		j++
	}
	return string(lower(data[i:j])), j
}

func startsWord(data []byte, i int, word string) bool {
	if i+len(word) > len(data) {
		return false
	}
	for k := 0; k < len(word); k++ {
		if lowerByte(data[i+k]) != word[k] {
			return false
		}
	}
	return i+len(word) == len(data) || !isIdent(data[i+len(word)])
}

func isLetter(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}

func isIdent(b byte) bool {
	return isLetter(b) || b >= '0' && b <= '9'
}

func lower(b []byte) []byte {
	out := make([]byte, len(b))
	for i, c := range b {
		out[i] = lowerByte(c)
	}
	return out
}

func lowerByte(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b - 'A' + 'a'
	}
	return b
}

func keyword(word string) bool {
	switch word {
	case "and", "array", "begin", "case", "const", "div", "do", "downto", "else", "end",
		"file", "for", "function", "goto", "if", "in", "label", "mod", "nil", "not",
		"of", "or", "packed", "procedure", "program", "record", "repeat", "set",
		"then", "to", "type", "until", "var", "while", "with":
		return true
	default:
		return false
	}
}
