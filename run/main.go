package run

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/xhd2015/go-mock/cmdsupport"
	"github.com/xhd2015/go-mock/inspect"
	_ "github.com/xhd2015/go-mock/inspect/mock" // for generated code to include mock correctly
)

// example:
//   $ go run github.com/xhd2015/go-mock help
//   $ go run github.com/xhd2015/go-mock print ./example/rewrite_example.go
//   $ go run github.com/xhd2015/go-mock rewrite -v ./inspect/testdata/demo/demo.go
//   $ go run github.com/xhd2015/go-mock build -v -debug ./inspect/testdata/demo/demo.go

var debug = flag.Bool("debug", false, "build debug(available for: build,run)")
var output = flag.String("o", "", "output executable(default: exec.bin or debug.bin,available for: build,run)")
var verbose = flag.Bool("v", false, "verbose")
var veryVerbose = flag.Bool("vv", false, "more verbose")
var filter = flag.String("filter", "", "specify functions should be mocked.\ntake a regex with matching against the form '<package>::<owner>::<type>'.\nexample: '.*::.*::Run', means matching any package name,any owner type and function name with 'Run'.\nthe special prefix 'not:' will invert the filter.\nexample: 'not:.*::.*::Run'")
var enableMockGen = flag.Bool("mock-gen", true, "generate mock stubs into test/mock_gen")
var mockConfig = flag.String("mock-config", "test/mock_gen.json", "path to read project's mock configs.\nthe special value 'none' skips reading config")
var mockPkgs = flag.String("mock-pkg", "", "a comma separated list:pkg1,pkg2...,denoting packages to be mocked")
var mockModules = flag.String("mock-module", "", "a comma separated list:module1,module2...,denoting modules to be mocked")
var allowMissing = flag.String("allow-missing", "", "missing packages: skip, warn,ignore")
var onlyPkg = flag.String("only-pkg", "", "only rewrite pkg specified, ignore any packages introduced by other modules or packages")
var force = flag.Bool("f", false, "force regenerate all files")
var printRewrite = flag.Bool("print-rewrite", true, "print rewrite content")
var printMock = flag.Bool("print-mock", true, "print mock content")
var buildFlags = flag.String("build-flags", "", "flags passed to underlying go command(go build,go run).\nNOTE: the flag is passed verbatim so you must quote it well to make is understood correctly by underlying shell.\nfor flags for go test can be passed after --, adding 'test.' prefix, for example: -- -test.v -args ...")
var testMode = flag.Bool("test", false, "cause build,run to deal with test packages instead of regular packages.if test command is ran, -test is implied.")
var mod = flag.String("mod", "", "load packages with -mod={given}")

var coverProfile = flag.String("coverprofile", "", "for test")
var coverPkg = flag.String("coverpkg", "", "for test")

var commands = map[string]func(comm string, args []string, extraArgs []string){
	"help":    help,
	"rewrite": rewrite,
	"print":   print,
	"build":   build,
	"run":     run,
	"test":    test,
}

func Main() {
	arg0 := os.Args[0]
	args := os.Args[1:]
	commd := ""
	if len(args) > 0 {
		commd = args[0]
		args = args[1:]
	}

	// other args
	var extraArgs []string
	n := len(args)
	for i := 0; i < n; i++ {
		if args[i] == "--" {
			// modify extraArgs first
			if i < n-1 {
				extraArgs = args[i+1:]
			}
			args = args[:i]
			break
		}
	}

	os.Args = append([]string{arg0}, args...)
	flag.Parse()
	args = flag.Args()

	// set usage
	flag.Usage = usage(flag.Usage)

	if *veryVerbose {
		*verbose = true
	}

	handler := commands[commd]
	if handler == nil {
		handler = defaultCommand
	}
	handler(commd, args, extraArgs)
}

type MockConfig struct {
	Packages     []string `json:"packages"` // including packages
	Modules      []string `json:"modules"`  // including modules
	AllowMissing string   `json:"allow_missing"`

	// map version of Packages
	pkgsMap map[string]bool
	modsMap map[string]bool
}

var cfg MockConfig

func initRewriteConfigs() {
	initMockConfig()
	initCmdLineConfig()

	if cfg.AllowMissing == "" {
		cfg.AllowMissing = "error"
	}
	cfg.pkgsMap = make(map[string]bool, len(cfg.Packages))
	for _, p := range cfg.Packages {
		cfg.pkgsMap[p] = true
	}
	cfg.modsMap = make(map[string]bool, len(cfg.Modules))
	for _, m := range cfg.Modules {
		cfg.modsMap[m] = true
	}
}

// merge config
func initCmdLineConfig() {
	pkgs := strings.TrimSpace(*mockPkgs)
	if pkgs != "" {
		cmdLinePkgList := strings.Split(pkgs, ",")
		cfg.Packages = append(cfg.Packages, cmdLinePkgList...)
	}
	mods := strings.TrimSpace(*mockModules)
	if mods != "" {
		cmdMods := strings.Split(mods, ",")
		cfg.Modules = append(cfg.Modules, cmdMods...)
	}

	if *allowMissing != "" {
		// override cfg's allow missing
		cfg.AllowMissing = *allowMissing
	}
}
func getOnlyPkgs() map[string]bool {
	if *onlyPkg == "" {
		return nil
	}
	return map[string]bool{
		*onlyPkg: true,
	}
}

func initMockConfig() {
	f := *mockConfig
	if f == "none" {
		return
	}
	allowNonExists := false
	if f == "" || f == "test/mock_gen.json" {
		allowNonExists = true
		f = "test/mock_gen.json"
	}
	if allowNonExists {
		if stat, err := os.Stat(f); errors.Is(err, os.ErrNotExist) || (err == nil && stat.IsDir()) {
			return
		}
	}

	content, err := ioutil.ReadFile(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading config from %s error:%v", f, err)
		os.Exit(1)
	}
	err = json.Unmarshal(content, &cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parsing config from %s error:%v", f, err)
		os.Exit(1)
	}
}

func help(commd string, args []string, extraArgs []string) {
	flag.Usage()
	os.Exit(0)
}

func rewrite(commd string, args []string, extraArgs []string) {
	cmdsupport.GenRewrite(args, cmdsupport.GetRewriteRoot(), getRewriteOptions())
}
func getRewriteOptions() *cmdsupport.GenRewriteOptions {
	initRewriteConfigs()
	filterFn := createFilter(*filter)

	var loadArgs []string
	if *mod != "" {
		loadArgs = append(loadArgs, "-mod="+*mod)
	}
	return &cmdsupport.GenRewriteOptions{
		Verbose:        *verbose,
		VerboseCopy:    *veryVerbose,
		VerboseRewrite: *veryVerbose,
		SkipGenMock:    !*enableMockGen,
		OnlyPackages:   getOnlyPkgs(),
		Packages:       cfg.pkgsMap,
		Modules:        cfg.modsMap,
		AllowMissing:   getAllowMissing(),
		Force:          *force,
		ForTest:        *testMode,
		LoadArgs:       loadArgs,
		RewriteOptions: &inspect.RewriteOptions{
			Filter: filterFn,
		},
	}
}

func print(commd string, args []string, extraArgs []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "requires 1 file")
		os.Exit(1)
	}
	if len(args) > 1 {
		fmt.Fprintf(os.Stderr, "requires 1 file, given %d", len(args))
		os.Exit(1)
	}

	filterFn := createFilter(*filter)
	cmdsupport.PrintRewrite(args[0], *printRewrite, *printMock, &inspect.RewriteOptions{
		Filter: filterFn,
	})
}

func getBuildOptions() *cmdsupport.BuildOptions {
	goFlags := *buildFlags
	if *coverProfile != "" {
		goFlags = "-coverprofile=" + *coverProfile + " " + goFlags
	}
	if *coverPkg != "" {
		goFlags = "-coverpkg=" + *coverPkg + " " + goFlags
	}
	if *mod != "" {
		goFlags = "-mod=" + *mod + " " + goFlags
	}
	return &cmdsupport.BuildOptions{
		Verbose: *verbose,
		Debug:   *debug,
		Output:  *output,
		ForTest: *testMode,
		GoFlags: goFlags,
	}
}

func build(commd string, args []string, extraArgs []string) {
	cmdsupport.BuildRewrite(args, getRewriteOptions(), getBuildOptions())
}
func getAllowMissing() bool {
	return cfg.AllowMissing != "error"
}

func run(commd string, args []string, extraArgs []string) {
	if *testMode {
		test(commd, args, extraArgs)
		return
	}
	buildResult := cmdsupport.BuildRewrite(args, getRewriteOptions(), getBuildOptions())

	bashCmd := cmdsupport.Quotes(append([]string{buildResult.Output}, extraArgs...)...)
	if *verbose {
		log.Printf("%s", bashCmd)
	}
	execCmd := exec.Command("bash", "-c", bashCmd)

	execCmd.Stderr = os.Stderr
	execCmd.Stdout = os.Stdout
	err := execCmd.Run()
	if err != nil {
		log.Fatalf("failed to run %s", buildResult.Output)
	}
}

func test(commd string, args []string, extraArgs []string) {
	rwOpts := getRewriteOptions()
	buildOpts := getBuildOptions()
	buildOpts.ForTest = true
	rwOpts.ForTest = true
	buildResult := cmdsupport.BuildRewrite(args, rwOpts, buildOpts)

	if *coverProfile != "" {
		oldArgs := extraArgs
		extraArgs = []string{"-test.coverprofile=" + *coverProfile}
		extraArgs = append(extraArgs, oldArgs...)
	}

	bashCmd := cmdsupport.Quotes(append([]string{buildResult.Output}, extraArgs...)...)
	if *verbose {
		log.Printf("%s", bashCmd)
	}
	execCmd := exec.Command("bash", "-c", bashCmd)

	execCmd.Stderr = os.Stderr
	execCmd.Stdout = os.Stdout
	err := execCmd.Run()
	if err != nil {
		log.Fatalf("failed to run %s", buildResult.Output)
	}
}

func defaultCommand(commd string, args []string, extraArgs []string) {
	if commd == "" {
		fmt.Printf("requries cmd: build,run,test,rewrite,show,help\n")
	} else {
		fmt.Printf("unknown cmd:%s\n", commd)
	}
	flag.Usage()
	os.Exit(1)
}

func usage(defaultUsage func()) func() {
	return func() {
		fmt.Printf("supported commands: build,run,test,rewrite,help\n")
		fmt.Printf("    build ARGS\n")
		fmt.Printf("        build the package with generated mock stubs,default output is exec.bin or debug.bin if -debug\n")
		fmt.Printf("    run ARGS [--] [EXEC_ARGS]\n")
		fmt.Printf("        run the package with generated mock stubs\n")
		fmt.Printf("    test ARGS [--] [EXEC_ARGS]\n")
		fmt.Printf("        test the package with generated mock stubs, this implies -test\n")
		fmt.Printf("    rewrite ARGS\n")
		fmt.Printf("        rewrite the package with generated mock stubs into a temp directory,show the directory if -v\n")
		fmt.Printf("    print FILE\n")
		fmt.Printf("        print rewritten content of a file, can use -print-rewrite=true(default)|false,-print-mock=true(default)|false to toggle display\n")
		fmt.Printf("    help\n")
		fmt.Printf("        show help message\n")
		defaultUsage()
		fmt.Printf("examples:\n")
		fmt.Printf("    # build:\n")
		fmt.Printf("    $  go run -mod=readonly github.com/xhd2015/go-mock build -debug ./src/main.go\n")
		fmt.Printf("\n")
		fmt.Printf("    # run:\n")
		fmt.Printf("    $  go run -mod=readolny github.com/xhd2015/go-mock run ./src/main.go\n")
		fmt.Printf("\n")
		fmt.Printf("    # run, args passed to the executable after --:\n")
		fmt.Printf("    $  go run -mod=readonly github.com/xhd2015/go-mock run ./src/main.go -- -config config_dir\n")
		fmt.Printf("\n")
		fmt.Printf("    # test:\n")
		fmt.Printf("    $  go test -mod=readonly github.com/xhd2015/go-mock test ./test/main_test.go -- -config config_dir\n")
		fmt.Printf("\n")
		fmt.Printf("    # test with coverage:\n")
		fmt.Printf("    $  go run -mod=readonly github.com/xhd2015/go-mock test -build-flags='-coverprofile=cover.out -coverpkg ./...' -v ./verify_test_cmd/\n")
	}
}

func createFilter(filter string) func(pkgPath string, fileName string, ownerName string, ownerIsPtr bool, funcName string) bool {
	expectMatch := true
	if strings.HasPrefix(filter, "not:") {
		expectMatch = false
		filter = filter[len("not:"):]
	}
	if filter == "" {
		return nil
	}

	exactFilter := strings.TrimPrefix(filter, "^")
	exactFilter = strings.TrimSuffix(exactFilter, "$")
	re := regexp.MustCompile("^" + exactFilter + "$")
	return func(pkgPath, fileName, ownerName string, ownerIsPtr bool, funcName string) bool {
		s := fmt.Sprintf("%s::%s::%s", pkgPath, ownerName, funcName)
		match := re.MatchString(s)
		// log.Printf("filter match:%s, filter=%s, match=%v,expectMatch=%v",s, exactFilter,match,expectMatch)
		return match == expectMatch
	}
}
