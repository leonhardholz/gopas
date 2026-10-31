package runtime

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/printer"
	"go/token"
	"go/types"
	"strings"
)

// Bundle copies the selected pieces of this package into prog, which must be
// a package-main program. Exported runtime names gain an rt prefix, so rtText
// is this package's Text. prog calls those names directly and does not import
// gopas/runtime.
func Bundle(prog string, part Part) (string, error) {
	srcs, cleanup := sources(part)
	if len(srcs) == 0 {
		return prog, nil
	}
	fset := token.NewFileSet()
	user, err := parser.ParseFile(fset, "main.go", prog, parser.SkipObjectResolution)
	if err != nil {
		return "", err
	}
	if len(user.Imports) > 0 {
		return "", fmt.Errorf("generated program imports %s", user.Imports[0].Path.Value)
	}
	files := make([]*ast.File, 0, len(srcs))
	for i, src := range srcs {
		f, err := parser.ParseFile(fset, fmt.Sprintf("rt%d.go", i), src, parser.SkipObjectResolution)
		if err != nil {
			return "", err
		}
		files = append(files, f)
	}
	if err := prefixExported(fset, files); err != nil {
		return "", err
	}
	rt := merge(files)
	rt.Decls = append(rt.Decls, user.Decls...)
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, rt); err != nil {
		return "", err
	}
	out := buf.String()
	if cleanup {
		out = injectCleanup(out)
	}
	return out, nil
}

// merge combines the runtime files into one package-main file.
// Imports used by more than one piece appear once.
func merge(files []*ast.File) *ast.File {
	out := &ast.File{Name: ast.NewIdent("main")}
	seen := map[string]bool{}
	var specs []ast.Spec
	var decls []ast.Decl
	for _, f := range files {
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.IMPORT {
				decls = append(decls, d)
				continue
			}
			for _, s := range gd.Specs {
				im := s.(*ast.ImportSpec)
				if seen[im.Path.Value] {
					continue
				}
				seen[im.Path.Value] = true
				specs = append(specs, im)
			}
		}
	}
	if len(specs) > 0 {
		out.Decls = append(out.Decls, &ast.GenDecl{
			Tok:    token.IMPORT,
			Lparen: specs[0].Pos(),
			Specs:  specs,
			Rparen: specs[len(specs)-1].End(),
		})
	}
	out.Decls = append(out.Decls, decls...)
	return out
}

// injectCleanup removes temporary files when main returns.
func injectCleanup(src string) string {
	i := strings.Index(src, "func main()")
	if i < 0 {
		return src
	}
	j := strings.Index(src[i:], "{")
	if j < 0 {
		return src
	}
	j += i + 1
	return src[:j] + "\n\tdefer rtCleanup()\n" + src[j:]
}

// prefixExported renames exported package-level declarations to rtName and
// updates every use of those declarations. Methods keep their names.
func prefixExported(fset *token.FileSet, files []*ast.File) error {
	info := &types.Info{
		Defs: make(map[*ast.Ident]types.Object),
		Uses: make(map[*ast.Ident]types.Object),
	}
	pkg, err := (&types.Config{Importer: importer.Default()}).Check("runtime", fset, files, info)
	if err != nil {
		return err
	}
	names := map[types.Object]string{}
	for _, obj := range info.Defs {
		if obj == nil || obj.Parent() != pkg.Scope() || !obj.Exported() {
			continue
		}
		if fn, ok := obj.(*types.Func); ok {
			// Type, not Signature: Signature arrived in Go 1.23.
			sig, _ := fn.Type().(*types.Signature)
			if sig != nil && sig.Recv() != nil {
				continue
			}
		}
		names[obj] = "rt" + obj.Name()
	}
	rename := func(id *ast.Ident, obj types.Object) {
		if neu, ok := names[obj]; ok {
			id.Name = neu
		}
	}
	for id, obj := range info.Defs {
		rename(id, obj)
	}
	for id, obj := range info.Uses {
		rename(id, obj)
	}
	return nil
}
