package inspect

import "testing"

// go test -run TestHasPrefixSplit -v ./support/xgo/inspect
func TestHasPrefixSplit(t *testing.T){
	v := HasPrefixSplit("test/aka", "test",'/')

	t.Logf("%v",v)
}