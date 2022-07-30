package cmdsupport

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/tools/go/packages"

	"github.com/xhd2015/go-mock/code/gen"
	"github.com/xhd2015/go-mock/filecopy"
	"github.com/xhd2015/go-mock/inspect"
	"github.com/xhd2015/go-mock/sh"
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

	LoadArgs []string // passed to packages.Load

	ForTest bool
}

type GenRewriteResult struct {
	// original dir to new dir, original dir may contain @version, new dir replace @ with /
	// to be used as -trim when building
	MappedMod    map[string]string
	UseNewGOROOT string
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
	needGenMock := !skipGenMock  // gen mock inside test/mock_stub
	needMockRegistering := false // gen mock registering info,for debug info. TODO: may use another option
	needAnyMockStub := needGenMock || needMockRegistering

	stubGenDir := opts.StubGenDir
	projectDir := opts.ProjectDir
	projectDir, err = toAbsPath(projectDir)
	if err != nil {
		panic(fmt.Errorf("get abs dir err:%v", err))
	}
	stubInitEntryDir := "" // ${stubGenDir}/${xxxName}_init
	if needAnyMockStub && stubGenDir == "" {
		if needGenMock {
			stubGenDir = "test/mock_gen"
		} else {
			// try mock_gen, mock_gen1,...
			stubGenDir = inspect.NextFileNameUnderDir(projectDir, "mock_gen", "")
			stubInitEntryDir = stubGenDir + "/" + stubGenDir + "_init"
		}
	}
	if needAnyMockStub && stubInitEntryDir == "" {
		// try mock_gen_init, mock_gen_init1,...
		genName := inspect.NextFileNameUnderDir(projectDir, "mock_gen_init", "")
		stubInitEntryDir = stubGenDir + "/" + genName
	}
	stubRelGenDir := ""
	// make absolute
	if !path.IsAbs(stubGenDir) {
		stubRelGenDir = stubGenDir
		stubGenDir = path.Join(projectDir, stubGenDir)
	}

	loadPkgTime := time.Now()
	fset, starterPkgs, err := inspect.LoadPackages(args, &inspect.LoadOptions{
		ProjectDir: projectDir,
		ForTest:    opts.ForTest,
		BuildFlags: opts.LoadArgs,
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
	if len(starterPkgs) == 0 {
		panic(fmt.Errorf("no packages loaded."))
	}
	starterPkg0 := starterPkgs[0]
	starterPkg0Dir := inspect.GetFsPathOfPkg(starterPkg0.Module, starterPkg0.PkgPath)

	destFsPath := func(origFsPath string) string {
		return path.Join(rootDir, origFsPath)
	}

	// return relative directory
	stubFsRelDir := func(pkgModPath, pkgPath string) string {
		rel := ""
		if pkgModPath == modPath {
			rel = inspect.GetRelativePath(pkgModPath, pkgPath)
		} else {
			rel = path.Join("ext", pkgPath)
		}
		return rel
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

	extraPkgInInVendor := false
	hasStd := false
	for _, p := range extraPkgs {
		dir := inspect.GetModuleDir(p.Module)
		if dir == "" {
			// has module, but no dir
			// check if any file is inside vendor
			if inspect.IsVendor(modDir, p.GoFiles[0]) /*empty GoFiles are filtered*/ {
				extraPkgInInVendor = true
				break
			}
		}
		hasStd = hasStd || inspect.IsStdModule(p.Module)
	}

	if hasStd {
		res.UseNewGOROOT = inspect.GetGOROOT()
	}

	if verbose {
		if len(extraPkgs) > 0 {
			log.Printf("extra packages in vendor:%v", extraPkgInInVendor)
		}
	}

	// copy files
	var destUpdatedBySource map[string]bool
	doCopy := func() {
		if verbose {
			log.Printf("copying packages files into rewrite dir: total packages=%d", len(allPkgs))
		}
		copyTime := time.Now()
		destUpdatedBySource = copyPackageFiles(starterPkgs, extraPkgs, rootDir, extraPkgInInVendor, hasStd, force, verboseCopy, verbose)
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
	if !extraPkgInInVendor {
		doMod()
	}

	writeContentTime := time.Now()
	// overwrite new content
	nrewriteFile := 0
	nmock := 0
	type content struct {
		srcFile string
		bytes   []byte
	}

	var mockPkgList []string

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
			if pkgRes.MockContentError != nil {
				continue
			}
			nrewriteFile++
			backMap[cleanGoFsPath(destFsPath(fileRes.OrigFile))] = &content{
				srcFile: fileRes.OrigFile,
				bytes:   []byte(fileRes.Content),
			}
		}
		// generate mock stubs
		if needAnyMockStub && pkgRes.MockContentError == nil && pkgRes.MockContent != "" {
			// relative to current module
			rel := stubFsRelDir(pkg.Module.Path, pkgPath)
			genDir := path.Join(stubGenDir, rel)
			genFile := path.Join(genDir, "mock.go")
			if verboseRewrite || (verbose && len(allPkgs) < 10) {
				log.Printf("generate mock file %s", genFile)
			}

			pkgDir := inspect.GetFsPathOfPkg(pkg.Module, pkgPath)

			mockContent := []byte(pkgRes.MockContent)

			if needGenMock {
				backMap[genFile] = &content{
					srcFile: pkgDir,
					bytes:   mockContent,
				}
			}

			// TODO: may skip this for 'go test'
			if needMockRegistering {
				genRewriteFile := destFsPath(genFile)
				backMap[genRewriteFile] = &content{
					srcFile: pkgDir,
					bytes:   mockContent,
				}
				rdir := ""
				if stubGenDir != "" {
					rdir = "/" + strings.TrimPrefix(stubRelGenDir, "/")
				}
				mockPkgList = append(mockPkgList, pkg.Module.Path+rdir+"/"+rel)
				nmock++
			}
		}
	}

	if needMockRegistering {
		addMockRegisterContent := func(stubInitEntryDir string, mockPkgList []string) {
			// an entry init.go to import all registering types
			stubGenCode := genImportListContent(stubInitEntryDir, mockPkgList)
			backMap[destFsPath(path.Join(modDir, stubInitEntryDir, "init.go"))] = &content{
				bytes: []byte(stubGenCode),
			}

			// create a mock_init.go aside with original project files, to import the entry file above
			starterName := inspect.NextFileNameUnderDir(starterPkg0Dir, "mock_init", ".go")
			backMap[destFsPath(path.Join(starterPkg0Dir, starterName))] = &content{
				bytes: []byte(fmt.Sprintf("package %s\nimport _ %q", starterPkg0.Name, modPath+"/"+stubInitEntryDir)),
			}
		}
		addMockRegisterContent(stubInitEntryDir, mockPkgList)
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
			backFile := backMap[filePath].srcFile
			if backFile == "" {
				return true // should always copy if no back file
			}
			modTime, ferr := filecopy.GetNewestModTime(backFile)
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
	// debug
	// for _, p := range starterPkgs {
	// 	fmt.Printf("starter pkg:%v\n", p.PkgPath)
	// 	if p.Module != nil {
	// 		fmt.Printf("starter model:%v %v\n", p.PkgPath, p.Module.Path)
	// 	}
	// }
	for _, p := range starterPkgs {
		mod := p.Module
		if p.Module == nil {
			if inspect.IsGoTestPkg(p) {
				continue
			}
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
func copyPackageFiles(starterPkgs []*packages.Package, extraPkgs []*packages.Package, rootDir string, extraPkgInVendor bool, hasStd bool, force bool, verboseDetail bool, verboseOverall bool) (destUpdated map[string]bool) {
	var dirList []string
	fileIgnores := append([]string(nil), ignores...)

	// in test mode, go loads 3 types package under the same dir:
	// 1.normal package
	// 2.bridge package, which contains module
	// 3.test package, which does not contain module

	// copy go files
	moduleDirs := make(map[string]bool)
	addMod := func(pkgs []*packages.Package) {
		for _, p := range pkgs {
			// std packages are processed as a whole
			if inspect.IsGoTestPkg(p) || inspect.IsStdModule(p.Module) {
				continue
			}
			moduleDirs[inspect.GetModuleDir(p.Module)] = true
		}
	}

	addMod(starterPkgs)
	if !extraPkgInVendor {
		addMod(extraPkgs)
		// NOTE: not ignoring vendor
		// ignores = append(ignores, "vendor")
	}
	dirList = make([]string, 0, len(moduleDirs))
	for modDir := range moduleDirs {
		dirList = append(dirList, modDir)
	}
	if hasStd {
		// TODO: what if GOROOT is /usr/local/bin?
		dirList = append(dirList, inspect.GetGOROOT())
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
		if inspect.IsStdModule(p.Module) {
			// std modules are replaced via golabl env: GOROOT=xxx
			continue
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

	// get modules(for mods, actually only 1 module, i.e. the current module will be processed)
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
		gomod, err := inspect.GetGoMod(dir)
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
			err = sh.RunBash(cmds, verbose)
			if err != nil {
				panic(err)
			}
		}
	}
	return
}

// genImportListContent
// Deprecated: mock are registered in original package,not in a standalone import file
func genImportListContent(stubInitEntryDir string, mockPkgList []string) string {
	stubGen := gen.NewTemplateBuilder().Block(
		fmt.Sprintf("package %s", path.Base(stubInitEntryDir)),
		"",
		"import (",
	)
	for _, mokcPkg := range mockPkgList {
		stubGen.Block(fmt.Sprintf(`    _ %q`, mokcPkg))
	}
	stubGen.Block(
		")",
	)
	return stubGen.Format(nil)
}
