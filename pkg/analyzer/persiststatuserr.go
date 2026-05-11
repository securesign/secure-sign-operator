package analyzer

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var PersistStatusErr = &analysis.Analyzer{
	Name:     "persiststatuserr",
	Doc:      "checks that PersistStatus error return value is not discarded",
	Run:      runPersistStatusErr,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
}

func runPersistStatusErr(pass *analysis.Pass) (interface{}, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.AssignStmt)(nil),
		(*ast.ExprStmt)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			if len(stmt.Rhs) != 1 {
				return
			}
			call, ok := stmt.Rhs[0].(*ast.CallExpr)
			if !ok || !isPersistStatusCall(pass, call) {
				return
			}
			if len(stmt.Lhs) >= 2 {
				if ident, ok := stmt.Lhs[1].(*ast.Ident); ok && ident.Name == "_" {
					pass.Reportf(stmt.Pos(), "PersistStatus error return value must not be discarded")
				}
			}

		case *ast.ExprStmt:
			call, ok := stmt.X.(*ast.CallExpr)
			if !ok || !isPersistStatusCall(pass, call) {
				return
			}
			pass.Reportf(stmt.Pos(), "PersistStatus return values must not be discarded")
		}
	})

	return nil, nil
}

func isPersistStatusCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "PersistStatus" {
		return false
	}

	obj := pass.TypesInfo.ObjectOf(sel.Sel)
	if obj == nil {
		return false
	}

	fn, ok := obj.(*types.Func)
	if !ok {
		return false
	}

	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return false
	}

	results := sig.Results()
	if results.Len() != 2 {
		return false
	}

	// Second return must be error
	if !types.Implements(results.At(1).Type(), errorInterface()) {
		return false
	}

	return true
}

var cachedErrorInterface *types.Interface

func errorInterface() *types.Interface {
	if cachedErrorInterface == nil {
		cachedErrorInterface = types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	}
	return cachedErrorInterface
}
