package cmdsupport

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/xhd2015/go-mock/inspect"
)

func PrintRewrite(file string, printRewrite bool, printMock bool, opts *inspect.RewriteOptions) {
	if file == "" {
		panic(fmt.Errorf("requires file"))
	}
	if !strings.HasSuffix(file, ".go") {
		panic(fmt.Errorf("requires go file"))
	}
	absFile, err := toAbsPath(file)
	if err != nil {
		panic(fmt.Errorf("make file absolute error:%v", err))
	}
	stat, err := os.Stat(absFile)
	if err != nil {
		panic(fmt.Errorf("file does not exist:%v %v", file, err))
	}
	if stat.IsDir() {
		panic(fmt.Errorf("path is a directory, expecting a file:%v", file))
	}

	projectDir := path.Dir(absFile)
	for projectDir != "/" {
		modStat, ferr := os.Stat(path.Join(projectDir, "go.mod"))
		if ferr == nil && !modStat.IsDir() {
			break
		}
		projectDir = path.Dir(projectDir)
		continue
	}
	if projectDir == "/" {
		panic(fmt.Errorf("no go.mod found for file:%v", file))
	}

	rel,ok := inspect.RelPath(projectDir,absFile)
	if !ok{
		panic(fmt.Errorf("%s not child of module:%s",absFile,projectDir))
	}
	
	loadPkg := "./" + strings.TrimPrefix(path.Dir(rel),"./")
	fset, starterPkgs, err := inspect.LoadPackages([]string{loadPkg}, &inspect.LoadOptions{
		ProjectDir: projectDir,
	})
	if err!=nil{
		panic(fmt.Errorf("loading packages error:%v",err))
	}

	contents := inspect.RewritePackages(fset, starterPkgs, opts)
	var foundContent *inspect.FileContentError
	var mockContent string
	for _, pkgRes := range contents {
		// generate rewritting files
		foundContent = pkgRes.Files[absFile]
		if foundContent != nil {
			mockContent = pkgRes.MockContent
			break
		}
	}
	if foundContent == nil {
		fmt.Fprintf(os.Stderr, "no content\n")
		return
	}
	if foundContent.Error != nil {
		panic(foundContent.Error)
	}

	if printRewrite {
		if printMock {
			fmt.Printf("// rewrite of %s:\n", absFile)
		}
		fmt.Print(string(foundContent.Content))
	}

	if printMock {
		if printRewrite {
			fmt.Printf("//\n// mock of %s:\n", absFile)
		}
		fmt.Print(string(mockContent))
	}
}
