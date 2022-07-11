package cmdsupport

import (
	"testing"

	"github.com/xhd2015/go-mock/inspect"
)

// go test -run '^TestGenRewrite$' -v  ./support/xgo/cmd
func TestGenRewrite(t *testing.T) {
	GenRewrite([]string{"../inspect/testdata/inspect_rewrite.go"}, GetRewriteRoot(), &GenRewriteOptions{
		RewriteOptions: &inspect.RewriteOptions{
			Filter: func(pkgPath, fileName, ownerName string, ownerIsPtr bool, funcName string) bool {
				if funcName == "Run" {
					return true
				}
				return false
			},
		},
	})
}

// go test -run '^TestBuildRewrite$' -v  ./support/xgo/cmd
func TestBuildRewrite(t *testing.T) {
	BuildRewrite(
		[]string{"../inspect/testdata/inspect_rewrite.go"},
		&GenRewriteOptions{
			RewriteOptions: &inspect.RewriteOptions{
				Filter: func(pkgPath, fileName, ownerName string, ownerIsPtr bool, funcName string) bool {
					if funcName == "Run" {
						return true
					}
					return false
				},
			},
		},
		&BuildOptions{
			Debug: true,
			ProjectRoot: "../../..",
		},
	)
}
