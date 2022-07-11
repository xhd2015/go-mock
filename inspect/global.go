package inspect

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sync"

	"golang.org/x/tools/go/packages"
)

var globalInitOnce sync.Once

var errorInitOnce sync.Once
var errType *types.Named

type GlobalPkgs struct {
	Ctx       *packages.Package
	Builtin   *packages.Package
	CtxType   *types.Interface
	ErrorType *types.Interface
}

var g = &GlobalPkgs{}

func GetGlobalPackages() *GlobalPkgs {
	globalInitOnce.Do(func() {
		fset := token.NewFileSet()
		cfg := &packages.Config{
			Mode: packages.NeedFiles | packages.NeedSyntax | packages.NeedDeps | packages.NeedImports | packages.NeedTypes | packages.NeedTypesInfo,
			Fset: fset,
		}
		pkgs, err := packages.Load(cfg, "context", "builtin")
		if err != nil {
			panic(err)
		}
		for _, pkg := range pkgs {
			switch pkg.Types.Path() {
			case "context":
				g.Ctx = pkg
			case "builtin":
				g.Builtin = pkg
			}
		}
		g.CtxType = SearchDecl("Context", g.Ctx).Underlying().(*types.Interface)
		g.ErrorType = SearchDecl("error", g.Builtin).Underlying().(*types.Interface)
	})
	return g
}
func GetCtxType() *types.Interface {
	return GetGlobalPackages().CtxType
}

func GetErrorType() *types.Named {
	errorInitOnce.Do(func() {
		errType = types.Universe.Lookup("error").(*types.TypeName).Type().(*types.Named)
	})
	return errType
}

func SearchDecl(name string, pkg *packages.Package) types.Type {
	if pkg.TypesInfo == nil {
		panic("requries TypesInfo of package")
	}
	if name == "" {
		return nil
	}
	for _, f := range pkg.Syntax {
		for _, decl := range f.Decls {
			if t, ok := decl.(*ast.GenDecl); ok && t.Tok == token.TYPE {
				for _, spec := range t.Specs {
					if tspec, ok := spec.(*ast.TypeSpec); ok && tspec.Name.Name == name {
						return pkg.TypesInfo.TypeOf(tspec.Type)
					}
				}
			}
		}
	}
	panic(fmt.Errorf("declare of %q in %v not found", name, pkg))
}
