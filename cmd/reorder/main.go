// Program reorder reorganizes Go source so that: consts are at the top (after
// package and imports), then top-level type definitions, then vars, then
// functions with main() last when present. Output is printed in gofmt style.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
)

func main() {
	write := flag.Bool("w", false, "write result to (source) file instead of stdout")
	doFmt := flag.Bool("fmt", false, "run go fmt on the file after rewriting (only with -w)")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: reorder [-w] [-fmt] <file.go> [file.go ...]")
		os.Exit(1)
	}

	var failed int
	for _, path := range args {
		if err := process(path, *write, *doFmt); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			failed++
		}
	}
	if failed > 0 {
		os.Exit(1)
	}
}

func process(path string, write, doFmt bool) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return err
	}

	reorderFile(f)

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return fmt.Errorf("format: %w", err)
	}
	out := buf.Bytes()

	if write {
		if err := os.WriteFile(path, out, 0644); err != nil {
			return err
		}
		if doFmt {
			cmd := exec.Command("go", "fmt", path)
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("go fmt: %w", err)
			}
		}
	} else {
		os.Stdout.Write(out)
	}
	return nil
}

func reorderFile(f *ast.File) {
	var imports, consts, types, vars, funcs []ast.Decl
	var mainFunc ast.Decl

	for _, d := range f.Decls {
		switch x := d.(type) {
		case *ast.GenDecl:
			switch x.Tok {
			case token.IMPORT:
				imports = append(imports, d)
			case token.CONST:
				consts = append(consts, d)
			case token.TYPE:
				types = append(types, d)
			case token.VAR:
				vars = append(vars, d)
			default:
				// leave unknown GenDecl in a sensible place (e.g. after vars)
				vars = append(vars, d)
			}
		case *ast.FuncDecl:
			if isMain(x) {
				mainFunc = d
			} else {
				funcs = append(funcs, d)
			}
		default:
			funcs = append(funcs, d)
		}
	}

	newDecls := make([]ast.Decl, 0, len(f.Decls))
	newDecls = append(newDecls, imports...)
	newDecls = append(newDecls, consts...)
	newDecls = append(newDecls, types...)
	newDecls = append(newDecls, vars...)
	newDecls = append(newDecls, funcs...)
	if mainFunc != nil {
		newDecls = append(newDecls, mainFunc)
	}
	f.Decls = newDecls
}

func isMain(fn *ast.FuncDecl) bool {
	return fn.Recv == nil && fn.Name != nil && fn.Name.Name == "main"
}
