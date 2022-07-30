package inspect

import "testing"

// go test -run TestGetNextFileName -v ./support/xgo/inspect
func TestGetNextFileName(t *testing.T) {
	e0 := NextFileNameUnderDir("./testdata", "example", ".go")
	expectE0 := `example1.go`
	if e0 != expectE0 {
		t.Fatalf("expect %s = %+v, actual:%+v", `E0`, expectE0, e0)
	}

	e1 := NextFileNameUnderDir("./testdata", "example1", ".go")
	expectE1 := `example1.go`
	if e1 != expectE1 {
		t.Fatalf("expect %s = %+v, actual:%+v", `E1`, expectE1, e1)
	}

	// non-existent
	e2 := NextFileNameUnderDir("./testdata2", "example", ".go")
	expectE2 := `example.go`
	if e2 != expectE2 {
		t.Fatalf("expect %s = %+v, actual:%+v", `E1`, expectE2, e2)
	}
}
