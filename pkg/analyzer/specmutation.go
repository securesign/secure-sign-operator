package analyzer

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// MutatesParam is an analysis fact exported on *types.Func objects.
// Indices records which pointer parameters a function writes through (any field).
// SpecIndices records which parameters have their .Spec field specifically mutated.
type MutatesParam struct {
	Indices     []int
	SpecIndices []int
}

func (*MutatesParam) AFact() {}
func (f *MutatesParam) String() string {
	return "mutatesParam"
}

var SpecMutation = &analysis.Analyzer{
	Name:      "specmutation",
	Doc:       "checks that action types do not mutate instance.Spec (only Status is persisted)",
	Run:       runSpecMutation,
	Requires:  []*analysis.Analyzer{inspect.Analyzer},
	FactTypes: []analysis.Fact{(*MutatesParam)(nil)},
}

func runSpecMutation(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.FuncDecl)(nil),
	}

	// Phase 1: export mutation facts for every function in this package.
	insp.Preorder(nodeFilter, func(n ast.Node) {
		fn := n.(*ast.FuncDecl)
		if fn.Body == nil {
			return
		}
		indices, specIndices := mutatedParamIndices(pass, fn)
		if len(indices) == 0 && len(specIndices) == 0 {
			return
		}
		obj := pass.TypesInfo.ObjectOf(fn.Name)
		if obj == nil {
			return
		}
		funcObj, ok := obj.(*types.Func)
		if !ok {
			return
		}
		pass.ExportObjectFact(funcObj, &MutatesParam{Indices: indices, SpecIndices: specIndices})
	})

	// Phase 2: collect action types.
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

	// Phase 3: in any method on an action type, flag spec mutations.
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
			switch node := n.(type) {
			case *ast.AssignStmt:
				for _, lhs := range node.Lhs {
					for _, param := range instanceParams {
						if isSpecFieldAccess(pass, lhs, param) {
							pass.Reportf(node.Pos(), "action type must not mutate instance.Spec; only Status is persisted by the action framework")
						}
					}
				}
			case *ast.CallExpr:
				checkIndirectSpecMutation(pass, node, instanceParams)
			}
			return true
		})
	})

	return nil, nil
}

// mutatedParamIndices returns two sets of 0-based parameter indices:
//   - indices: pointer parameters written through (any field, e.g. param.Field = ...)
//   - specIndices: parameters where .Spec specifically is mutated (e.g. param.Spec.Field = ...)
func mutatedParamIndices(pass *analysis.Pass, fn *ast.FuncDecl) (indices []int, specIndices []int) {
	if fn.Type.Params == nil {
		return nil, nil
	}

	paramVars := collectParamVars(pass, fn)
	if len(paramVars) == 0 {
		return nil, nil
	}

	ptrParams := map[*types.Var]int{}
	for idx, v := range paramVars {
		if v == nil {
			continue
		}
		t := v.Type()
		if _, ok := t.(*types.Pointer); ok {
			ptrParams[v] = idx
		}
	}
	if len(ptrParams) == 0 {
		return nil, nil
	}

	mutated := map[int]bool{}
	specMutated := map[int]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range assign.Lhs {
			if v := rootVar(pass, lhs); v != nil {
				if idx, ok := ptrParams[v]; ok {
					mutated[idx] = true
					if containsSpecSelector(lhs, v, pass) {
						specMutated[idx] = true
					}
				}
			}
			if star, ok := lhs.(*ast.StarExpr); ok {
				if v := rootVar(pass, star.X); v != nil {
					if idx, ok := ptrParams[v]; ok {
						mutated[idx] = true
					}
				}
			}
		}
		return true
	})

	for idx := range mutated {
		indices = append(indices, idx)
	}
	for idx := range specMutated {
		specIndices = append(specIndices, idx)
	}
	return indices, specIndices
}

// containsSpecSelector returns true if the selector chain from expr back to
// paramVar passes through a field named "Spec" (e.g. param.Spec.Field).
func containsSpecSelector(expr ast.Expr, paramVar *types.Var, pass *analysis.Pass) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	cur := sel
	for {
		if cur.Sel.Name == "Spec" {
			if v := resolveToVar(pass, cur.X); v == paramVar {
				return true
			}
		}
		parent, ok := cur.X.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		cur = parent
	}
}

// collectParamVars returns the *types.Var for each parameter position.
// Unnamed parameters get a nil entry.
func collectParamVars(pass *analysis.Pass, fn *ast.FuncDecl) []*types.Var {
	var vars []*types.Var
	for _, field := range fn.Type.Params.List {
		if len(field.Names) == 0 {
			vars = append(vars, nil)
			continue
		}
		for _, name := range field.Names {
			obj := pass.TypesInfo.ObjectOf(name)
			if obj == nil {
				vars = append(vars, nil)
				continue
			}
			v, ok := obj.(*types.Var)
			if !ok {
				vars = append(vars, nil)
				continue
			}
			vars = append(vars, v)
		}
	}
	return vars
}

// rootVar walks a selector chain (a.b.c) back to the root identifier
// and returns its *types.Var if it is a local variable/parameter.
func rootVar(pass *analysis.Pass, expr ast.Expr) *types.Var {
	for {
		switch e := expr.(type) {
		case *ast.SelectorExpr:
			expr = e.X
		case *ast.Ident:
			obj := pass.TypesInfo.ObjectOf(e)
			if obj == nil {
				return nil
			}
			v, ok := obj.(*types.Var)
			if !ok {
				return nil
			}
			return v
		case *ast.IndexExpr:
			expr = e.X
		default:
			return nil
		}
	}
}

// checkIndirectSpecMutation flags a call if it passes a spec-bearing
// expression to a parameter position that the callee mutates.
func checkIndirectSpecMutation(pass *analysis.Pass, call *ast.CallExpr, instanceParams []*types.Var) {
	callee := resolveCallee(pass, call)
	if callee == nil {
		return
	}

	var fact MutatesParam
	if !pass.ImportObjectFact(callee, &fact) {
		return
	}

	sig, ok := callee.Type().(*types.Signature)
	if !ok {
		return
	}

	// Check Indices: callee mutates param — is the argument a Spec pointer?
	for _, idx := range fact.Indices {
		if idx >= len(call.Args) || idx >= sig.Params().Len() {
			continue
		}
		if isSpecArgument(pass, call.Args[idx], instanceParams) {
			pass.Reportf(call.Pos(), "action type must not mutate instance.Spec; only Status is persisted by the action framework")
			return
		}
	}

	// Check SpecIndices: callee mutates param.Spec — is the argument the instance object?
	for _, idx := range fact.SpecIndices {
		if idx >= len(call.Args) || idx >= sig.Params().Len() {
			continue
		}
		if isInstanceArgument(pass, call.Args[idx], instanceParams) {
			pass.Reportf(call.Pos(), "action type must not mutate instance.Spec; only Status is persisted by the action framework")
			return
		}
	}
}

// isInstanceArgument returns true if expr is one of the spec-bearing
// instance parameters (the whole object, not just &instance.Spec).
func isInstanceArgument(pass *analysis.Pass, expr ast.Expr, instanceParams []*types.Var) bool {
	v := resolveToVar(pass, expr)
	if v == nil {
		return false
	}
	for _, param := range instanceParams {
		if v == param {
			return true
		}
	}
	return false
}

// resolveCallee resolves a call expression to a *types.Func.
func resolveCallee(pass *analysis.Pass, call *ast.CallExpr) *types.Func {
	var id *ast.Ident
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		id = fn
	case *ast.SelectorExpr:
		id = fn.Sel
	default:
		return nil
	}

	obj := pass.TypesInfo.ObjectOf(id)
	if obj == nil {
		return nil
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		return nil
	}
	return fn
}

// isSpecArgument returns true if expr is a pointer to a spec field:
//   - &instance.Spec or &instance.Spec.SubField
//   - instance.Spec.PointerField (already a pointer type)
func isSpecArgument(pass *analysis.Pass, expr ast.Expr, instanceParams []*types.Var) bool {
	// Handle &instance.Spec... expressions.
	if unary, ok := expr.(*ast.UnaryExpr); ok {
		return isSpecOrSpecChild(pass, unary.X, instanceParams)
	}

	// Handle instance.Spec.PointerField — the field is already a pointer.
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	t := pass.TypesInfo.TypeOf(expr)
	if t == nil {
		return false
	}
	if _, isPtr := t.(*types.Pointer); !isPtr {
		return false
	}
	return isSpecOrSpecChild(pass, sel, instanceParams)
}

// isSpecOrSpecChild returns true if expr is instance.Spec or
// instance.Spec.SubField.SubField... (any depth under Spec).
func isSpecOrSpecChild(pass *analysis.Pass, expr ast.Expr, instanceParams []*types.Var) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	cur := sel
	for {
		if cur.Sel.Name == "Spec" {
			obj := resolveToVar(pass, cur.X)
			for _, param := range instanceParams {
				if obj == param {
					return true
				}
			}
			return false
		}
		parent, ok := cur.X.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		cur = parent
	}
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
