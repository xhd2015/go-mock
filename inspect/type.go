package inspect

import (
	"fmt"
	"go/ast"
	"go/types"
	"strings"
)

type Kind int

const (
	Basic     Kind = 1
	Named     Kind = 2
	Struct    Kind = 3
	Ptr       Kind = 4
	Interface Kind = 5
	Func      Kind = 6
	Slice     Kind = 7
	Array     Kind = 8
	Map       Kind = 9
	Chan      Kind = 10
)

// TypeExpr represents Type appeared inside a package.
type TypeExpr struct {
	Kind Kind
	Elem *TypeExpr
	Key  *TypeExpr

	// Array
	Len int

	// struct
	Fields  []*StructFieldExpr
	Methods []*Arg // interface type

	// func or method
	Recv    *Arg
	Args    []*Arg
	Results []*Arg

	// named type
	PkgPath      string // valid when named type
	ShortPkgPath string
	Name         string
	Expr         string
}

type StructFieldExpr struct {
	Name      string
	Type      *TypeExpr
	Tag       string
	Anonymous bool // is an embedded field
}

type Arg struct {
	Name string
	Type *TypeExpr
}

func NewTypeExpr(t types.Type) *TypeExpr {
	return buildTypeExpr(t, make(map[types.Type]*TypeExpr))
}

func buildTypeExpr(t types.Type, m map[types.Type]*TypeExpr) *TypeExpr {
	if m[t] != nil {
		return m[t]
	}

	exp := &TypeExpr{}
	var kind Kind
	switch t := t.(type) {
	case *types.Basic:
		kind = Basic
		exp.Name = t.Name()
	case *types.Named:
		kind = Named
		exp.PkgPath = t.Obj().Pkg().Path()
		exp.Name = t.Obj().Name()
		exp.ShortPkgPath = t.Obj().Pkg().Name()
		exp.Expr = t.String()
	case *types.Struct:
		fields := make([]*StructFieldExpr, 0, t.NumFields())
		for i := 0; i < t.NumFields(); i++ {
			f := t.Field(i)
			fields = append(fields, &StructFieldExpr{
				Name:      f.Name(),
				Anonymous: f.Embedded(),
				Tag:       t.Tag(i),
				Type:      buildTypeExpr(f.Type(), m),
			})
		}
		exp.Fields = fields
	case *types.Interface:
		kind = Interface
		// TODO: embedded types
		methods := make([]*Arg, 0, t.NumMethods())
		for i := 0; i < t.NumMethods(); i++ {
			fn := t.Method(i)
			methods = append(methods, &Arg{
				Name: fn.Name(),
				Type: buildTypeExpr(fn.Type(), m),
			})
		}
		exp.Methods = methods
	case *types.Pointer:
		kind = Ptr
		exp.Elem = buildTypeExpr(t.Elem(), m)
	case *types.Signature:
		kind = Func
		if t.Recv() != nil {
			exp.Recv = parseArg(t.Recv(), m)
		}
		exp.Args = parseArgs(t.Params(), m)
		exp.Results = parseArgs(t.Results(), m)
	case *types.Array:
		kind = Array
		exp.Len = int(t.Len())
		exp.Elem = buildTypeExpr(t.Elem(), m)
	case *types.Slice:
		kind = Slice
		exp.Elem = buildTypeExpr(t.Elem(), m)
	case *types.Map:
		kind = Map
		exp.Key = buildTypeExpr(t.Key(), m)
		exp.Elem = buildTypeExpr(t.Elem(), m)
	case *types.Chan:
		kind = Chan
		exp.Elem = buildTypeExpr(t.Elem(), m)
	default:
		panic(fmt.Errorf("unrecognized type:%T", t))
	}
	exp.Kind = kind
	return exp
}
func parseArgs(args *types.Tuple, m map[types.Type]*TypeExpr) []*Arg {
	res := make([]*Arg, 0, args.Len())
	for i := 0; i < args.Len(); i++ {
		v := args.At(i)
		res = append(res, parseArg(v, m))
	}
	return res
}

func parseArg(v *types.Var, m map[types.Type]*TypeExpr) *Arg {
	t := buildTypeExpr(v.Type(), m)
	return &Arg{Name: v.Name(), Type: t}
}

func (c *TypeExpr) Traverse(fn func(subType *TypeExpr)) {
	c.traverseNoRepeat(fn, make(map[*TypeExpr]bool))
}
func (c *TypeExpr) traverseNoRepeat(fn func(subType *TypeExpr), seen map[*TypeExpr]bool) {
	if seen[c] {
		return
	}
	seen[c] = true
	fn(c)
	switch c.Kind {
	case Slice, Array, Ptr:
		c.Elem.traverseNoRepeat(fn, seen)
	case Map:
		c.Key.traverseNoRepeat(fn, seen)
		c.Elem.traverseNoRepeat(fn, seen)
	case Struct:
		for _, field := range c.Fields {
			field.Type.traverseNoRepeat(fn, seen)
		}
	case Interface:

	case Func:
		if c.Recv != nil {
			c.Recv.Type.traverseNoRepeat(fn, seen)
		}
		for _, arg := range c.Args {
			arg.Type.traverseNoRepeat(fn, seen)
		}
		for _, out := range c.Results {
			out.Type.traverseNoRepeat(fn, seen)
		}
	default:
	}
}

// func GetTypeExprs(types []reflect.Type) []*TypeExpr {
// 	exprs := make([]*TypeExpr, 0, len(types))
// 	for _, argType := range types {
// 		exprs = append(exprs, NewTypeExpr(argType))
// 	}
// 	return exprs
// }

func ShortPackagePath(pkgPath string) string {
	if pkgPath == "" {
		return ""
	}
	idx := strings.LastIndex(pkgPath, "/")
	if idx < 0 {
		return pkgPath
	}
	lastPkg := pkgPath[idx+1:]
	if lastPkg == "" {
		panic(fmt.Errorf("invalid package:%s", pkgPath))
	}
	return lastPkg
}
func (c *TypeExpr) shortRef() string {
	pkgPrefix := ""
	if c.ShortPkgPath != "" {
		pkgPrefix = c.ShortPkgPath + "."
	}
	return pkgPrefix + c.Name
}
func (c *TypeExpr) IsPrimitive() bool {
	return c.Kind == Basic
}
func (c *TypeExpr) IsPtr() bool {
	return c.Kind == Ptr
}
func (c *TypeExpr) String() string {
	if c.Name != "" {
		return c.shortRef()
	}
	switch c.Kind {
	case Slice:
		return "[]" + c.Elem.String()
	case Array:
		return fmt.Sprintf("[%d]%s", c.Len, c.Elem.String())
	case Ptr:
		return "*" + c.Elem.String()
	case Map:
		return fmt.Sprintf("map[%s]%s", c.Key, c.Elem)
	case Func:
		args := make([]string, 0, len(c.Args))
		for _, arg := range c.Args {
			args = append(args, arg.Type.String())
		}
		results := make([]string, 0, len(c.Results))
		for _, res := range c.Results {
			results = append(results, res.Type.String())
		}
		left := ""
		right := ""
		if len(c.Results) > 1 {
			left = "("
			right = ")"
		}
		return fmt.Sprintf("func(%s) %s%s%s", strings.Join(args, ","), left, strings.Join(results, ","), right)
	// case reflect.Chan: // TODO
	default:
		panic(fmt.Errorf("unhandled kind:%v", c.Kind))
	}
}

func TraverseTypes(t []types.Type, fn func(t types.Type) bool) {
	m := make(map[types.Type]bool, len(t))
	for _, x := range t {
		traverseType(x, fn, m)
	}
}
func TraverseType(t types.Type, fn func(t types.Type) bool) {
	traverseType(t, fn, make(map[types.Type]bool))
}

func traverseType(t types.Type, fn func(t types.Type) bool, m map[types.Type]bool) {
	if t == nil {
		return
	}
	if m[t] {
		return
	}
	m[t] = true
	if !fn(t) {
		return
	}
	switch t := t.(type) {
	case *types.Basic:
		// already done
	case *types.Named:
		// underlying?
		traverseType(t.Underlying(), fn, m)
	case *types.Struct:
		for i := 0; i < t.NumFields(); i++ {
			traverseType(t.Field(i).Type(), fn, m)
		}
	case *types.Interface:
		// TODO: embedded types
		for i := 0; i < t.NumMethods(); i++ {
			traverseType(t.Method(i).Type(), fn, m)
		}
	case *types.Pointer:
		traverseType(t.Elem(), fn, m)
	case *types.Signature:
		if t.Recv() != nil {
			traverseType(t.Recv().Type(), fn, m)
		}
		for i := 0; i < t.Params().Len(); i++ {
			traverseType(t.Params().At(i).Type(), fn, m)
		}
		for i := 0; i < t.Results().Len(); i++ {
			traverseType(t.Results().At(i).Type(), fn, m)
		}
	case *types.Array:
		traverseType(t.Elem(), fn, m)
	case *types.Slice:
		traverseType(t.Elem(), fn, m)
	case *types.Map:
		traverseType(t.Key(), fn, m)
		traverseType(t.Elem(), fn, m)
	case *types.Chan:
		traverseType(t.Elem(), fn, m)
	default:
		panic(fmt.Errorf("unrecognized type:%T", t))
	}
}

func debugShow(typesInfo *types.Info, node ast.Node) {
	idt, _ := node.(*ast.Ident)
	sel, _ := node.(*ast.SelectorExpr)
	expr, _ := node.(ast.Expr)
	t := typesInfo.Defs[idt]
	t2 := typesInfo.Implicits[node]
	t3 := typesInfo.Selections[sel]
	t4 := typesInfo.Scopes[node]
	t5 := typesInfo.Types[expr]
	t6 := typesInfo.Types[expr]
	t7 := typesInfo.Uses[idt]
	t8 := typesInfo.TypeOf(expr)
	t9 := typesInfo.TypeOf(expr)
	a := sel
	_ = a
	_ = t
	_ = t2
	_ = t3
	_ = t4
	_ = t5
	_ = t6
	_ = t7
	_ = t8
	_ = t9

	if sel!=nil{
		debugShow(typesInfo,sel.X)
		debugShow(typesInfo,sel.Sel)
	}
}
