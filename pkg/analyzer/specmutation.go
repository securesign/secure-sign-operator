package analyzer

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var SpecMutation = &analysis.Analyzer{
	Name:     "specmutation",
	Doc:      "checks that action types do not mutate instance.Spec (only Status is persisted)",
	Run:      runSpecMutation,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
}

func runSpecMutation(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.FuncDecl)(nil),
	}

	// Pass 1: collect action types (receiver types with Handle(...) *Result).
	actionTypes := map[types.Type]bool{}
	insp.Preorder(nodeFilter, func(n ast.Node) {
		fn := n.(*ast.FuncDecl)
		if rt := actionReceiverType(pass, fn); rt != nil {
			actionTypes[rt] = true
		}
	})

	if len(actionTypes) == 0 {
		return nil, nil
	}

	// Pass 2: in any method on an action type, flag Spec mutations.
	insp.Preorder(nodeFilter, func(n ast.Node) {
		fn := n.(*ast.FuncDecl)
		if fn.Recv == nil || fn.Body == nil {
			return
		}

		recvType := receiverBaseType(pass, fn)
		if recvType == nil || !actionTypes[recvType] {
			return
		}

		instanceParams := specBearingParams(pass, fn)
		if len(instanceParams) == 0 {
			return
		}

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, lhs := range assign.Lhs {
				for _, param := range instanceParams {
					if isSpecFieldAccess(pass, lhs, param) {
						pass.Reportf(assign.Pos(), "action type must not mutate instance.Spec; only Status is persisted by the action framework")
					}
				}
			}
			return true
		})
	})

	return nil, nil
}

// actionReceiverType returns the named receiver base type if fn is a Handle
// method with signature Handle(context.Context, T) *Result. Returns nil otherwise.
func actionReceiverType(pass *analysis.Pass, fn *ast.FuncDecl) types.Type {
	if fn.Name.Name != "Handle" || fn.Recv == nil {
		return nil
	}

	fnType := fn.Type
	if fnType.Params == nil || len(fnType.Params.List) < 2 {
		return nil
	}
	if fnType.Results == nil || len(fnType.Results.List) != 1 {
		return nil
	}

	resultType := pass.TypesInfo.TypeOf(fnType.Results.List[0].Type)
	if resultType == nil {
		return nil
	}
	ptr, ok := resultType.(*types.Pointer)
	if !ok {
		return nil
	}
	named, ok := ptr.Elem().(*types.Named)
	if !ok || named.Obj().Name() != "Result" {
		return nil
	}

	return receiverBaseType(pass, fn)
}

// receiverBaseType returns the underlying named type of the receiver,
// stripping the pointer if present.
func receiverBaseType(pass *analysis.Pass, fn *ast.FuncDecl) types.Type {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return nil
	}
	t := pass.TypesInfo.TypeOf(fn.Recv.List[0].Type)
	if t == nil {
		return nil
	}
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	return t
}

// specBearingParams returns all parameters whose underlying struct type has a "Spec" field.
func specBearingParams(pass *analysis.Pass, fn *ast.FuncDecl) []*types.Var {
	if fn.Type.Params == nil {
		return nil
	}
	var result []*types.Var
	for _, field := range fn.Type.Params.List {
		for _, name := range field.Names {
			obj := pass.TypesInfo.ObjectOf(name)
			if obj == nil {
				continue
			}
			v, ok := obj.(*types.Var)
			if !ok {
				continue
			}
			if hasSpecField(v.Type()) {
				result = append(result, v)
			}
		}
	}
	return result
}

func hasSpecField(t types.Type) bool {
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		return false
	}
	for i := 0; i < st.NumFields(); i++ {
		if st.Field(i).Name() == "Spec" {
			return true
		}
	}
	return false
}

// isSpecFieldAccess checks if expr is an assignment target of the form instance.Spec.* or instance.Spec
func isSpecFieldAccess(pass *analysis.Pass, expr ast.Expr, instanceParam *types.Var) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	cur := sel
	for {
		if cur.Sel.Name == "Spec" {
			obj := resolveToVar(pass, cur.X)
			return obj == instanceParam
		}
		parent, ok := cur.X.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		cur = parent
	}
}

func resolveToVar(pass *analysis.Pass, expr ast.Expr) *types.Var {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return nil
	}
	obj := pass.TypesInfo.ObjectOf(ident)
	if obj == nil {
		return nil
	}
	v, ok := obj.(*types.Var)
	if !ok {
		return nil
	}
	return v
}
