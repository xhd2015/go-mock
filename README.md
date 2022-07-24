# Introduction
A compile-time mock framework or library, it can be used in place where mock is needed, just like you would otherwise use other monkey-patch or interface-mock libraries.

# Quick start
```bash
# add dependency
go get github.com/xhd2015/go-mock

# generate a initial mock
go run -mod=readonly github.com/xhd2015/go-mock build -o ./exec.bin -v ./path/to/your_main_package

...add some mocks...

# rebuild the binary with mock enabled,then run
go run -mod=readonly github.com/xhd2015/go-mock build -o ./exec.bin -v ./path/to/your_main_package
./exec.bin

# or just run
go run -mod=readonly github.com/xhd2015/go-mock run -v ./path/to/your_main_package
```

For a working example, check [example/demo](./example/demo) for more details.

# Features
- Source code function extraction;
- Function level trace;
- Dynamic skipping mock(use `mock.CallOld()`);
- Nested mock;
- RPC mock;
- Nearly any function from any package mock;
- Type safe;
- build with go compiler, no dependency on any architecture nor os;
- Work on any platform: amd64, arm64...
- Work on any os: windows, linux, darwin...

# Integrate this library into tests
For business development usage, we recommend putting all test data and code into the top level directory `test`, to separate concern between product code and testing code.

You can make your own testing code layout by copying all contents under the [archtype](./archtype/) directory into your `test` directory. 

NOTE: it is important that [archtype/vendorhelp/vendorhelp.go](./archtype/vendorhelp/vendorhelp.go) should be included in your code base. Since `go-mock` is a main package, normally its code will not be included in the go's `vendor` directory, making it inconvienent to run in offline enviornment like CI. To include the `vendorhelp` but not really import it anywhere, the `go-mock` can be built and run from source directly, without having to pre-build one.

# Advaced usage
## `-v` verbose flag
Show verbose log.

## `-f` force flag
If encountered with building problems, try to add `-f` to refresh all cached files.

# Design internals
## Source code rewriting
The [https://go.dev/blog/cover](https://go.dev/blog/cover) provides a very good explanation on how coverage in go is implemented.

Take this simple `hello.go` fo example:
```go
package hello
 
import "fmt"
 
func Hello() {
        if true {
                fmt.Printf("hello world")
        }
}
```

Running `go tool cover -mode=count hello.go` would give us:
```go
package hello

import "fmt"

func Hello() {GoCover.Count[0]++;
        if true {GoCover.Count[1]++;
                fmt.Printf("hello world")
        }
}

var GoCover = struct {
        Count     [2]uint32
        Pos       [3 * 2]uint32
        NumStmt   [2]uint16
} {
        Pos: [3 * 2]uint32{
                5, 6, 0xa000e, // [0]
                6, 8, 0x3000a, // [1]
        },
        NumStmt: [2]uint16{
                1, // 0
                1, // 1
        },
}
```

You'll see that the code is rewritten by go in the way that:
- line layout does not change at all;
- extra statistics code is added to beginning of each block

NOTE: it is important that the line layout does not change at all, so that we can still debug the generated code as if it is written verbatim in the original file. So if panic,log and other line-based debugging info are reported, they will match the original file's lines.

With all these said, let's see how the go-mock library does all these:
```bash
go run github.com/xhd2015/go-mock print -print-rewrite=true -print-mock=false ./hello.go 
```

Output:
```go
package hello;import _mock "github.com/xhd2015/go-mock/mock"

import "fmt"

func Hello() {var _mockreq = struct{}{};var _mockresp struct{};_mock.TrapFunc(nil,&_mock.StubInfo{PkgName:"github.com/xhd2015/go-mock/tmp",Owner:"",OwnerPtr:false,Name:"Hello"}, nil, &_mockreq, &_mockresp,_mockHello,false,false,false);}; func _mockHello(){
        if true {
                fmt.Printf("hello world")
        }
}
```

You can relate each line of the generated file to the original file, with 2 changes:
- after `package hello`, we import this mock library
- in beginning of func `Hello`'s body, a `_mock.TrapFunc` is inserted so that when `Hello` is called, its control flow is transferred to `_mock.TrapFunc` inside which we add interceptor and other mock strategies.

## Build with `-trimpath`
After the original code rewritten, they are put into a temp directory, on Linux this is usually `/tmp/go-rewrite`.

To map the generated files to original files, we add `-trimpath=GEN_DIR=>ORIG_DIR` flag, as output by adding `-v` we can verify that:
```bash
cd /tmp/go-rewrite/Users/x/gopath/src/github.com/xhd2015/go-mock/example/demo
go build -o /Users/x/gopath/src/github.com/xhd2015/go-mock/example/demo/exec.bin '-gcflags=all=-trimpath=/tmp/go-rewrite/Users/x/gopath/src/github.com/xhd2015/go-mock/example/demo=>/Users/x/gopath/src/github.com/xhd2015/go-mock/example/demo' ./
```

NOTE the `-gcflags=all=-trimpath=/tmp/go-rewrite/Users/x/gopath/src/github.com/xhd2015/go-mock/example/demo=>/Users/x/gopath/src/github.com/xhd2015/go-mock/example/demo` will map files under `/tmp/go-rewrite/...` to their original directory.