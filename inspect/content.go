package inspect

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"io/ioutil"
	"strings"

	"golang.org/x/tools/go/packages"
)

func OffsetOf(fset *token.FileSet, pos token.Pos) int {
	if pos == token.NoPos {
		return -1
	}
	return fset.Position(pos).Offset
}

func getContent(fset *token.FileSet, content []byte, begin token.Pos, end token.Pos) []byte {
	beginOff := fset.Position(begin).Offset
	endOff := len(content)
	if end != token.NoPos {
		endOff = fset.Position(end).Offset
	}
	return content[beginOff:endOff]
}

func IsExportedName(name string) bool {
	if name == "" {
		return false
	}
	c := name[0]
	return c >= 'A' && c <= 'Z'

	// buggy: _X is not exported, but this still gets it.
	// return len(name) > 0 && strings.ToUpper(name[0:1]) == name[0:1]
}
func StripNewline(s string) string {
	return strings.ReplaceAll(s, "\n", "")
}
func ToExported(name string) string {
	if name == "" {
		return name
	}
	c := name[0]
	if c >= 'A' && c <= 'Z' {
		return name
	}
	// c is lower or other
	c1 := strings.ToUpper(name[0:1])
	if c1[0] != c {
		return c1 + name[1:]

	}
	// failed to make expored, such as "_A", just make a "M_" prefix
	return "M_" + name
}
func fileNameOf(fset *token.FileSet, f *ast.File) string {
	tokenFile := fset.File(f.Package)
	if tokenFile == nil {
		panic(fmt.Errorf("no filename of:%v", f))
	}
	return tokenFile.Name()
}

func NextName(addIfNotExists func(string) bool, name string) string {
	if addIfNotExists(name) {
		return name
	}
	for i := 1; i < 100000; i++ {
		namei := fmt.Sprintf("%s%d", name, i)
		if addIfNotExists(namei) {
			return namei
		}
	}
	panic(fmt.Errorf("nextName failed, tried 10,0000 times.name: %v", name))
}

func NextFileNameUnderDir(dir string, name string, suffix string) string {
	starterNames, _ := ioutil.ReadDir(dir)
	return NextName(func(s string) bool {
		for _, f := range starterNames {
			if f.Name() == s+suffix {
				return false
			}
		}
		return true
	}, name) + suffix
}

// empty if not set
func typeName(pkg *packages.Package, t types.Type) (ptr bool, name string) {
	ptr, named := getNamedOrPtr(t)
	if named != nil {
		name = named.Obj().Name()
	}
	return
}
func resolveType(pkg *packages.Package, t ast.Expr) types.Type {
	return pkg.TypesInfo.TypeOf(t)
}

func getNamedOrPtr(t types.Type) (ptr bool, n *types.Named) {
	if x, ok := t.(*types.Pointer); ok {
		ptr = true
		t = x.Elem()
	}
	if named, ok := t.(*types.Named); ok {
		n = named
	}
	return
}

type rewriteVisitor struct {
	parent      *rewriteVisitor // for debug
	rangeNode   ast.Node
	rewrite     func(node ast.Node, getNodeText func(start token.Pos, end token.Pos) []byte) ([]byte, bool)
	getNodeText func(start token.Pos, end token.Pos) []byte

	children       []*rewriteVisitor
	rangeRewrite   []byte
	rangeRewriteOK bool
	// childRewrite map[ast.Node][]byte
}

// during walk, c is for node
func (c *rewriteVisitor) Visit(node ast.Node) ast.Visitor {
	if node == nil { // end of current visiting
		return nil
	}
	// debug check
	if c.parent != nil && c.parent.rangeNode != nil {
		if node.Pos() < c.parent.rangeNode.Pos() {
			panic(fmt.Errorf("node begin lower than parent's:%d < %d, %v", node.Pos(), c.parent.rangeNode.Pos(), node))
		}
		if node.End() > c.parent.rangeNode.End() {
			panic(fmt.Errorf("node end bigger than parent's:%d > %d %v", node.End(), c.parent.rangeNode.End(), node))
		}
	}

	// child
	child := &rewriteVisitor{
		parent:      c,
		rangeNode:   node,
		rewrite:     c.rewrite,
		getNodeText: c.getNodeText,
	}
	c.children = append(c.children, child)
	if c.rewrite != nil {
		child.rangeRewrite, child.rangeRewriteOK = c.rewrite(node, c.getNodeText)
		if child.rangeRewriteOK {
			// this node gets written
			// do not traverse its children any more.
			return nil
		}
	}

	return child
}

func (c *rewriteVisitor) join(depth int, hook func(ast.Node, []byte) []byte) []byte {
	if c.rangeRewriteOK {
		return hook(c.rangeNode, c.rangeRewrite)
	}
	var res []byte

	off := c.rangeNode.Pos()
	// sort.Slice(c.children, func(i, j int) bool {
	// 	return c.children[i].rangeNode.Pos() < c.children[j].rangeNode.Pos()
	// })
	// check (always correct)
	// for i, ch := range c.children {
	// 	if i == 0 {
	// 		continue
	// 	}
	// 	last := c.children[i-1]
	// 	// begin of this node should be bigger than last node's end
	// 	if ch.rangeNode.Pos() < last.rangeNode.End() {
	// 		panic(fmt.Errorf("node overlap:%d", i))
	// 	}
	// }

	for _, ch := range c.children {
		n := ch.rangeNode
		nstart := n.Pos()
		nend := n.End()
		// missing slots
		// off->start
		res = append(res, c.getNodeText(off, nstart)...)

		// start->end
		res = append(res, ch.join(depth+1, hook)...)

		// update off
		off = nend
	}

	// process trailing
	if off != token.NoPos {
		res = append(res, c.getNodeText(off, c.rangeNode.End())...)
	}
	return hook(c.rangeNode, res)
}

// joining them together makes a complete statement, in string format.
func RewriteAstNodeText(node ast.Node, getNodeText func(start token.Pos, end token.Pos) []byte, rewrite AstNodeRewritter) []byte {
	return RewriteAstNodeTextHooked(node, getNodeText, rewrite, nil)
}

func RewriteAstNodeTextHooked(node ast.Node, getNodeText func(start token.Pos, end token.Pos) []byte, rewrite AstNodeRewritter, hook func(node ast.Node, c []byte) []byte) []byte {
	// traverse to get all rewrite info
	root := &rewriteVisitor{
		rewrite:     rewrite,
		getNodeText: getNodeText,
	}
	ast.Walk(root, node)
	if hook == nil {
		hook = func(node ast.Node, c []byte) []byte {
			return c
		}
	}

	// parent is responsible for fill in uncovered slots
	return root.children[0].join(0, hook)
}

// like filter, first hook gets firstly executed
func CombineHooks(hooks ...func(node ast.Node, c []byte) []byte) func(node ast.Node, c []byte) []byte {
	cur := func(node ast.Node, c []byte) []byte {
		return c
	}
	for _, _hook := range hooks {
		if _hook == nil {
			continue
		}
		hook := _hook
		last := cur
		cur = func(node ast.Node, c []byte) []byte {
			return hook(node, last(node, c))
		}
	}
	return cur
}
