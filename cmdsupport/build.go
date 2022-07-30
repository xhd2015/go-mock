package cmdsupport

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"

	"github.com/xhd2015/go-mock/sh"
)

func GetRewriteRoot() string {
	// return path.Join(os.MkdirTemp(, "go-rewrite")
	return path.Join(os.TempDir(), "go-rewrite")
}

type BuildOptions struct {
	Verbose     bool
	ProjectRoot string // default CWD
	Debug       bool
	Output      string
	ForTest     bool
	GoFlags     string
	// extra trim path map to be applied
	// cleanedModOrigAbsDir - modOrigAbsDir
	mappedMod map[string]string
	newGoROOT string
}

type BuildResult struct {
	Output string
}

func BuildRewrite(args []string, genOpts *GenRewriteOptions, opts *BuildOptions) *BuildResult {
	if opts == nil {
		opts = &BuildOptions{}
	}
	verbose := opts.Verbose
	if genOpts == nil {
		genOpts = &GenRewriteOptions{
			Verbose: verbose,
		}
	}
	genOpts.ProjectDir = opts.ProjectRoot

	res := GenRewrite(args, GetRewriteRoot(), genOpts)
	opts.mappedMod = res.MappedMod
	opts.newGoROOT = res.UseNewGOROOT
	return Build(args, opts)
}

func Build(args []string, opts *BuildOptions) *BuildResult {
	if opts == nil {
		opts = &BuildOptions{}
	}
	verbose := opts.Verbose
	debug := opts.Debug
	mappedMod := opts.mappedMod
	newGoROOT := opts.newGoROOT
	forTest := opts.ForTest
	goFlags := opts.GoFlags
	// project root
	projectRoot := ""
	if opts != nil {
		projectRoot = opts.ProjectRoot
	}
	var err error
	projectRoot, err = toAbsPath(projectRoot)
	if err != nil {
		panic(err)
	}
	// output
	output := ""
	if opts != nil && opts.Output != "" {
		output = opts.Output
		if !path.IsAbs(output) {
			output, err = toAbsPath(output)
			if err != nil {
				panic(fmt.Errorf("make abs path err:%v", err))
			}
		}
	} else {
		output = "exec"
		if debug {
			output = "debug"
		}
		if forTest {
			output = output + "-test"
		}
		output = output + ".bin"
		if !path.IsAbs(output) {
			output = path.Join(projectRoot, output)
		}
	}

	var gcflagList []string

	// root dir is errous:
	//     /path/to/rewrite-root=>/
	//     //Users/x/gopath/pkg/mod/github.com/xhd2015/go-mock/v1/src/db/impl/util.go
	//
	// so replacement must have at least one child:
	//     /path/to/rewrite-root/X=>/X
	rewriteRoot := GetRewriteRoot()
	root, err := toAbsPath(rewriteRoot)
	if err != nil {
		panic(fmt.Errorf("get absolute path failed:%v %v", rewriteRoot, err))
	}
	if debug {
		gcflagList = append(gcflagList, "-N", "-l")
	}
	fmtTrimPath := func(from, to string) string {
		if to == "" {
			// cannot be empty, dlv does not support relative path
			panic(fmt.Errorf("trimPath to must not be empty:%v", from))
		}
		if to == "/" {
			log.Printf("WARNING trim path found / replacement, should contains at least one child:from=%v, to=%v", from, to)
		}
		return fmt.Sprintf("%s=>%s", from, to)
	}
	newWorkRoot := path.Join(root, projectRoot)
	trimList := []string{fmtTrimPath(newWorkRoot, projectRoot)}
	for origAbsDir, cleanedAbsDir := range mappedMod {
		trimList = append(trimList, fmtTrimPath(path.Join(root, cleanedAbsDir), origAbsDir))
	}
	gcflagList = append(gcflagList, fmt.Sprintf("-trimpath=%s", strings.Join(trimList, ";")))
	outputFlags := ""
	if output != "" {
		outputFlags = fmt.Sprintf(`-o %s`, Quote(output))
	}
	gcflags := " "
	if len(gcflagList) > 0 {
		gcflags = "-gcflags=all=" + Quotes(gcflagList...)
	}

	// NOTE: can only specify -gcflags once, the last flag wins.
	// example:
	//     MOD=$(go list -m);W=${workspaceFolder};R=/var/folders/y8/kmfy7f8s5bb5qfsp0z8h7j5m0000gq/T/go-rewrite;D=$R$W;cd $D;DP=$(cd $D;cd ..;pwd); with-go1.14 go build -gcflags="all=-N -l -trimpath=/var/folders/y8/kmfy7f8s5bb5qfsp0z8h7j5m0000gq/T/go-rewrite/Users/xhd2015/Projects/gopath/src/github.com/xhd2015/go-mock=>/Users/xhd2015/Projects/gopath/src/github.com/xhd2015/go-mock" -o /tmp/xgo/${workspaceFolderBasename}/inspect_rewrite.with_go_mod.bin ./support/xgo/inspect/testdata/inspect_rewrite.go
	cmdList := []string{
		"set -e",
		// fmt.Sprintf("REWRITE_ROOT=%s", quote(root)),
		// fmt.Sprintf("PROJECT_ROOT=%s", quote(projectRoot)),
		fmt.Sprintf("cd %s", Quote(newWorkRoot)),
	}
	if newGoROOT != "" {
		cmdList = append(cmdList, fmt.Sprintf("export GOROOT=%s", Quote(path.Join(root, newGoROOT))))
	}
	buildCmd := "build"
	if forTest {
		buildCmd = "test -c"
	}
	goFlagsSpace := ""
	if goFlags != "" {
		goFlagsSpace = " " + goFlags
	}
	cmdList = append(cmdList, fmt.Sprintf(`go %s %s %s%s %s`, buildCmd, outputFlags, Quote(gcflags), goFlagsSpace, sh.JoinArgs(args)))

	_, _, err = sh.RunBashWithOpts(cmdList, sh.RunBashOptions{
		Verbose: verbose,
	})
	if err != nil {
		log.Printf("build %s failed", output)
		panic(err)
	}

	if verbose {
		log.Printf("build successful: %s", output)
	}

	return &BuildResult{
		Output: output,
	}
}

var Quotes = sh.Quotes
var Quote = sh.Quote

// if pathName is "", cwd is returned
func toAbsPath(pathName string) (string, error) {
	// if pathName == "" {
	// 	return "", fmt.Errorf("dir should not be empty")
	// }
	if path.IsAbs(pathName) {
		return pathName, nil
	}
	// _, err := os.Stat(pathName)
	// if err != nil {
	// 	return "", fmt.Errorf("%s not exists:%v", pathName, err)
	// }
	// if !f.IsDir() {
	// 	return "", fmt.Errorf("%s is not a dir", pathName)
	// }
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get cwd error:%v", err)
	}
	return path.Join(cwd, pathName), nil
}

// we are actually creating overlay, so CopyDirs can be ignored.

type CopyOpts struct {
	Verbose     bool
	IgnoreNames []string // dirs should be ignored from srcDirs. Still to be supported
	ProcessDest func(name string) string
}

// CopyDirs
// TODO: may use hard link or soft link instead of copy
func CopyDirs(srcDirs []string, destRoot string, opts CopyOpts) error {
	if len(srcDirs) == 0 {
		return fmt.Errorf("CopyDirs empty srcDirs")
	}
	for i, srcDir := range srcDirs {
		if srcDir == "" {
			return fmt.Errorf("srcDirs contains empty dir:%v at %d", srcDirs, i)
		}
	}
	if destRoot == "" {
		return fmt.Errorf("CopyDirs no destRoot")
	}
	if destRoot == "/" {
		return fmt.Errorf("destRoot cannot be /")
	}

	ignoreMap := make(map[string]bool, len(opts.IgnoreNames))
	for _, ignore := range opts.IgnoreNames {
		ignoreMap[ignore] = true
	}

	// try our best to ignore level-1 files
	files := make([][]string, 0, len(srcDirs))
	for _, srcDir := range srcDirs {
		dirFiles, err := ioutil.ReadDir(srcDir)
		if err != nil {
			return fmt.Errorf("list file of %s error:%v", srcDir, err)
		}
		dirFileNames := make([]string, 0, len(dirFiles))
		for _, f := range dirFiles {
			if ignoreMap[f.Name()] || (!f.IsDir() && f.Size() > 10*1024*1024 /* >10M */) {
				continue
			}
			dirFileNames = append(dirFileNames, f.Name())
		}
		files = append(files, dirFileNames)
	}

	cmdList := make([]string, 0, len(srcDirs))
	cmdList = append(cmdList,
		"set -e",
		fmt.Sprintf("rm -rf %s", Quote(destRoot)),
	)
	for i, srcDir := range srcDirs {
		srcFiles := files[i]
		if len(srcFiles) == 0 {
			continue
		}

		dstDir := path.Join(destRoot, srcDir)
		if opts.ProcessDest != nil {
			dstDir = opts.ProcessDest(dstDir)
			if dstDir == "" {
				continue
			}
		}
		qsrcDir := Quote(srcDir)
		qdstDir := Quote(dstDir)

		cmdList = append(cmdList, fmt.Sprintf("rm -rf %s && mkdir -p %s", qdstDir, qdstDir))
		for _, srcFile := range srcFiles {
			qsrcFile := Quote(srcFile)
			cmdList = append(cmdList, fmt.Sprintf("cp -R %s/%s %s/%s", qsrcDir, qsrcFile, qdstDir, qsrcFile))
		}
		cmdList = append(cmdList, fmt.Sprintf("chmod -R 0777 %s", qdstDir))
	}
	if opts.Verbose {
		log.Printf("copying dirs:%v", srcDirs)
	}
	return sh.RunBash(
		cmdList,
		opts.Verbose,
	)
}

// go's replace cannot have '@' character, so we replace it with ver_
// this is used for files to be copied into tmp dir, and will appear on replace verb.
func cleanGoFsPath(s string) string {
	// example:
	// /Users/xhd2015/Projects/gopath/pkg/mod/google.golang.org/grpc@v1.47.0/xds
	return strings.ReplaceAll(s, "@", "/")
}
