package cmdsupport

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/tools/go/packages"

	"github.com/xhd2015/go-mock/filecopy"
	"github.com/xhd2015/go-mock/inspect"
)

type GenRewriteOptions struct {
	Verbose        bool
	VerboseCopy    bool
	VerboseRewrite bool
	// VerboseGomod   bool
	ProjectDir     string // the project dir
	RewriteOptions *inspect.RewriteOptions
	StubGenDir     string // relative path, default test/mock_gen
	SkipGenMock    bool

	OnlyPackages map[string]bool
	Packages     map[string]bool
	Modules      map[string]bool
	AllowMissing bool

	Force bool // force indicates no cache
}

type GenRewriteResult struct {
	// original dir to new dir, original dir may contain @version, new dir replace @ with /
	MappedMod map[string]string
}

var ignores = []string{"(.*/)?\\.git\\b", "(.*/)?node_modules\\b"}

/*
COST stats of serial process:
2022/07/02 21:56:38 COST load packages:4.522980981s
2022/07/02 21:56:38 COST load package -> filter package:131.63µs
2022/07/02 21:56:38 COST filter package:1.875944ms
2022/07/02 21:56:38 COST rewrite:229.670816ms
2022/07/02 21:56:48 COST copy:9.850795028s
2022/07/02 21:56:48 COST go mod:152.905388ms
2022/07/02 21:56:50 COST write content:2.157077768s
2022/07/02 21:56:50 COST GenRewrite:16.937557165s

shows that copy is the most time consuming point.
*/

func GenRewrite(args []string, rootDir string, opts *GenRewriteOptions) (res *GenRewriteResult) {
	res = &GenRewriteResult{}
	if opts == nil {
		opts = &GenRewriteOptions{}
	}
	verbose := opts.Verbose
	verboseCopy := opts.VerboseCopy
	verboseRewrite := opts.VerboseRewrite
	skipGenMock := opts.SkipGenMock
	verboseCost := false
	force := opts.Force

	if rootDir == "" {
		panic(fmt.Errorf("rootDir is empty"))
	}
	if verbose {
		log.Printf("rewrite root: %s", rootDir)
	}
	err := os.MkdirAll(rootDir, 0777)
	if err != nil {
		panic(fmt.Errorf("error mkdir %s %v", rootDir, err))
	}

	stubGenDir := ""
	projectDir := ""
	if opts != nil {
		projectDir = opts.ProjectDir
		stubGenDir = opts.StubGenDir
	}
	if stubGenDir == "" {
		stubGenDir = "test/mock_gen"
	}
	projectDir, err = toAbsPath(projectDir)
	if err != nil {
		panic(fmt.Errorf("get abs dir err:%v", err))
	}
	// make absolute
	if !path.IsAbs(stubGenDir) {
		stubGenDir = path.Join(projectDir, stubGenDir)
	}

	loadPkgTime := time.Now()
	fset, starterPkgs, err := inspect.LoadPackages(args, &inspect.LoadOptions{
		ProjectDir: projectDir,
	})
	loadPkgEnd := time.Now()
	if verboseCost {
		log.Printf("COST load packages:%v", loadPkgEnd.Sub(loadPkgTime))
	}
	if err != nil {
		panic(err)
	}

	// ensure that starterPkgs have exactly one module
	modPath, modDir := extractSingleMod(starterPkgs)
	_ = modDir // maybe used later
	if verbose {
		log.Printf("current module: %s , dir %s", modPath, modDir)
	}

	destFsPath := func(origFsPath string) string {
		return path.Join(rootDir, origFsPath)
	}
	// return absolute directory
	stubFsDir := func(pkgModPath, pkgPath string) string {
		rel := ""
		if pkgModPath == modPath {
			rel = inspect.GetRelativePath(pkgModPath, pkgPath)
		} else {
			rel = path.Join("ext", pkgPath)
		}
		return path.Join(stubGenDir, rel)
	}

	// init rewrite opts
	onlyPkgs := opts.OnlyPackages
	wantsExtraPkgs := opts.Packages
	wantsExtrPkgsByMod := opts.Modules
	allowMissing := opts.AllowMissing

	rewriteOpts := opts.RewriteOptions
	if rewriteOpts == nil {
		rewriteOpts = &inspect.RewriteOptions{}
	}

	// expand to all packages under the same module that depended by starter packages
	filterPkgTime := time.Now()
	if verboseCost {
		log.Printf("COST load package -> filter package:%v", filterPkgTime.Sub(loadPkgEnd))
	}
	var modPkgs []*packages.Package
	var extraPkgs []*packages.Package
	if len(onlyPkgs) == 0 {
		modPkgs, extraPkgs = inspect.GetSameModulePackagesAndPkgsGiven(starterPkgs, wantsExtraPkgs, wantsExtrPkgsByMod)
	} else {
		var oldModPkgs []*packages.Package
		oldModPkgs, extraPkgs = inspect.GetSameModulePackagesAndPkgsGiven(starterPkgs, onlyPkgs, nil)
		for _, p := range oldModPkgs {
			if onlyPkgs[p.PkgPath] {
				modPkgs = append(modPkgs, p)
			}
		}
	}
	filterPkgEnd := time.Now()
	if verboseCost {
		log.Printf("COST filter package:%v", filterPkgEnd.Sub(filterPkgTime))
	}

	allPkgs := make([]*packages.Package, 0, len(modPkgs)+len(extraPkgs))
	allPkgs = append(allPkgs, modPkgs...)
	for _, p := range extraPkgs {
		if len(p.GoFiles) == 0 {
			continue
		}
		allPkgs = append(allPkgs, p)
	}
	pkgMap := inspect.MakePackageMap(allPkgs)

	if verbose {
		log.Printf("found %d packages", len(allPkgs))
	}

	// check if wanted pkgs are all found
	var missingExtra []string
	for extraPkg := range wantsExtraPkgs {
		if pkgMap[extraPkg] == nil {
			missingExtra = append(missingExtra, extraPkg)
		}
	}
	if len(missingExtra) > 0 {
		if !allowMissing {
			panic(fmt.Errorf("packages not found:%v", missingExtra))
		}
		log.Printf("WARNING: not found packages will be skipped:%v", missingExtra)
	}

	// rewrite
	rewriteTime := time.Now()
	contents := inspect.RewritePackages(fset, allPkgs, rewriteOpts)
	rewriteEnd := time.Now()
	if verboseCost {
		log.Printf("COST rewrite:%v", rewriteEnd.Sub(rewriteTime))
	}

	vendorMod := false
	for _, p := range extraPkgs {
		dir := inspect.GetModuleDir(p.Module)
		if dir == "" {
			// has module, but no dir
			// check if any file is inside vendor
			if inspect.IsVendor(modDir, p.GoFiles[0]) /*empty GoFiles are filtered*/ {
				vendorMod = true
				break
			}
		}
	}

	if verbose {
		log.Printf("vendor mode:%v", vendorMod)
	}

	// copy files
	var destUpdatedBySource map[string]bool
	doCopy := func() {
		if verbose {
			log.Printf("copying packages files into rewrite dir: total packages=%d", len(allPkgs))
		}
		copyTime := time.Now()
		destUpdatedBySource = copyPackageFiles(starterPkgs, extraPkgs, rootDir, vendorMod, force, verboseCopy, verbose)
		copyEnd := time.Now()
		if verboseCost {
			log.Printf("COST copy:%v", copyEnd.Sub(copyTime))
		}
	}
	doCopy()

	// mod replace only work at module-level, so if at least
	// one package inside a module is modified, we need to
	// copy its module out.
	doMod := func() {
		// after copied, modify go.mod with replace absoluted
		if verbose {
			log.Printf("replacing go.mod with rewritten paths")
		}
		goModTime := time.Now()
		res.MappedMod = makeGomodReplaceAboslute(modPkgs, extraPkgs, rootDir, verbose)
		goModEnd := time.Now()
		if verboseCost {
			log.Printf("COST go mod:%v", goModEnd.Sub(goModTime))
		}
	}
	if !vendorMod {
		doMod()
	}

	useOldCopy := false

	writeContentTime := time.Now()
	// overwrite new content
	nrewriteFile := 0
	nmock := 0
	if !useOldCopy {
		type content struct {
			srcFile string
			bytes   []byte
		}

		backMap := make(map[string]*content)
		for _, pkgRes := range contents {
			pkgPath := pkgRes.PkgPath
			pkg := pkgMap[pkgPath]
			if pkg == nil {
				panic(fmt.Errorf("pkg not found:%v", pkgPath))
			}

			// generate rewritting files
			for _, fileRes := range pkgRes.Files {
				if fileRes.OrigFile == "" {
					panic(fmt.Errorf("orig file not found:%v", pkgPath))
				}
				if pkgRes.Error != nil {
					continue
				}
				nrewriteFile++
				backMap[cleanGoFsPath(destFsPath(fileRes.OrigFile))] = &content{
					srcFile: fileRes.OrigFile,
					bytes:   []byte(fileRes.Content),
				}
			}
			if !skipGenMock {
				// generate mock stubs
				if pkgRes.Error == nil && pkgRes.MockContent != "" {
					// relative to current module
					genDir := stubFsDir(pkg.Module.Path, pkgPath)
					genFile := path.Join(genDir, "mock.go")
					if verboseRewrite || (verbose && len(allPkgs) < 10) {
						log.Printf("generate mock file %s", genFile)
					}

					pkgDir := inspect.GetFsPathOfPkg(pkg.Module, pkgPath)

					mockContent := []byte(pkgRes.MockContent)
					backMap[genFile] = &content{
						srcFile: pkgDir,
						bytes:   mockContent,
					}
					nmock++
					genRewriteFile := destFsPath(genFile)
					backMap[genRewriteFile] = &content{
						srcFile: pkgDir,
						bytes:   mockContent,
					}
				}
			}
		}

		// in this copy config, srcPath is the same with destPath
		// the extra info is looked up in a back map
		filecopy.SyncGenerated(
			func(fn func(path string)) {
				for path := range backMap {
					fn(path)
				}
			},
			func(name string) []byte {
				c, ok := backMap[name]
				if !ok {
					panic(fmt.Errorf("no such file:%v", name))
				}
				return c.bytes
			},
			"", // already rooted
			func(filePath, destPath string, destFileInfo os.FileInfo) bool {
				// if ever updated by source, then we always need to update again.
				// NOTE: this only applies to rewritten file,mock file not influenced.
				if destUpdatedBySource[filePath] {
					// log.Printf("DEBUG update by source:%v", filePath)
					return true
				}
				modTime, ferr := filecopy.GetNewestModTime(backMap[filePath].srcFile)
				if ferr != nil {
					panic(ferr)
				}
				return !modTime.IsZero() && modTime.After(destFileInfo.ModTime())
			},
			filecopy.SyncRebaseOptions{
				Force:   force,
				Ignores: ignores,
				// ProcessDestPath: cleanFsGoPath, // not needed as we already did that
				OnUpdateStats: filecopy.NewLogger(func(format string, args ...interface{}) {
					log.Printf(format, args...)
				}, verboseRewrite, verbose, 200*time.Millisecond),
			},
		)
	} else {
		// collect directories to be created,and create them first
		if !skipGenMock {
			var dirCmds []string
			for _, pkgRes := range contents {
				pkgPath := pkgRes.PkgPath
				pkg := pkgMap[pkgPath]
				if pkg == nil {
					panic(fmt.Errorf("pkg not found:%v", pkgPath))
				}
				if pkgRes.Error == nil && pkgRes.MockContent != "" {
					genDir := stubFsDir(pkg.Module.Path, pkgPath)
					genRewriteDir := destFsPath(genDir)
					dirCmds = append(dirCmds,
						fmt.Sprintf("mkdir -p %s", Quote(genDir)),
						fmt.Sprintf("mkdir -p %s", Quote(genRewriteDir)),
					)
				}
			}
			if len(dirCmds) > 0 {
				err := RunBash(dirCmds, verboseRewrite)
				if err != nil {
					panic(fmt.Errorf("create dir error:%v", err))
				}
			}
		}
		for _, pkgRes := range contents {
			pkgPath := pkgRes.PkgPath
			pkg := pkgMap[pkgPath]
			if pkg == nil {
				panic(fmt.Errorf("pkg not found:%v", pkgPath))
			}

			// generate rewritting files
			for _, fileRes := range pkgRes.Files {
				if fileRes.OrigFile == "" {
					panic(fmt.Errorf("orig file not found:%v", pkgPath))
				}
				if pkgRes.Error != nil {
					continue
				}
				nrewriteFile++
				destFile := cleanGoFsPath(destFsPath(fileRes.OrigFile))
				if verboseRewrite || len(allPkgs) < 10 {
					log.Printf("rewrite file %s , original file %s", destFile, fileRes.OrigFile)
				}
				err = ioutil.WriteFile(destFile, []byte(fileRes.Content), 0666)
				if err != nil {
					panic(fmt.Errorf("write file error:%v %v", destFile, err))
				}
			}

			if !skipGenMock {
				// generate mock stubs
				if pkgRes.Error == nil && pkgRes.MockContent != "" {
					// relative to current module
					genDir := stubFsDir(pkg.Module.Path, pkgPath)
					genFile := path.Join(genDir, "mock.go")
					if verboseRewrite || len(allPkgs) < 10 {
						log.Printf("generate mock file %s", genFile)
					}
					nmock++
					mockContent := []byte(pkgRes.MockContent)
					err = ioutil.WriteFile(genFile, mockContent, 0666)
					if err != nil {
						panic(fmt.Errorf("write file error:%v %v", genFile, err))
					}
					genRewriteFile := destFsPath(genFile)
					err = ioutil.WriteFile(genRewriteFile, mockContent, 0666)
					if err != nil {
						panic(fmt.Errorf("write file error:%v %v", genRewriteFile, err))
					}
				}
			}
		}
	}
	writeContentEnd := time.Now()
	if verboseCost {
		log.Printf("COST write content:%v", writeContentEnd.Sub(writeContentTime))
	}

	if verbose {
		log.Printf("rewritten files:%d, generate mock files:%d", nrewriteFile, nmock)
	}
	if verboseCost {
		log.Printf("COST GenRewrite:%v", time.Since(loadPkgTime))
	}
	return
}

func extractSingleMod(starterPkgs []*packages.Package) (modPath string, modDir string) {
	for _, p := range starterPkgs {
		mod := p.Module
		if p.Module == nil {
			panic(fmt.Errorf("package %s has no module", p.PkgPath))
		}
		if mod.Replace != nil {
			panic(fmt.Errorf("package %s has a replacement module %s, but want a self-rooted module: %s", p.PkgPath, mod.Replace.Dir, mod.Path))
		}
		if modPath == "" {
			modPath = mod.Path
			modDir = mod.Dir
			continue
		}
		if modPath != mod.Path || modDir != mod.Dir {
			panic(fmt.Errorf("package %s has different module %s, want a single module:%s", p.PkgPath, mod.Path, modPath))
		}
	}
	if modPath == "" || modDir == "" {
		panic("no modules loaded")
	}
	return
}

// copyPackageFiles copy starter packages(with all packages under the same module) and extra packages into rootDir, to bundle them together.
func copyPackageFiles(starterPkgs []*packages.Package, extraPkgs []*packages.Package, rootDir string, vendorMod bool, force bool, verboseDetail bool, verboseOverall bool) (destUpdated map[string]bool) {
	var dirList []string
	fileIgnores := append([]string(nil), ignores...)

	// copy go files
	moduleDirs := make(map[string]bool)
	addMod := func(pkgs []*packages.Package) {
		for _, p := range pkgs {
			moduleDirs[inspect.GetModuleDir(p.Module)] = true
		}
	}

	addMod(starterPkgs)
	if !vendorMod {
		addMod(extraPkgs)
		// NOTE: not ignoring vendor
		// ignores = append(ignores, "vendor")
	}
	dirList = make([]string, 0, len(moduleDirs))
	for modDir := range moduleDirs {
		dirList = append(dirList, modDir)
	}
	// copy other pkgs (deprecated, this only copies package files, but we need to module if any package is modfied.may be used in the future when overlay is supported)
	// for _, p := range extraPkgs {
	// 	if p.Module == nil {
	// 		panic(fmt.Errorf("package has no module:%v", p.PkgPath))
	// 	}
	// 	dirList = append(dirList, inspect.GetFsPathOfPkg(p.Module, p.PkgPath))
	// }

	var destUpdatedM sync.Map

	size := int64(0)
	err := filecopy.SyncRebase(dirList, rootDir, filecopy.SyncRebaseOptions{
		Ignores:         fileIgnores,
		Force:           force,
		DeleteNotFound:  true, // uncovered files are deleted
		ProcessDestPath: cleanGoFsPath,
		OnUpdateStats: filecopy.NewLogger(func(format string, args ...interface{}) {
			log.Printf(format, args...)
		}, verboseDetail, verboseOverall, 200*time.Millisecond),
		DidCopy: func(srcPath, destPath string) {
			destUpdatedM.Store(destPath, true)
			atomic.AddInt64(&size, 1)
		},
	})

	destUpdated = make(map[string]bool, atomic.LoadInt64(&size))
	destUpdatedM.Range(func(destPath, value interface{}) bool {
		destUpdated[destPath.(string)] = true
		return true
	})

	// err := CopyDirs(dirList, rootDir, CopyOpts{
	// 	Verbose:     verbose,
	// 	IgnoreNames: ignores,
	// 	ProcessDest: cleanGoFsPath,
	// })
	if err != nil {
		panic(err)
	}
	return
}

// go mod's replace, find relative paths and replace them with absolute path
func makeGomodReplaceAboslute(modPkgs []*packages.Package, extraPkgs []*packages.Package, rebaseDir string, verbose bool) (mappedMod map[string]string) {
	goModEditReplace := func(oldpath string, newPath string) string {
		return fmt.Sprintf("go mod edit -replace=%s=%s", Quote(oldpath), Quote(newPath))
	}
	// premap: modPath -> ${rebaseDir}/${modDir}
	preMap := make(map[string]string, len(extraPkgs))
	preCmdList := make([]string, 0, len(extraPkgs))
	mappedMod = make(map[string]string)
	for _, p := range extraPkgs {
		if p.Module == nil {
			panic(fmt.Errorf("cannot replace non-module package:%v", p.PkgPath))
		}
		mod := p.Module
		if mod.Replace != nil {
			mod = mod.Replace
		}
		if preMap[mod.Path] != "" {
			continue
		}
		cleanDir := cleanGoFsPath(mod.Dir)
		newPath := path.Join(rebaseDir, cleanDir)
		preMap[mod.Path] = newPath
		preCmdList = append(preCmdList, goModEditReplace(mod.Path, newPath))

		mappedMod[mod.Dir] = cleanDir
	}

	// get modules
	mods := make([]*packages.Module, 0, 1)
	modMap := make(map[string]bool, 1)
	for _, p := range modPkgs {
		if p.Module == nil {
			continue
		}
		mod := p.Module
		if mod.Replace != nil {
			mod = mod.Replace
		}
		if modMap[mod.Path] {
			continue
		}
		modMap[mod.Path] = true
		mods = append(mods, mod)
	}
	for _, mod := range mods {
		dir := mod.Dir
		origDir := dir
		// rebase to rootDir
		if rebaseDir != "" {
			dir = path.Join(rebaseDir, dir)
		}

		type Module struct {
			Path    string
			Version string
		}
		type Replace struct {
			Old Module
			New Module
		}
		type GoMod struct {
			Replace []Replace
		}

		var gomod GoMod
		_, _, err := RunBashWithOpts([]string{
			fmt.Sprintf("cd %s", Quote(dir)),
			"go mod edit -json", // get json
		}, RunBashOptions{
			Verbose:      false, // don't make read verbose
			StdoutToJSON: &gomod,
		})
		if err != nil {
			panic(err)
		}

		// replace with absolute paths
		var replaceList []string
		if len(gomod.Replace) > 0 {
			replaceList = make([]string, 0, len(gomod.Replace))
		}

		for _, rp := range gomod.Replace {
			newPath := preMap[rp.Old.Path]
			// skip replace made by us
			if newPath != "" {
				continue
			}
			if strings.HasPrefix(rp.New.Path, "./") || strings.HasPrefix(rp.New.Path, "../") {
				oldv := rp.Old.Path
				if rp.Old.Version != "" {
					oldv += "@" + rp.Old.Version
				}
				replaceList = append(replaceList, goModEditReplace(oldv, path.Join(origDir, rp.New.Path)))
			}
		}

		if len(replaceList) > 0 || len(preCmdList) > 0 {
			if verbose {
				log.Printf("make absolute replace in go.mod for %v", mod.Path)
			}
			cmds := append([]string{
				fmt.Sprintf("cd %s", Quote(dir)),
			}, replaceList...)
			cmds = append(cmds, preCmdList...)
			err = RunBash(cmds, verbose)
			if err != nil {
				panic(err)
			}
		}
	}
	return
}
