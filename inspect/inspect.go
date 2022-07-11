package inspect

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/packages"
)

// ParseArgs
func ParseArgs(args []string) {

}

// what ever
func PrintInitTree(args []string) {

	// go list -deps -f '{{.Dir}}:{{.GoFiles}}' ./src
}

func CreateOverlay(overlayDir string, args []string) {

}

func runArgs(args []string) {
	cfg := &packages.Config{Mode: packages.NeedFiles | packages.NeedSyntax | packages.NeedDeps | packages.NeedImports | packages.NeedTypes | packages.NeedTypesInfo}

	if len(args) == 0 {
		args = []string{"."}
	}

	// global packages:
	// args = append(args)

	pkgs, err := packages.Load(cfg, args...)

	if err != nil {
		panic(err)
	}
	// command line pkgs
	packages.Visit(pkgs, func(p *packages.Package) bool {
		fmt.Printf("Visit:%v, pkgPath=%v,types.Path=%v\n", p, p.PkgPath, p.Types.Path())
		fmt.Printf("go files:%v\n", p.GoFiles)
		fmt.Printf("imports:%v\n", p.Imports)
		return true
	}, nil)

	// for _, p := range pkgs {
	// 	fmt.Printf("Visist:%v, pkgPath=%v\n", p, p.PkgPath)
	// 	fmt.Printf("go files:%v\n", p.GoFiles)
	// 	fmt.Printf("imports:%v\n", p.Imports)

	// 	for _, f := range p.Syntax {
	// 		parseFile(f)
	// 	}
	// }
}

func parseFile(f *ast.File) {
	ast.Inspect(f, func(n ast.Node) bool {
		fmt.Printf("inspecting:%T %v\n", n, n)
		switch n := n.(type) {
		case *ast.FuncDecl:
			fmt.Printf("    FuncDecl:%s\n", n.Name.Name)
		}
		return true
	})
}
