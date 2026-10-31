//go:build ignore

package main

import (
	"os"
	"strings"
)

func main() {
	const (
		path = "parser.go"
		old  = "func (p *parser) parse(g *grammar) (val any, err error) {\n"
		new  = "func (p *parser) parse(g *grammar) (val any, err error) {\n\tparseMu.Lock()\n\tdefer parseMu.Unlock()\n\tactive = p\n\tdefer func() { active = nil }()\n"
	)
	b, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	s := string(b)
	if strings.Contains(s, "active = p") {
		return
	}
	if !strings.Contains(s, old) {
		panic("parse function not found")
	}
	if err := os.WriteFile(path, []byte(strings.Replace(s, old, new, 1)), 0o644); err != nil {
		panic(err)
	}
}
