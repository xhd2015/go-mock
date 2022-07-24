package do_not_import_me

// github.com/xhd2015/go-mock is a main package, normally it will not be included in vendor.
// this package makes vendor include go-mock correctly.
// please place this file inside your project's test directory, and do not import it.
// when you run 'go mod vendor', the main package go-mock will be included.
// This is useful in CI environment where module cannot be downloaded but instead read from vendor mode.
import (
	_ "github.com/xhd2015/go-mock"
)
