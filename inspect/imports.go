package inspect

import (
	"fmt"
	"go/ast"
	"go/token"
	"sort"
	"strconv"

	"github.com/xhd2015/go-mock/code/edit"
)

// ImportList represents a list of imports
// imports allows multiple time imports of the same package with different alias, one for each import.
// but disallow the same alias of different packages.
// also, pkg name should also be checked.
// ImportList implies the merge behaviour.
type ImportList struct {
	NameMap     map[string]string // this provides the default name of the package. It must be uniq.
	PkgToUseMap map[string]string
	UseToPkgMap map[string]string // all tokens, must be uniq.
	List        []string
	CanUseName  func(name string) bool // can use this as name?
}

func NewImportList() *ImportList {
	return &ImportList{
		NameMap:     make(map[string]string),
		PkgToUseMap: make(map[string]string),
		UseToPkgMap: make(map[string]string),
	}
}

// return the effective name
// ImportOrUseNext will always succeed.
// It do extra work to ensure that only one effective name exists in the list.
// This involves rewritting
// This makes a pkg path has only one name.
func (c *ImportList) ImportOrUseNext(pkgPath string, suggestAlias string, name string) string {
	if pkgPath == "" {
		panic(fmt.Errorf("pkgPath cannot be empty"))
	}

	// always check name consistentency
	c.checkName(pkgPath, name)

	// check existing
	prevUse := c.PkgToUseMap[pkgPath]
	if prevUse != "" {
		return prevUse
	}

	// try next name
	return NextName(func(s string) bool {
		if c.CanUseName != nil && !c.CanUseName(s) {
			return false
		}
		if c.UseToPkgMap[s] != "" {
			// next name
			return false
		}
		// this means this name can be allocated to this package
		c.UseToPkgMap[s] = pkgPath
		c.PkgToUseMap[pkgPath] = s

		// default don't make alias unless we need one
		alias := s
		if alias == name {
			alias = ""
		}
		c.List = append(c.List, formatImport(pkgPath, alias))
		return true
	}, usePkgName(suggestAlias, name))
}
func (c *ImportList) SortedList() []string {
	sort.Strings(c.List)
	return c.List
}

func usePkgName(alias string, name string) string {
	if alias != "" {
		return alias
	}
	return name
}

func (c *ImportList) checkName(pkgPath, name string) {
	if name == "" {
		panic("name cannot be empty")
	}
	prevName := c.NameMap[pkgPath]
	if prevName == "" {
		if c.NameMap == nil {
			c.NameMap = make(map[string]string, 1)
		}
		c.NameMap[pkgPath] = name
	} else if prevName != name {
		panic(fmt.Errorf("inconsistent name of package:%v given:%v, previous:%v", pkgPath, name, prevName))
	}
}

func formatImport(pkgPath string, alias string) string {
	if alias != "" {
		return fmt.Sprintf("%s %q", alias, pkgPath)
	}
	return fmt.Sprintf("%q", pkgPath)
}

// ensureImports
// refName represents real name
func ensureImports(fset *token.FileSet, f *ast.File, buf *edit.Buffer, alias string, name string, path string) (refName string, exists bool) {
	if name == "" {
		panic(fmt.Errorf("requires pkg name to be known first"))
	}
	refName = name
	exAlias, exists := getFileImport(f, path)
	if exists {
		if exAlias != "" {
			refName = exAlias
		}
		return
	}
	off := OffsetOf(fset, f.Name.End())
	space := ""
	if alias != "" {
		space = " "
		refName = alias
	}
	inserts := fmt.Sprintf(";import %s%s%q", alias, space, path)
	buf.Insert(off, inserts)

	return
}

func getFileImport(f *ast.File, path string) (alias string, ok bool) {
	qpath := strconv.Quote(path)
	for _, imp := range f.Imports {
		if imp.Path.Value == qpath {
			if imp.Name != nil {
				return imp.Name.Name, true
			}
			return "", true
		}
	}
	return "", false
}
