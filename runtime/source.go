package runtime

import _ "embed"

// Part selects which pieces of the runtime a translated program needs.
// Each piece carries only the standard-library imports it uses.
type Part uint8

const (
	PartBase Part = 1 << iota // sets, integer checks, string order, pack, chr, succ
	PartMem                   // conformant arrays
	PartReal                  // real functions
	PartText                  // text files
	PartFile                  // typed files; includes the text piece
)

//go:embed base.go
var baseSrc string

//go:embed mem.go
var memSrc string

//go:embed real.go
var realSrc string

//go:embed text.go
var textSrc string

//go:embed file.go
var fileSrc string

func sources(part Part) (out []string, cleanup bool) {
	if part&PartFile != 0 {
		part |= PartText
	}
	if part&PartBase != 0 {
		out = append(out, baseSrc)
	}
	if part&PartMem != 0 {
		out = append(out, memSrc)
	}
	if part&PartReal != 0 {
		out = append(out, realSrc)
	}
	if part&PartText != 0 {
		out = append(out, textSrc)
		cleanup = true
	}
	if part&PartFile != 0 {
		out = append(out, fileSrc)
	}
	return out, cleanup
}
