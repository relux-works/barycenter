package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestRenderBoundaryHasNoBlockingOrDynamicOperations(t *testing.T) {
	t.Parallel()
	checks := map[string][]string{
		"engine.go": {"Render", "renderMusic", "mixOverlay", "mixInterrupt", "mixLive", "applyOverlayLimiter"},
		"gain.go":   {"ApplyMusicRamp", "Amplitude"},
	}
	for filename, functions := range checks {
		file, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", filename, err)
		}
		for _, name := range functions {
			function := findFunction(file, name)
			if function == nil {
				t.Fatalf("%s: function %s not found", filename, name)
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				switch value := node.(type) {
				case *ast.GoStmt:
					t.Errorf("%s.%s starts a goroutine in the render boundary", filename, name)
				case *ast.CallExpr:
					if ident, ok := value.Fun.(*ast.Ident); ok {
						switch ident.Name {
						case "make", "new", "append":
							t.Errorf("%s.%s calls %s in the render boundary", filename, name, ident.Name)
						}
					}
					if selector, ok := value.Fun.(*ast.SelectorExpr); ok {
						switch selector.Sel.Name {
						case "Lock", "RLock", "Wait", "Sleep":
							t.Errorf("%s.%s calls blocking %s in the render boundary", filename, name, selector.Sel.Name)
						}
					}
				}
				return true
			})
		}
	}
}

func findFunction(file *ast.File, name string) *ast.FuncDecl {
	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok && function.Name.Name == name {
			return function
		}
	}
	return nil
}
