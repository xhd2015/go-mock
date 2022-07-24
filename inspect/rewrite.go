package inspect

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"io/ioutil"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/xhd2015/go-mock/code/edit"
	"github.com/xhd2015/go-mock/code/gen"
)

// this file provides source file rewrite and mock stub generation.
// the generated stub are copied into a sub-directory with the same
// package hierarchy.
// For example, package 'a/b/c' is put into 'test/mock_gen/a/c'.
// The same package name is still present. This time all file
// are merged into one.
// For unexported types and their dependency unexported types,
// an exported name will be made available to external packages.

const MOCK_PKG = "github.com/xhd2015/go-mock/mock"
const SKIP_MOCK_PKG = "_SKIP_MOCK"
const SKIP_MOCK_FILE = "_SKIP_MOCK_THIS_FILE"

type RewriteOptions struct {
	// Filter tests whether the specific function should be rewritten
	Filter func(pkgPath string, fileName string, ownerName string, ownerIsPtr bool, funcName string) bool
}

type RewriteResult struct {
	Funcs map[string]map[string]string
	Error error // error if any
}

type ContentError struct {
	PkgPath string // a repeat of the map result
	Files   map[string]*FileContentError

	// exported types
	// function prototypes are based on
	MockContent string
	Error       error // error if any
}
type FileContentError struct {
	OrigFile string // a repeat of the key
	Content  string
	Error    error
}

// Rewrite returns a map of rewritten content,
func Rewrite(args []string, opts *RewriteOptions) map[string]*ContentError {
	fset, pkgs, err := LoadPackages(args, nil)
	if err != nil {
		panic(err)
	}
	pkgs, _ = GetSameModulePackagesAndPkgsGiven(pkgs, nil, nil)
	return RewritePackages(fset, pkgs, opts)
}

// RewritePackages
func RewritePackages(fset *token.FileSet, pkgs []*packages.Package, opts *RewriteOptions) map[string]*ContentError {
	m := make(map[string]*ContentError)
	for _, p := range pkgs {
		c := rewritePackage(p, fset, opts)
		if c == nil {
			// maybe skipped
			continue
		}
		m[c.PkgPath] = c
	}
	return m
}

func rewritePackage(p *packages.Package, fset *token.FileSet, opts *RewriteOptions) *ContentError {
	if p.Types.Scope().Lookup(SKIP_MOCK_PKG) != nil {
		return nil
	}

	pkgPath := p.PkgPath
	m := make(map[string]*FileContentError, len(p.Syntax))

	var fileDetails []*RewriteFileDetail
	for _, f := range p.Syntax {
		if f.Scope.Lookup(SKIP_MOCK_FILE) != nil {
			continue
		}
		// the token may be loaded from cached file
		// which means there is no change in the content
		// so just skip it.
		// "/Users/xhd2015/Library/Caches/go-build/b9/b922abe0d6b605b09d7d9c1439988dc01564a743e3bcfd403e491bb07a4a7f22-d"
		// the simplest workaround is to detect if it ends with ".go"
		// NOTE: there may exists both gofiles and cacehd files for one package
		// ignoring cached files does not affect correctness.
		fname := fileNameOf(fset, f)
		if !strings.HasSuffix(fname, ".go") {
			continue
		}
		// skip test file: x_test.go
		if strings.HasSuffix(fname, "_test.go") {
			continue
		}

		content, details, noChange, err := rewriteFile(p, pkgPath, fset, f, fname, opts)
		if noChange {
			continue
		}
		m[fname] = &FileContentError{OrigFile: fname, Content: content, Error: err}
		fileDetails = append(fileDetails, details)
	}
	if len(m) == 0 {
		// if no file
		return nil
	}

	mockStub, err := genMockStub(p, fileDetails)

	// gen from details
	return &ContentError{
		PkgPath:     pkgPath,
		Files:       m,
		MockContent: mockStub,
		Error:       err,
	}
}

type AstNodeRewritter = func(node ast.Node, getNodeText func(start token.Pos, end token.Pos) []byte) ([]byte, bool)

type rewriteFuncDetail struct {
	File          string
	RewriteConfig *RewriteConfig

	// original no re-packaged
	// Args    string // including receiver
	// Results string

	ArgsRewritter    func(r AstNodeRewritter, hook func(node ast.Node, c []byte) []byte) string // re-packaged
	ResultsRewritter func(r AstNodeRewritter, hook func(node ast.Node, c []byte) []byte) string // re-packaged
}

type RewriteFileDetail struct {
	File     *ast.File
	FilePath string
	Funcs    []*rewriteFuncDetail

	// name -> EXPORTED name
	AllExportNames map[string]string
	// pkgs imported by function types
	// this info is possibly used by external generator(plus the package itself will be imported by external generator.)
	// pkg path -> name/alias
	ImportPkgByTypes map[string]*NameAlias

	// the content getter
	GetContentByPos func(start, end token.Pos) []byte
}
type NameAlias struct {
	Name  string
	Alias string
	Use   string // the effective appearance
}

func rewriteFile(pkg *packages.Package, pkgPath string, fset *token.FileSet, f *ast.File, fileName string, opts *RewriteOptions) (rewriteContent string, detail *RewriteFileDetail, noChange bool, err error) {
	// fileName is always is absolute
	content, err := ioutil.ReadFile(fileName)
	if err != nil {
		err = fmt.Errorf("rewriteFile:read file error:%v", err)
		return
	}
	funcDetails := make([]*rewriteFuncDetail, 0, 4)
	buf := edit.NewBuffer(content)
	mockImported := false

	starterTypesMapping := make(map[types.Type]bool)
	starterTypes := make([]types.Type, 0)
	addType := func(t types.Type) {
		if starterTypesMapping[t] {
			return
		}
		starterTypesMapping[t] = true
		starterTypes = append(starterTypes, t)
	}

	getContentByPos := func(start, end token.Pos) []byte {
		return getContent(fset, content, start, end)
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			if n.Body == nil {
				return true // external linked functions have no body
			}
			recv := parseRecv(n, pkg, make(map[types.Type]*Type))
			var ownerIsPtr bool
			var ownerType string
			if recv != nil {
				ownerIsPtr, ownerType = recv.Type.Ptr, recv.Type.Name
			}
			funcName := n.Name.Name

			// package level init function cannot be mocked
			if ownerType == "" && funcName == "init" {
				return true
			}

			if opts != nil && opts.Filter != nil && !opts.Filter(pkgPath, fileName, ownerType, ownerIsPtr, funcName) {
				return true
			}
			// special case, if the function returns ctx,
			// we do not mock it as such function violatiles
			// ctx-function pair relation.
			if n.Type.Results != nil && len(n.Type.Results.List) > 0 {
				for _, res := range n.Type.Results.List {
					if TokenHasQualifiedName(pkg, res.Type, "context", "Context") {
						return true
					}
				}
			}

			rc := initRewriteConfig(pkg, n, false /*skip no ctx*/)
			if rc == nil {
				// no ctx
				return true
			}
			rc.AllFields.FillFieldTypeExpr(fset, content)
			rc.Init()

			if !mockImported {
				ensureImports(fset, f, buf, "_mock", MOCK_PKG)
				mockImported = true
			}

			// rewrite names
			rc.AllFields.RenameFields(fset, buf)

			var originalResults string
			if n.Type.Results != nil && len(n.Type.Results.List) > 0 {
				if n.Type.Results.Opening == token.NoPos {
					// add ()
					buf.Insert(OffsetOf(fset, n.Type.Results.Pos())-1, "(")
					buf.Insert(OffsetOf(fset, n.Type.Results.End()), ")")
				}
				originalResults = string(getContent(fset, content, n.Type.Results.Pos(), n.Type.Results.End()))
			}
			recvCode := ""
			if n.Recv != nil {
				recvCode = string(getContent(fset, content, n.Recv.Pos(), n.Recv.End()))
			}

			// fix mixed names:
			// because we are combining recv+args,
			// so if either has no name, we must given a name _
			var args string
			if rc.Recv != nil && len(rc.FullArgs) > 0 &&
				(rc.Recv.OrigName == "" && rc.FullArgs[0].OrigName != "" ||
					rc.Recv.OrigName != "" && rc.FullArgs[0].OrigName == "") {
				if rc.FullArgs[0].OrigName != "" {
					args = string(getContent(fset, content, n.Type.Params.Pos(), n.Type.Params.End()))
				} else {
					// args has no name, but recv has name
					typePrefixMap := make(map[ast.Node]string, 1)
					for _, arg := range rc.FullArgs {
						if arg.OrigName == "" {
							typePrefixMap[arg.TypeExpr] = "_ "
						}
					}
					hook := func(node ast.Node, c []byte) []byte {
						prefix, ok := typePrefixMap[node]
						if !ok {
							return c
						}
						return append([]byte(prefix), c...)
					}
					args = string(RewriteAstNodeTextHooked(n.Type.Params, getContentByPos, nil, hook))
				}
				args = joinRecvArgs(recvCode, args, rc.Recv.OrigName, true, len(n.Type.Params.List))
			} else {
				args = string(getContent(fset, content, n.Type.Params.Pos(), n.Type.Params.End()))
				if n.Recv != nil {
					args = joinRecvArgs(recvCode, args, rc.Recv.OrigName, false, len(n.Type.Params.List))
				}
			}

			// generate patch content and insert
			newCode := rc.Gen(false /*pretty*/)
			patchContent := fmt.Sprintf(`%s}; func %s%s%s{`, newCode, rc.NewFuncName, StripNewline(args), StripNewline(originalResults))
			buf.Insert(OffsetOf(fset, n.Body.Lbrace)+1, patchContent)

			// make rewriteDetails
			funcDetails = append(funcDetails, &rewriteFuncDetail{
				File:          fileName,
				RewriteConfig: rc,

				ArgsRewritter: func(r AstNodeRewritter, hook func(node ast.Node, c []byte) []byte) (argsRepkg string) {
					argsRepkg = string(RewriteAstNodeTextHooked(n.Type.Params, getContentByPos, r, hook))
					if n.Recv != nil {
						// for exported name, should add package prefix for it.
						// unexported type has interface{}
						recvCode = string(RewriteAstNodeTextHooked(n.Recv, getContentByPos, r, hook))

						argsRepkg = joinRecvArgs(recvCode, argsRepkg, rc.Recv.OrigName, false, len(n.Type.Params.List))
					}
					return
				},
				ResultsRewritter: func(r AstNodeRewritter, hook func(node ast.Node, c []byte) []byte) (resultsRepkg string) {
					if n.Type.Results != nil && len(n.Type.Results.List) > 0 {
						resultsRepkg = string(RewriteAstNodeTextHooked(n.Type.Results, getContentByPos, r, hook))
					}
					return
				},
			})

			// traverse to export unexported names
			for _, arg := range rc.AllFields {
				addType(arg.Type.ResolvedType)
			}
		}
		return true
	})
	noChange = len(funcDetails) == 0
	if noChange {
		return
	}

	// name -> EXPORTED name
	needExportNames := make(map[string]string)
	allExportNames := make(map[string]string)
	importPkgByTypes := make(map[string]*NameAlias)
	TraverseTypes(starterTypes, func(t types.Type) bool {
		n, ok := t.(*types.Named)
		if !ok {
			return true //continue
		}
		pkg := n.Obj().Pkg()
		if pkg != nil { // int,error's pkg is nil
			impPkgPath := pkg.Path()
			if impPkgPath == pkgPath {
				name := n.Obj().Name()
				expName := name
				if !IsExportedName(name) {
					expName = EXPORT_PREFIX + name
					needExportNames[name] = expName
				}
				allExportNames[name] = expName
			} else {
				// find the correct name used
				if _, ok := importPkgByTypes[impPkgPath]; !ok {
					name := pkg.Name()
					alias, _ := getFileImport(f, impPkgPath)
					use := alias
					if alias == "" {
						use = name
					}
					importPkgByTypes[impPkgPath] = &NameAlias{
						Name:  name,
						Alias: alias,
						Use:   use,
					}
				}
			}
		}
		return false
	})

	// collect need exported names
	if false /*make unexported*/ {
		exportUnexported(f, fset, needExportNames, buf)
	}

	detail = &RewriteFileDetail{
		File:             f,
		FilePath:         fileName,
		Funcs:            funcDetails,
		AllExportNames:   allExportNames,
		ImportPkgByTypes: importPkgByTypes,
		GetContentByPos:  getContentByPos,
	}
	rewriteContent = buf.String()
	return
}
func IsInternalPkg(pkgPath string) bool {
	return ContainsSplitWith(pkgPath, "internal", '/')
}
func IsTestPkgOfModule(module string, pkgPath string) bool {
	if !strings.HasPrefix(pkgPath, module) {
		panic(fmt.Errorf("pkgPath %s not child of %s", pkgPath, module))
	}
	x := strings.TrimPrefix(pkgPath[len(module):], "/")
	return strings.HasPrefix(x, "test") && (len(x) == len("test") || x[len("test")] == '/')
}

// Module is nil, and ends with .test or _test
func IsGoTestPkg(pkg *packages.Package) bool {
	// may even have pkg.Name == "main", or pkg.Name == "xxx"(defined in your package)
	// actually just to check the "forTest" property can confirm that, but that field is not exported.
	return pkg.Module == nil && (strings.HasSuffix(pkg.PkgPath, ".test") || strings.HasSuffix(pkg.PkgPath, "_test"))
}

func IsVendor(modDir string, subPath string) bool {
	if modDir == "" || subPath == "" {
		return false
	}
	if !strings.HasPrefix(subPath, modDir) {
		return false
	}
	rel := subPath[len(modDir):]
	vdr := "/vendor/"
	if subPath[len(subPath)-1] == '/' {
		vdr = "vendor/"
	}
	return strings.HasPrefix(rel, vdr)
}

func ContainsSplitWith(s string, e string, split byte) bool {
	if e == "" {
		return false
	}
	idx := strings.Index(s, e)
	if idx < 0 {
		return false
	}
	eidx := idx + len(e)
	return (idx == 0 || s[idx-1] == split) && (eidx == len(s) || s[eidx] == '/')
}

// /a/b/c  ->  /a ok
// /a/b/c  ->  /am !ok
// /a/b/c/ -> /a/b/c ok
// /a/b/cd -> /a/b/c !ok
func HasPrefixSplit(s string, e string, split byte) bool {
	if e == "" {
		return s == ""
	}
	ns, ne := len(s), len(e)
	if ns < ne {
		return false
	}
	for i := 0; i < ne; i++ {
		if s[i] != e[i] {
			return false
		}
	}
	return ns == ne || s[ne] == split
}

func refInternalPkg(typesInfo *types.Info, n *ast.FuncDecl) bool {
	found := false
	// special case, if the function references to internal
	// packages,we skip
	ast.Inspect(n.Type, func(n ast.Node) bool {
		if found {
			return false
		}
		if x, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := x.X.(*ast.Ident); ok {
				ref := typesInfo.Uses[id]
				if pkgName, ok := ref.(*types.PkgName); ok {
					extPkgPath := pkgName.Pkg().Path()
					if IsInternalPkg(extPkgPath) {
						found = true
						return false
					}
				}
			}
		}
		return true
	})
	return found
}

func joinRecvArgs(recvCode string, args string, recvOrigName string, needEmptyName bool, argLen int) string {
	recvCode = strings.TrimPrefix(recvCode, "(")
	recvCode = strings.TrimSuffix(recvCode, ")")
	comma := ""
	if argLen > 0 {
		comma = ","
	}
	if recvOrigName == "" && needEmptyName { // no name,given _
		recvCode = "_ " + recvCode
	}

	return "(" + recvCode + comma + strings.TrimPrefix(args, "(")
}

// name -> EXPORTED name
func exportUnexported(f *ast.File, fset *token.FileSet, needExportNames map[string]string, buf *edit.Buffer) {
	for _, decl := range f.Decls {
		if gdecl, ok := decl.(*ast.GenDecl); ok && gdecl.Tok == token.TYPE {
			for _, spec := range gdecl.Specs {
				if tspec, ok := spec.(*ast.TypeSpec); ok {
					exportedName := needExportNames[tspec.Name.Name]
					if exportedName != "" {
						buf.Insert(OffsetOf(fset, gdecl.End()), fmt.Sprintf(";type %s = %s", exportedName, tspec.Name.Name))
					}
				}
			}
		}
	}
}

type NamedType struct {
	t *types.Named
}

func NewNamedType(t *types.Named) *NamedType {
	return &NamedType{t}
}

func initRewriteConfig(pkg *packages.Package, decl *ast.FuncDecl, skipNonCtx bool) *RewriteConfig {
	pkgPath := pkg.PkgPath
	rc := &RewriteConfig{
		Names:         make(map[string]bool),
		SupportPkgRef: "_mock",
		VarPrefix:     "_mock",
		Pkg:           pkgPath,
		FuncName:      decl.Name.Name,
	}
	rc.Exported = IsExportedName(rc.FuncName)

	firstIsCtx := false
	params := decl.Type.Params
	if params != nil && len(params.List) > 0 {
		firstParam := params.List[0]
		var firstName string
		if len(firstParam.Names) > 0 {
			firstName = firstParam.Names[0].Name
		}

		if TokenHasQualifiedName(pkg, firstParam.Type, "context", "Context") {
			firstIsCtx = true
			if firstName == "" || firstName == "_" {
				// not give
				rc.CtxName = "ctx"
			} else {
				rc.CtxName = firstName
			}
		}
	}
	if !firstIsCtx && skipNonCtx {
		return nil
	}

	// at least one res has names
	rc.ResultsNameGen = !hasName(decl.Type.Results)

	lastIsErr := false
	if decl.Type.Results != nil && len(decl.Type.Results.List) > 0 {
		lastRes := decl.Type.Results.List[len(decl.Type.Results.List)-1]
		var lastName string
		if len(lastRes.Names) > 0 {
			lastName = lastRes.Names[len(lastRes.Names)-1].Name
		}
		retType := pkg.TypesInfo.TypeOf(lastRes.Type)

		if HasQualifiedName(retType, "", "error") {
			lastIsErr = true
			if lastName == "" || lastName == "_" {
				rc.ErrName = "err"
			} else {
				rc.ErrName = lastName
			}
		}
	}

	typeInfo := make(map[types.Type]*Type)

	rc.FirstArgIsCtx = firstIsCtx
	rc.LastResIsError = lastIsErr
	if decl.Type.Params != nil {
		rc.FullArgs = parseTypes(decl.Type.Params.List, pkg, "unused_", typeInfo)
	}

	// return may be empty
	if decl.Type.Results != nil {
		rc.FullResults = parseTypes(decl.Type.Results.List, pkg, "Resp_", typeInfo)
	}

	rc.Args = rc.FullArgs
	rc.Results = rc.FullResults
	if firstIsCtx {
		rc.FullArgs[0].Name = rc.CtxName
		rc.Args = rc.FullArgs[1:]
	}
	if lastIsErr {
		rc.FullResults[len(rc.Results)-1].Name = rc.ErrName
		rc.Results = rc.Results[:len(rc.Results)-1]
	}

	// recv
	rc.Recv = parseRecv(decl, pkg, typeInfo)
	if rc.Recv != nil {
		rc.OwnerPtr, rc.Owner = rc.Recv.Type.Ptr, rc.Recv.Type.Name
		rc.AllFields = append(rc.AllFields, rc.Recv)
	}

	rc.AllFields = append(rc.AllFields, rc.FullArgs...)
	rc.AllFields = append(rc.AllFields, rc.FullResults...)

	rc.NewFuncName = "_mock" + rc.GetFullName()

	// find existing names
	// if any fieldName conflicts with an existing name,
	// we should rename it.
	// for example :
	//     func (dao *impl) Find(filter dao.Filter)
	usedSelector := make(map[string]bool)
	ast.Inspect(decl.Type, func(n ast.Node) bool {
		if x, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := x.X.(*ast.Ident); ok {
				usedSelector[id.Name] = true
			}
		}
		return true
	})

	// unique names
	allVisible := true
	for _, field := range rc.AllFields {
		allVisible = allVisible && field.Type.Visible
		// will not modify original names as they are already validated
		if field.OrigName != "" && field.OrigName != "_" && !usedSelector[field.OrigName] {
			rc.Names[field.Name] = true
			field.ExportedName = ToExported(field.Name)
			continue
		}

		field.Name = nextName(func(k string) bool {
			if rc.Names[k] || usedSelector[k] {
				return false
			}
			rc.Names[k] = true
			return true
		}, field.Name)
		field.ExportedName = ToExported(field.Name)
	}

	// CtxName ,ErrName,
	if rc.FirstArgIsCtx {
		rc.CtxName = rc.FullArgs[0].Name
	}
	if rc.LastResIsError {
		rc.ErrName = rc.FullResults[len(rc.FullResults)-1].Name
	}

	return rc
}

func (c *RewriteConfig) GetFullName() string {
	if c.Recv != nil {
		return c.Owner + "_" + c.FuncName
	}
	return c.FuncName
}

func parseRecv(decl *ast.FuncDecl, pkg *packages.Package, typeInfo map[types.Type]*Type) *Field {
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		if len(decl.Recv.List) != 1 {
			panic(fmt.Errorf("multiple receiver found:%s", decl.Name.Name))
		}
		return parseTypes(decl.Recv.List, pkg, "unused_Recv", typeInfo)[0]
	}
	return nil
}

const EXPORT_PREFIX = "MExport_"

func parseTypes(list []*ast.Field, pkg *packages.Package, genPrefix string, typeInfo map[types.Type]*Type) []*Field {
	// if ignoreFirst {
	// 	if len(list) == 0 {
	// 		panic(fmt.Errorf("ignoreFirst set but len(list)==0"))
	// 	}
	// 	list = list[1:]
	// }
	// if ignoreLast {
	// 	if len(list) == 0 {
	// 		panic(fmt.Errorf("ignoreLast set but len(list)==0"))
	// 	}
	// 	list = list[:len(list)-1]
	// }
	fields := make([]*Field, 0, len(list))

	forEachName(list, func(i int, nameNode *ast.Ident, name string, t ast.Expr) {
		fName := name
		origName := fName
		if fName == "_" || fName == "" {
			fName = fmt.Sprintf("%s%d", genPrefix, i)
		}
		rtype := resolveType(pkg, t)
		tIsPtr, tName := typeName(pkg, rtype)

		_, ellipsis := t.(*ast.Ellipsis)

		typeInfoCache := typeInfo[rtype]
		if typeInfoCache == nil {
			exported := IsExportedName(tName)
			exportedName := tName
			if !exported {
				exportedName = EXPORT_PREFIX + tName
			}
			foundInvisible := false
			TraverseType(rtype, func(t types.Type) bool {
				if foundInvisible {
					return false
				}
				n, ok := t.(*types.Named)
				if !ok {
					return true
				}
				// error has no package
				if n.Obj().Pkg() != nil && (!IsExportedName(n.Obj().Name()) || IsInternalPkg(n.Obj().Pkg().Path())) {
					// TODO: get aliased name, may can use that alias is that is exported
					// if n.Obj().IsAlias()
					foundInvisible = true
				}

				// since it is named, so a name stop's traversing its underlying.
				return false
			})
			typeInfoCache = &Type{
				Ptr:          tIsPtr,
				Name:         tName,
				Exported:     exported,
				ExportedName: exportedName, // TODO: fix for error, MExport_error is not correct
				ResolvedType: rtype,
				Visible:      !foundInvisible,
			}
			typeInfo[rtype] = typeInfoCache
		}

		fields = append(fields, &Field{
			Name:     fName,
			OrigName: origName,
			NameNode: nameNode,
			Type:     typeInfoCache,
			TypeExpr: t,
			Ellipsis: ellipsis,
		})
	})
	return fields
}

// NOTE: a replacement of implements. No successful try made yet.
// TODO: test types.AssignableTo() for types from the same Load.
func HasQualifiedName(t types.Type, pkg, name string) bool {
	switch t := t.(type) {
	case *types.Named:
		o := t.Obj()
		p := o.Pkg()
		if (p == nil && pkg != "") || (p != nil && p.Path() != pkg) {
			return false
		}
		return o.Name() == name
	}
	return false
}

func TokenHasQualifiedName(p *packages.Package, t ast.Expr, pkg string, name string) bool {
	argType := p.TypesInfo.TypeOf(t)
	return HasQualifiedName(argType, pkg, name)
}

func hasName(fields *ast.FieldList) bool {
	if fields != nil && len(fields.List) > 0 {
		for _, res := range fields.List {
			for _, x := range res.Names {
				if x.Name != "" {
					return true
				}
			}
		}
	}
	return false
}

func formatAssign(dst []string, colon bool, src string) string {
	if len(dst) == 0 {
		return src
	}
	eq := "="
	if colon {
		eq = ":="
	}
	return fmt.Sprintf("%s %s %s", strings.Join(dst, " "), eq, src)
}

// names must be pre-assigned
// the rewritter will change all _ names to generated names, like '_unusedReq_${i}', '_unusedResp_${i}`
// these names will not appear to mock json.
type RewriteConfig struct {
	Names         map[string]bool // declared names(unique)
	SupportPkgRef string          // _mock
	VarPrefix     string          // _mock
	Pkg           string
	Owner         string // the owner type name (always inside Pkg)
	OwnerPtr      bool   // is owner type a pointer type?
	Exported      bool   // is name exported?
	FuncName      string
	NewFuncName   string
	// HasCtx always be true
	CtxName        string // if "", has no ctx. if "_", should adjust outside this config
	ErrName        string // if "", has no error.
	ResultsNameGen bool   // Results names generated ?
	FirstArgIsCtx  bool
	LastResIsError bool
	Recv           *Field
	FullArgs       FieldList
	FullResults    FieldList
	AllFields      FieldList // all fields, including recv(if any),args,results
	Args           FieldList
	Results        FieldList
}

type Signature struct {
	Args           string
	ArgRecvMayIntf string // version where Recv changed to interface{}
	Results        string
}

func (c *Signature) String() string {
	return fmt.Sprintf("func%s%s", c.Args, c.Results)
}

// not needed
// func (c *Signature) StringIntf() string {
// 	return fmt.Sprintf("func%s%s", c.ArgRecvMayIntf, c.Results)
// }

type FieldList []*Field

func (c FieldList) ForEachField(ignoreFirst, ignoreLast bool, fn func(f *Field)) {
	for i, f := range c {
		if ignoreFirst && i == 0 {
			continue
		}
		if ignoreLast && i == len(c)-1 {
			continue
		}
		fn(f)
	}
}

// are all fields visible to outside? meaning it does not reference unexported names.
// this function can be added to generated mock only when `AllTypesVisible` is true.
// even when its name is unexported.
func (c FieldList) AllTypesVisible() bool {
	for _, f := range c {
		if !f.Type.Visible {
			return false
		}
	}
	return true
}
func (c FieldList) RenameFields(fset *token.FileSet, buf *edit.Buffer) {
	for _, f := range c {
		f.Rename(fset, buf)
	}
}
func (c *Field) Rename(fset *token.FileSet, buf *edit.Buffer) {
	// always rewrite if: origName does not exist, or has been renamed
	if c.OrigName == "" || c.OrigName == "_" || c.OrigName != c.Name {
		if c.NameNode == nil {
			buf.Insert(OffsetOf(fset, c.TypeExpr.Pos()), c.Name+" ")
		} else {
			buf.Replace(OffsetOf(fset, c.NameNode.Pos()), OffsetOf(fset, c.NameNode.End()), c.Name)
		}
	}
}
func (c FieldList) FillFieldTypeExpr(fset *token.FileSet, content []byte) {
	for _, f := range c {
		f.TypeExprString = string(getContent(fset, content, f.TypeExpr.Pos(), f.TypeExpr.End()))
	}
}

func forEachName(list []*ast.Field, fn func(i int, nameNode *ast.Ident, name string, t ast.Expr)) {
	i := 0
	for _, e := range list {
		if len(e.Names) > 0 {
			for _, n := range e.Names {
				fn(i, n, n.Name, e.Type)
				i++
			}
		} else {
			fn(i, nil, "", e.Type)
			i++
		}
	}
}

type Field struct {
	Name         string // original name or generated name
	ExportedName string // exported version of Name
	NameNode     *ast.Ident
	// Type     *Type
	OrigName string // original name

	Type           *Type
	TypeExpr       ast.Expr // the type expr,indicate the position. maybe *ast.Indent, *ast.SelectorExpr or recursively
	TypeExprString string
	Ellipsis       bool // when true, TypeExpr is *ast.Ellipsis, and ResolvedType is slice type(unnamed)
}

type Type struct {
	Ptr          bool
	Name         string
	Exported     bool   // true if original Name is exported
	ExportedName string // if !Exported, the generated name

	ResolvedType types.Type
	Visible      bool // visible to outside? either is an exported name, or name from another non-internal package, or contains names of such. TODO: add internal detection.
}

func (c *RewriteConfig) Init() {
	if c.SupportPkgRef == "" {
		c.SupportPkgRef = "_mock"
	}
	if c.VarPrefix == "" {
		c.VarPrefix = "_mock"
	}
}

func (c *RewriteConfig) Validate() {
	// if c.CtxName == "" {
	// 	panic(fmt.Errorf("no ctx var, pkg:%v, owner:%v, func:%v", c.Pkg, c.Owner, c.FuncName))
	// }
	if c.CtxName == "_" {
		panic(fmt.Errorf("var ctx must not be _"))
	}
	if c.NewFuncName == "" {
		panic(fmt.Errorf("NewFuncName must not be empty"))
	}
	for i, arg := range c.Args {
		if arg.Name == "" {
			panic(fmt.Errorf("arg name %d is empty", i))
		}
	}
	for i, res := range c.Results {
		if res.Name == "" {
			panic(fmt.Errorf("results %d is empty", i))
		}
	}
}

func (c *RewriteConfig) Gen(pretty bool) string {
	c.Validate()

	resNames := make([]string, 0, len(c.Results))
	for _, res := range c.Results {
		resNames = append(resNames, res.Name)
	}

	makeStructDefs := func(c FieldList) string {
		reqDefList := make([]string, 0, len(c))
		for _, f := range c {
			typeExpr := f.TypeExprString
			if f.Ellipsis {
				typeExpr = "[]" + strings.TrimPrefix(typeExpr, "...") // hack: replace ... with []
			}
			reqDefList = append(reqDefList, fmt.Sprintf("%v %s `json:%v`", f.ExportedName, typeExpr, strconv.Quote(f.Name)))
		}

		structDefs := strings.Join(reqDefList, ";")
		if len(structDefs) > 0 {
			structDefs = structDefs + ";"
		}
		return structDefs
	}

	getFieldAssigns := func(fields []*Field) string {
		assignList := make([]string, 0, len(fields))
		for _, arg := range fields {
			assignList = append(assignList, fmt.Sprintf("%v: %v", arg.ExportedName, arg.Name))
		}
		reqDefs := strings.Join(assignList, ",")
		return reqDefs
	}
	getResFields := func(fields []*Field, base string) string {
		assignList := make([]string, 0, len(fields))
		for _, arg := range fields {
			assignList = append(assignList, fmt.Sprintf("%s.%s", base, arg.ExportedName))
		}
		reqDefs := strings.Join(assignList, ",")
		return reqDefs
	}
	recvVar := "nil"
	if c.Recv != nil {
		recvVar = c.Recv.Name
	}

	varMap := gen.VarMap{
		"__V__":               c.VarPrefix,
		"__P__":               c.SupportPkgRef,
		"__RECV_VAR__":        recvVar,
		"__PKG_NAME_Q__":      strconv.Quote(c.Pkg),
		"__OWNER_NAME_Q__":    strconv.Quote(c.Owner),
		"__OWNER_IS_PTR__":    strconv.FormatBool(c.OwnerPtr),
		"__FUNC_NAME_Q__":     strconv.Quote(c.FuncName),
		"__NEW_FUNC__":        c.NewFuncName,
		"__ERR_NAME__":        c.ErrName,
		"__REQ_DEFS__":        makeStructDefs(c.Args),
		"__RESP_DEFS__":       makeStructDefs(c.Results),
		"__RES_NAMES__":       strings.Join(resNames, ","),
		"__REQ_DEF_ASSIGN__":  getFieldAssigns(c.Args),
		"__MOCK_RES_FIELDS__": getResFields(c.Results, fmt.Sprintf("%sresp", c.VarPrefix)),
		"__HAS_RECV__":        strconv.FormatBool(c.Recv != nil),
		"__FIRST_IS_CTX__":    strconv.FormatBool(c.FirstArgIsCtx),
		"__LAST_IS_ERR__":     strconv.FormatBool(c.LastResIsError),
	}
	t := gen.NewTemplateBuilder()
	t.Block(
		"var __V__req = struct{__REQ_DEFS__}{__REQ_DEF_ASSIGN__}",
		"var __V__resp struct{__RESP_DEFS__}",
		// func TrapFunc(ctx context.Context, stubInfo *StubInfo, inst interface{}, req interface{}, resp interface{}, oldFunc interface{}, hasRecv bool, firstIsCtx bool, lastIsErr bool) error
		gen.Group(
			gen.If(c.ErrName != "").Then("__ERR_NAME__ = "),
			gen.Group(
				"__P__.TrapFunc(",
				gen.If(c.CtxName != "").Then(c.CtxName).Else("nil"), ",",
				"&__P__.StubInfo{PkgName:__PKG_NAME_Q__,Owner:__OWNER_NAME_Q__,OwnerPtr:__OWNER_IS_PTR__,Name:__FUNC_NAME_Q__}, __RECV_VAR__, &__V__req, &__V__resp,__NEW_FUNC__,__HAS_RECV__,__FIRST_IS_CTX__,__LAST_IS_ERR__)",
			),
		),
		gen.If(len(c.Results) > 0).Then(
			"__RES_NAMES__ = __MOCK_RES_FIELDS__",
		),
		gen.If(len(c.Results) > 0 || c.ErrName != "").Then(
			"return",
		),
	)

	t.Pretty(pretty)

	// generate rule is that,  when no pretty, shoud
	// add ';' after each statement, unless that statements ends with '{'
	return t.Format(varMap)
}

func genMockStub(p *packages.Package, fileDetails []*RewriteFileDetail) (content string, err error) {
	imps := NewImportList()

	preMap := map[string]bool{
		"Setup":         true,
		"M":             true,
		SKIP_MOCK_FILE:  true,
		SKIP_MOCK_PKG:   true,
		"FULL_PKG_NAME": true, // TODO: may add go keywords.
	}
	imps.CanUseName = func(name string) bool {
		return !preMap[name]
	}

	var links gen.Statements
	var defs gen.Statements

	type decAndLink struct {
		decl *gen.Statements
		link *gen.Statements
	}
	defByOwner := make(map[string]*decAndLink)

	// var rePkg AstNodeRewritter
	codeWillBeCommented := false
	var interfacedIdent map[ast.Node]bool
	// rePkg, and also given a name
	rePkg := func(node ast.Node, getNodeText func(start token.Pos, end token.Pos) []byte) ([]byte, bool) {
		_, ok := interfacedIdent[node]
		if ok {
			return []byte("interface{}"), true
		}
		if idt, ok := node.(*ast.Ident); ok {
			ref := p.TypesInfo.Uses[idt]
			// is it a Type declared in current package?
			if t, ok := ref.(*types.TypeName); ok {
				realPkg := t.Pkg()  // may be dot import
				if realPkg != nil { // string will have no pkg
					refPkgName := realPkg.Name()
					if !codeWillBeCommented {
						refPkgName = imps.ImportOrUseNext(realPkg.Path(), "", realPkg.Name())
					}
					return []byte(fmt.Sprintf("%s.%s", refPkgName, idt.Name)), true
				}
			}
		} else if sel, ok := node.(*ast.SelectorExpr); ok {
			// external pkg
			// debugShow(p.TypesInfo, sel)
			if idt, ok := sel.X.(*ast.Ident); ok {
				ref := p.TypesInfo.Uses[idt]
				if pkgName, ok := ref.(*types.PkgName); ok {
					extPkgName := pkgName.Name()
					if !codeWillBeCommented {
						extPkgName = imps.ImportOrUseNext(pkgName.Imported().Path(), pkgName.Name(), pkgName.Imported().Name())
					}
					return []byte(fmt.Sprintf("%s.%s", extPkgName, sel.Sel.Name)), true
				}
			}
		}
		return nil, false
	}

	noOwnerDef := &decAndLink{decl: &gen.Statements{}, link: &gen.Statements{}}
	defs.Append(noOwnerDef.decl)
	links.Append(noOwnerDef.link)
	defByOwner[""] = noOwnerDef
	hasRefX := false
	for _, fd := range fileDetails {
		for _, d := range fd.Funcs {
			rc := d.RewriteConfig

			oname := ""
			if rc.Owner != "" {
				oname = rc.Owner
				if !rc.Recv.Type.Exported {
					oname = "M_" + oname
				}
			}

			// don't add mock stub for invisible functions
			// because the user cannot easily reference invisible
			// functions
			// we add a comment here
			defSt, ok := defByOwner[rc.Owner]
			if !ok {
				// create for new owner
				defSt = &decAndLink{decl: &gen.Statements{}, link: &gen.Statements{}}
				if rc.Owner != "" {
					defs.Append(
						fmt.Sprintf("    %s struct{", oname),
						gen.Indent("        ", defSt.decl),
						"    }",
					)
					links.Append(
						fmt.Sprintf(`    "%s":map[string]interface{}{`, oname),
						gen.Indent("         ", defSt.link),
						"    },",
					)
				} else {
					defs.Append(gen.Indent("    ", defSt.decl))
					links.Append(gen.Indent("    ", defSt.link))
				}
				// always append impl
				defByOwner[rc.Owner] = defSt
			}

			refFuncName := rc.FuncName
			if !rc.Exported {
				refFuncName = "M_" + refFuncName
			}
			codeWillBeCommented = !rc.FullArgs.AllTypesVisible() || !rc.FullResults.AllTypesVisible()
			interfacedIdent = map[ast.Node]bool(nil)

			var renameHook func(node ast.Node, c []byte) []byte
			if rc.Recv != nil && len(rc.FullArgs) > 0 &&
				(rc.Recv.OrigName == "" && rc.FullArgs[0].OrigName != "" || rc.Recv.OrigName != "" && rc.FullArgs[0].OrigName == "") {
				prefixMap := make(map[ast.Node][]byte, 1)
				// args has no name, but recv has name
				for _, arg := range rc.FullArgs {
					if arg.OrigName == "" {
						prefixMap[arg.TypeExpr] = []byte("_ ")
					}
				}
				if rc.Recv.OrigName == "" {
					prefixMap[rc.Recv.TypeExpr] = []byte("_ ")
				}
				renameHook = func(node ast.Node, c []byte) []byte {
					prefix, ok := prefixMap[node]
					if !ok {
						return c
					}
					x := append([]byte(nil), prefix...)
					return append(x, c...)
				}
			}
			if rc.Recv != nil && !rc.Recv.Type.Exported {
				if interfacedIdent == nil {
					interfacedIdent = make(map[ast.Node]bool, 1)
				}
				interfacedIdent[rc.Recv.TypeExpr] = true
			}

			args := d.ArgsRewritter(rePkg, CombineHooks(renameHook))
			results := d.ResultsRewritter(rePkg, nil)
			if codeWillBeCommented {
				// add decl statements
				list := strings.Split(fmt.Sprintf("%s func%s%s", refFuncName, args, results), "\n")
				list[len(list)-1] += fmt.Sprintf("// NOTE: %s contains invisible types", refFuncName)
				defSt.decl.Append(
					gen.Indent("//     ", list),
				)
			} else {
				defSt.decl.Append(fmt.Sprintf("    %s func%s%s", refFuncName, args, results))

				// don't add link for unexported type
				if rc.Exported && (rc.Recv == nil || rc.Recv.Type.Exported) {
					usePkgName := imps.ImportOrUseNext(p.PkgPath, "", p.Name)
					xref := ""
					ref := ""
					if rc.Recv != nil {
						// unused code, but leave here for future optimization
						if !rc.Recv.Type.Exported {
							ref = fmt.Sprintf("((*%s)(nil)).%s", rc.Recv.Type.Name /*use internal name*/, rc.FuncName)
						} else {
							ref = fmt.Sprintf("((*%s.%s)(nil)).%s", usePkgName, rc.Recv.Type.Name, rc.FuncName)
						}
						xref = fmt.Sprintf("e.%s.%s", oname, refFuncName)
					} else {
						ref = fmt.Sprintf("%s.%s", usePkgName, rc.FuncName)
						xref = fmt.Sprintf("e.%s", refFuncName)
					}
					hasRefX = true
					defSt.link.Append(fmt.Sprintf(`    "%s": Pair{%s,%s},`, refFuncName, xref, ref))
				}
			}
		}
	}

	// should traverse all types from args and results, finding referenced types:
	// - types in the same package
	//   -- exported: just delcare a name reference
	//   -- unexported: make an exported alias, and declare that
	// - types from another package
	//   -- must be exported: just import the package and name
	// - types from internal package
	// name conflictions may be processed later.
	var typeAlias []string
	if false /*need alias type*/ {
		typeAliased := make(map[string]bool)
		for _, fd := range fileDetails {
			for name, exportName := range fd.AllExportNames {
				if !typeAliased[name] {
					typeAliased[name] = true
					imps.ImportOrUseNext(p.PkgPath, "", p.Name)
					typeAlias = append(typeAlias, fmt.Sprintf("type %s = %s.%s", name, p.Name, exportName))
				}
			}
		}
	}

	// import predefined packages in the end
	// we try to not rename packages.
	ctxName := imps.ImportOrUseNext("context", "", "context")
	// reflectName := imps.ImportOrUseNext("reflect", "", "reflect")
	mockName := imps.ImportOrUseNext(MOCK_PKG, "_mock", "mock")

	varMap := gen.VarMap{
		"__PKG_NAME__": p.Name,
		"__FULL_PKG__": p.PkgPath,
		"__CTXP__":     ctxName,
		// "__REFLECTP__": reflectName,
		"__MOCKP__": mockName,
	}
	// example
	// type M interface {
	// 	context.Context
	// 	A(a interface{}) M
	// 	B(b interface{}) M
	// }
	// type A interface {
	// 	M(ctx context.Context) M
	// }
	// var a A
	// ctx = a.M(nil).A(nil).B(nil)
	t := gen.NewTemplateBuilder()
	t.Block(
		`// Code generated by go-mock; DO NOT EDIT.`,
		"",
		"package __PKG_NAME__",
		"",
		"import (",
		gen.Indent("    ", imps.SortedList()),
		")",
		"",
		fmt.Sprintf(`const %s = true`, SKIP_MOCK_PKG),
		`const FULL_PKG_NAME = "__FULL_PKG__"`,
		"",
		// usage:
		// ctx := mock_xx.Setup(ctx,func(ctx,t){
		//	     t.X = X
		//       t.Y = Y
		// })
		"func Setup(ctx __CTXP__.Context,setup func(m *M)) __CTXP__.Context {",
		"    m:=M{}",
		"    setup(&m)",
		`    return __MOCKP__.WithMockSetup(ctx,FULL_PKG_NAME,m)`,
		"}",
		"",
		typeAlias,
		"",
		"type M struct {",
		defs,
		"}",
		"",
		"/* prodives quick link */",
		// pre-grouped
		gen.Group(
			"var _ = func() { type Pair [2]interface{};",
			gen.If(hasRefX).Then("e:=M{};"),
			"_ = map[string]interface{}{",
		),
		links,
		"}}",
		"",
	)
	content = t.Format(varMap)

	return
}
