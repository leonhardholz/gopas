package ast

import (
	"reflect"
	"sync"
)

// Pos is where a construct starts in the source.
// Line and Col are 1-based. The zero value means the position is unknown.
type Pos struct {
	Line int
	Col  int
}

var (
	posMu sync.Mutex
	posOf = map[any]Pos{}
)

// SetPos records the start of n. n must be a pointer; values are ignored
// so that a later lookup uses the same pointer the tree holds.
func SetPos(n any, line, col int) {
	if n == nil || line == 0 || reflect.TypeOf(n).Kind() != reflect.Pointer {
		return
	}
	posMu.Lock()
	posOf[n] = Pos{Line: line, Col: col}
	posMu.Unlock()
}

// At returns the position recorded for n, or the zero Pos.
func At(n any) Pos {
	if n == nil {
		return Pos{}
	}
	posMu.Lock()
	defer posMu.Unlock()
	return posOf[n]
}

// spotKey keeps a second position for the same node, used when the node
// starts at a keyword and the reported name is further along the line.
type spotKey struct{ n any }

// SetNamePos records where the declared name starts. n is the routine node.
func SetNamePos(n any, line, col int) {
	if n == nil || line == 0 || reflect.TypeOf(n).Kind() != reflect.Pointer {
		return
	}
	posMu.Lock()
	posOf[spotKey{n}] = Pos{Line: line, Col: col}
	posMu.Unlock()
}

// NamePos returns the position recorded by SetNamePos.
func NamePos(n any) Pos {
	if n == nil {
		return Pos{}
	}
	posMu.Lock()
	defer posMu.Unlock()
	return posOf[spotKey{n}]
}
