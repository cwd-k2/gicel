package check

import (
	"fmt"

	"github.com/cwd-k2/gomputation/internal/core"
	"github.com/cwd-k2/gomputation/internal/syntax"
	"github.com/cwd-k2/gomputation/pkg/types"
)

func (ch *Checker) checkDecls(decls []syntax.Decl) *core.Program {
	prog := &core.Program{}

	// Collect type annotations.
	annotations := make(map[string]types.Type)
	for _, d := range decls {
		if ann, ok := d.(*syntax.DeclTypeAnn); ok {
			annotations[ann.Name] = ch.resolveTypeExpr(ann.Type)
		}
	}

	// Process data declarations.
	for _, d := range decls {
		if data, ok := d.(*syntax.DeclData); ok {
			ch.processDataDecl(data, prog)
		}
	}

	// Process type aliases.
	for _, d := range decls {
		if alias, ok := d.(*syntax.DeclTypeAlias); ok {
			ch.processTypeAlias(alias)
		}
	}

	// Detect cyclic aliases before expansion can diverge.
	ch.validateAliasGraph()

	// Process value definitions.
	for _, d := range decls {
		if def, ok := d.(*syntax.DeclValueDef); ok {
			ch.processValueDef(def, annotations, prog)
		}
	}

	return prog
}

func (ch *Checker) processDataDecl(d *syntax.DeclData, prog *core.Program) {
	// Register type constructor kind.
	var kind types.Kind = types.KType{}
	for i := len(d.Params) - 1; i >= 0; i-- {
		kind = &types.KArrow{From: types.KType{}, To: kind}
	}
	ch.config.RegisteredTypes[d.Name] = kind

	dataInfo := &DataTypeInfo{Name: d.Name}

	// Build result type: T a b c ...
	var resultType types.Type = &types.TyCon{Name: d.Name, S: d.S}
	for _, p := range d.Params {
		resultType = &types.TyApp{Fun: resultType, Arg: &types.TyVar{Name: p.Name, S: p.S}, S: d.S}
	}

	// Register each constructor.
	coreDecl := core.DataDecl{Name: d.Name, S: d.S}
	for _, p := range d.Params {
		coreDecl.TyParams = append(coreDecl.TyParams, core.TyParam{Name: p.Name, Kind: types.KType{}})
	}

	for _, con := range d.Cons {
		var conType types.Type = resultType
		var fieldTypes []types.Type
		for i := len(con.Fields) - 1; i >= 0; i-- {
			fieldTy := ch.resolveTypeExpr(con.Fields[i])
			fieldTypes = append([]types.Type{fieldTy}, fieldTypes...)
			conType = types.MkArrow(fieldTy, conType)
		}
		// Wrap in forall for type params.
		for i := len(d.Params) - 1; i >= 0; i-- {
			conType = types.MkForall(d.Params[i].Name, types.KType{}, conType)
		}

		ch.conTypes[con.Name] = conType
		ch.ctx.Push(&CtxVar{Name: con.Name, Type: conType})
		dataInfo.Constructors = append(dataInfo.Constructors, ConInfo{Name: con.Name, Arity: len(con.Fields)})
		ch.conInfo[con.Name] = dataInfo
		coreDecl.Cons = append(coreDecl.Cons, core.ConDecl{Name: con.Name, Fields: fieldTypes, S: con.S})
	}

	prog.DataDecls = append(prog.DataDecls, coreDecl)
}

// typeArity counts the number of arrow arguments in a type,
// stripping outer foralls. E.g. forall a. A -> B -> C has arity 2.
func typeArity(ty types.Type) int {
	for {
		switch t := ty.(type) {
		case *types.TyForall:
			ty = t.Body
		case *types.TyArrow:
			return 1 + typeArity(t.To)
		default:
			return 0
		}
	}
}

func (ch *Checker) processTypeAlias(d *syntax.DeclTypeAlias) {
	var params []string
	for _, p := range d.Params {
		params = append(params, p.Name)
	}
	body := ch.resolveTypeExpr(d.Body)
	ch.aliases[d.Name] = &aliasInfo{params: params, body: body}
}

func (ch *Checker) processValueDef(d *syntax.DeclValueDef, annotations map[string]types.Type, prog *core.Program) {
	annTy, hasAnn := annotations[d.Name]

	// Check if it's an assumption.
	if v, ok := d.Expr.(*syntax.ExprVar); ok && v.Name == "assumption" {
		// Try AST annotation first, then config assumptions.
		aTy := annTy
		if !hasAnn {
			if ch.config.Assumptions != nil {
				aTy, hasAnn = ch.config.Assumptions[d.Name]
			}
		}
		if !hasAnn {
			ch.addError(d.S, fmt.Sprintf("assumption %s requires a type annotation", d.Name))
			return
		}
		ch.ctx.Push(&CtxVar{Name: d.Name, Type: aTy})
		prog.Bindings = append(prog.Bindings, core.Binding{
			Name: d.Name,
			Type: aTy,
			Expr: &core.PrimOp{Name: d.Name, Arity: typeArity(aTy), S: d.S},
			S:    d.S,
		})
		return
	}

	var coreExpr core.Core
	var ty types.Type
	if hasAnn {
		coreExpr = ch.check(d.Expr, annTy)
		ty = annTy
	} else {
		ty, coreExpr = ch.infer(d.Expr)
	}

	// Zonk the type.
	ty = ch.unifier.Zonk(ty)

	ch.ctx.Push(&CtxVar{Name: d.Name, Type: ty})
	prog.Bindings = append(prog.Bindings, core.Binding{
		Name: d.Name,
		Type: ty,
		Expr: coreExpr,
		S:    d.S,
	})
}
