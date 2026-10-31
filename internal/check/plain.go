package check

import (
	"reflect"

	"gopas/internal/ast"
)

// PreferPlain reports that the parse used a misspelling rule on a name that
// is already declared, or supplied a missing end without an undeclared name.
// The program should be parsed again with that recovery off.
func PreferPlain(p *ast.Program, errs []Error) bool {
	if p == nil || len(errs) == 0 {
		return false
	}
	for _, e := range errs {
		if e.Code == ErrNotDeclared {
			return false
		}
	}
	return p.MissingEnd != nil || hasRecovery(p)
}

func hasRecovery(p *ast.Program) bool {
	seen := map[uintptr]bool{}
	var walk func(reflect.Value) bool
	walk = func(v reflect.Value) bool {
		if !v.IsValid() {
			return false
		}
		switch v.Kind() {
		case reflect.Pointer:
			if v.IsNil() {
				return false
			}
			ptr := v.Pointer()
			if seen[ptr] {
				return false
			}
			seen[ptr] = true
			switch n := v.Interface().(type) {
			case *ast.Loose:
				return true
			case *ast.TypeName:
				if n.Skip {
					return true
				}
			}
			return walk(v.Elem())
		case reflect.Interface:
			if v.IsNil() {
				return false
			}
			return walk(v.Elem())
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				if v.Type().Field(i).IsExported() && walk(v.Field(i)) {
					return true
				}
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < v.Len(); i++ {
				if walk(v.Index(i)) {
					return true
				}
			}
		}
		return false
	}
	return walk(reflect.ValueOf(p))
}
