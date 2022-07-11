package gen

import "testing"

// go test -run TestBuilderSimple -v ./support/code/gen
func TestBuilderSimple(t *testing.T) {
	b := NewTemplateBuilder()
	b.Pretty(false)

	b.Block(
		"if __VAR__a > 0 && __VAR__b < 0 && __VAR_C__ {",
		"    echo yes",
		"}",
	)

	s := b.Format(VarMap{
		"__VAR__": "haha",
	})
	expectS := `if hahaa > 0 && hahab < 0 && __VAR_C__ {echo yes;};`
	if s != expectS {
		t.Fatalf("expect %s = %+v, actual:%+v", `s`, expectS, s)
	}
}

// go test -run TestBuilderIf -v ./support/code/gen
func TestBuilderIf(t *testing.T) {
	b := NewTemplateBuilder()
	b.Pretty(false)

	b.Block(
		"if __VAR__a > 0 && __VAR__b < 0 && __VAR_C__ {",
		"    echo yes",
		"}",
		b.If(true).Then(
			"A",
		).Else(
			"B",
		),
		b.If(false).Then(
			"Make It",
		).Else(
			"Make me",
		),
	)

	s := b.Format(VarMap{
		"__VAR__": "haha",
	})
	expectS := `if hahaa > 0 && hahab < 0 && __VAR_C__ {echo yes;};A;Make me;`
	if s != expectS {
		t.Fatalf("expect %s = %+v, actual:%+v", `s`, expectS, s)
	}
}
