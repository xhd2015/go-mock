package inspect

import (
	"testing"
)

func TestRun(t *testing.T) {
	runArgs([]string{"./testdata/inspect.go"}) // ok
	// runArgs([]string{"./testdata/inspect.go","context"}) // bad
	// runArgs([]string{"context"}) // ok
	// runArgs([]string{"builtin"}) // ok
	// runArgs([]string{"builtin2"}) // bad no error, but no such file
}

// go test -run '^TestRewrite$' -v  ./support/xgo/inspect/
func TestRewrite(t *testing.T) {
	res := Rewrite([]string{"./testdata/inspect_rewrite.go"}, &RewriteOptions{
		Filter: func(pkgPath, fileName, ownerName string, ownerPtr bool, funcName string) bool {
			if funcName == "Empty" {
				return true
			}
			return false
		},
	})

	for _, v := range res {
		t.Logf("package:%v", v.PkgPath)
		t.Logf("mock conent: %s",v.MockContent)
		for _, c := range v.Files {
			content, err := c.Content, c.Error
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("file:%v", c.OrigFile)
			t.Logf("file content: %s", content)
		}
	}
}

// go test -run '^TestRewriteFuncNoArg$' -v  ./support/xgo/inspect/
func TestRewriteFuncNoArg(t *testing.T) {
	rc := &RewriteConfig{
		SupportPkgRef:  "_mock",
		VarPrefix:      "_mock",
		Pkg:            "test_go_mock",
		Owner:          "",
		FuncName:       "Run",
		NewFuncName:    "_mockRun",
		CtxName:        "ctx",
		ErrName:        "err",
		ResultsNameGen: false,
		Args:           []*Field{},
		Results:        []*Field{},
	}

	code := rc.Gen(true)

	t.Logf("%s", code)
}

// go test -run '^TestRewriteFuncHasArg$' -v  ./support/xgo/inspect/
func TestRewriteFuncHasArg(t *testing.T) {
	rc := &RewriteConfig{
		SupportPkgRef:  "_mock",
		VarPrefix:      "_mock",
		Pkg:            "test_go_mock",
		Owner:          "",
		FuncName:       "Run",
		NewFuncName:    "_mockRun",
		CtxName:        "ctx",
		ErrName:        "err",
		ResultsNameGen: false,
		Args: []*Field{
			// {Name: "a", TypeExpr: "A"},
			// {Name: "b", TypeExpr: "B"},
		},
		Results: []*Field{
			// {Name: "c", TypeExpr: "C"},
			// {Name: "d", TypeExpr: "D"},
		},
	}

	code := rc.Gen(true)

	t.Logf("%s", code)
}
