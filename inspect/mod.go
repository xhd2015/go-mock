package inspect

import (
	"fmt"

	"github.com/xhd2015/go-mock/sh"
)

type Module struct {
	Path    string
	Version string
}

type Replace struct {
	Old Module
	New Module
}
type GoMod struct {
	Module  *Module // no version in toplevel Module.
	Go      string  // go version,like: 1.13
	Require []*Module
	Replace []Replace
}
type CmdError struct {
	Err string
}
type ListPackage struct {
	Dir        string
	ImportPath string
	Goroot     bool     // is this package in the Go root?
	Standard   bool     // is this package part of the standard Go library?
	ForTest    string   // package is only for use in named test
	DepOnly    bool     // package is only a dependency, not explicitly listed
	Deps       []string // all (recursively) imported dependencies
	Incomplete bool     // this package or a dependency has an error
}

type ListModule struct {
	Path      string
	Version   string
	Dir       string
	GoVersion string    // like "1.17"
	Error     *CmdError // example:	"Err": "module golang.org/x/tools2: not a known dependency"
}

func GetGoMod(dir string) (*GoMod, error) {
	var cmds []string
	if dir != "" {
		cmds = append(cmds, fmt.Sprintf("cd %s", sh.Quote(dir)))
	}
	cmds = append(cmds, "go mod edit -json")
	gomod := &GoMod{}
	_, _, err := sh.RunBashWithOpts(cmds, sh.RunBashOptions{
		Verbose:      false, // don't make read verbose
		StdoutToJSON: gomod,
	})
	if err != nil {
		return nil, err
	}
	return gomod, nil
}

func GoListModule(dir string, modulePath string) (*ListModule, error) {
	if modulePath == "" {
		panic(fmt.Errorf("go list -m empty modulePath"))
	}
	var cmds []string
	if dir != "" {
		cmds = append(cmds, fmt.Sprintf("cd %s", sh.Quote(dir)))
	}
	cmds = append(cmds, fmt.Sprintf("go list -mod=mod -e -m -json %s", sh.Quote(modulePath)))
	listMod := &ListModule{}
	_, _, err := sh.RunBashWithOpts(cmds, sh.RunBashOptions{
		StdoutToJSON: listMod,
	})
	if err != nil {
		return nil, err
	}
	return listMod, nil
}
