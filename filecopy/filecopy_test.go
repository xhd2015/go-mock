package filecopy

import (
	"errors"
	"io/ioutil"
	"os"
	"path"
	"testing"
	"time"
)

func TestListDir(t *testing.T) {
	tmp := os.TempDir()
	x := path.Join(tmp, "filecopy", "test", "list.txt")

	err := os.MkdirAll(path.Dir(x), 0777)
	if err != nil {
		t.Logf("mkdir all err:%v", err)
	}
	err = ioutil.WriteFile(x, []byte("hello"), 0777)
	if err != nil {
		t.Logf("create file err:%v", err)
	}

	_, err = ioutil.ReadDir(x)
	if err == nil {
		t.Fatalf("want error")
	}
	t.Logf("ReadDir err:%v", err)
}

func TestMkdirAll(t *testing.T) {
	tmp := os.TempDir()

	x := path.Join(tmp, "filecopy", "test", "aa")

	// RemoveAll does not report error if the dir does not exist
	err := os.RemoveAll(x)
	if err != nil {
		t.Logf("remove all err:%v", err)
	}

	err = os.MkdirAll(x, 0777)
	if err != nil {
		t.Logf("mkdir all err:%v", err)
	}
}

func TestSyncRebasedSimple(t *testing.T) {
	src, dest, err := prepareDir("simple")
	if err != nil {
		t.Fatalf("prepare test dir error:%v", err)
	}

	files := map[string]string{
		"a1.txt":       "hello",
		"b1.txt":       "hello b",
		"c1/a2.txt":    "hello what",
		"c1/b2/a3.txt": "hello c3",
	}
	err = createFiles(src, files)
	if err != nil {
		t.Fatalf("create files error:%v", err)
	}

	err = SyncRebase([]string{src}, dest, SyncRebaseOptions{
		DeleteNotFound: true,
		OnUpdateStats:  newLogger(t),
	})
	if err != nil {
		t.Fatalf("sync failed:%v", err)
	}

	for name, content := range files {
		rcontent, err := ioutil.ReadFile(path.Join(dest, src, name))
		if err != nil {
			t.Fatalf("read %v failed:%v", name, err)
		}
		if content != string(rcontent) {
			t.Fatalf("file:%v not same", name)
		}
	}

	// create some temp file, temp dir
	destSrc := path.Join(dest, src)
	delFiles := map[string]string{
		"del_1/x.txt":    "hello x",
		"del_2/x/yx.txt": "hello x",
		"x.txt":          "hello x",
	}
	err = createFiles(destSrc, delFiles)
	if err != nil {
		t.Fatalf("create files error:%v", err)
	}

	err = SyncRebase([]string{src}, dest, SyncRebaseOptions{
		DeleteNotFound: true,
		OnUpdateStats:  newLogger(t),
	})
	if err != nil {
		t.Fatalf("resync error:%v", err)
	}
	for delName := range delFiles {
		_, err := os.Stat(path.Join(destSrc, delName))
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expect file:%v to be deleted", delName)
		}
	}

	// compare original name
	for name, content := range files {
		rcontent, err := ioutil.ReadFile(path.Join(dest, src, name))
		if err != nil {
			t.Fatalf("read %v failed:%v", name, err)
		}
		if content != string(rcontent) {
			t.Fatalf("file:%v not same", name)
		}
	}
}
func TestSyncRebasedCustom(t *testing.T) {
	err := SyncRebase([]string{
		// define your roots here
	}, "/tmp/xyz", SyncRebaseOptions{
		DeleteNotFound: true,
		Ignores:        []string{"(.*/)?\\.git\\b", "(.*/)?node_modules\\b"},
		OnUpdateStats:  newLogger(t),
	})
	if err != nil {
		t.Fatalf("sync failed:%v", err)
	}
}

func newLogger(t *testing.T) func(total, finished, copied int64, lastStat bool) {
	return NewLogger(func(format string, args ...interface{}) {
		t.Logf(format, args...)
	}, true, true, 100*time.Millisecond)
}
func prepareDir(caseName string) (src string, dest string, err error) {
	tmp := os.TempDir()
	simpleRoot := path.Join(tmp, "filecopy", "test", caseName)
	src = path.Join(simpleRoot, "src")
	dest = path.Join(simpleRoot, "dest")

	// RemoveAll does not report error if the dir does not exist
	err = os.RemoveAll(src)
	if err != nil {
		return
	}
	err = os.RemoveAll(dest)
	if err != nil {
		return
	}

	err = os.MkdirAll(src, 0777)
	return
}

func createFiles(root string, files map[string]string) error {
	for name, content := range files {
		p := path.Join(root, name)
		err := os.MkdirAll(path.Dir(p), 0777)
		if err != nil {
			return err
		}
		err = ioutil.WriteFile(p, []byte(content), 0777)
		if err != nil {
			return err
		}
	}
	return nil
}

func TestSyncGeneratedSimple(t *testing.T) {
	src, dest, err := prepareDir("gen/simple")
	if err != nil {
		t.Fatalf("prepare test dir error:%v", err)
	}

	err = createFiles(src, map[string]string{
		"a/b/c-0.txt":    "soruce c-0",
		"a/b/c-1.txt":    "soruce c-1",
		"a/b/c2/c-1.txt": "soruce c2/c-1",
	})
	if err != nil {
		t.Fatalf("create files error:%v", err)
	}

	gen := map[string][]byte{
		"a/b/c.txt":    []byte("hello"),
		"a/b/c2/d.txt": []byte("hello d"),
	}

	err = SyncGeneratedMap(gen, dest, func(filePath, destPath string, destFileInfo os.FileInfo) bool {
		t.Logf("checking file:%v %v", filePath, destPath)
		t.Fatalf("should not call checker when all dest do not exist")
		return false
	}, SyncRebaseOptions{
		OnUpdateStats: newLogger(t),
	})
	if err != nil {
		t.Fatalf("sync failed:%v", err)
	}

	// now, resync
	err = SyncGeneratedMap(gen, dest, func(filePath, destPath string, destFileInfo os.FileInfo) bool {
		t.Logf("checking file:%v %v", filePath, destPath)
		if !destFileInfo.Mode().IsRegular() {
			return true
		}
		return false
	}, SyncRebaseOptions{
		OnUpdateStats: newLogger(t),
	})
	if err != nil {
		t.Fatalf("resync sync failed:%v", err)
	}

}
