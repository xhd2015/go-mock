package filecopy

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// sync model:
//     files -> rebaseDir/${file}
// file can either be file or dir, but links are ignored.

type SyncRebaseOptions struct {
	// Ignores tells the syncer that these dirs are not synced
	Ignores []string

	DeleteNotFound bool

	Force bool

	OnUpdateStats func(total int64, finished int64, copied int64, lastStat bool)

	ProcessDestPath func(s string) string

	DidCopy func(srcPath string, destPath string)
}

func SyncRebase(initPaths []string, rebaseDir string, opts SyncRebaseOptions) error {
	return Sync(initPaths, &rebaseSourcer{rebaseDir: rebaseDir}, opts)
}

// SyncGeneratedMap
// when `sourceNewerChecker` returns true, the target file is overwritten.
func SyncGeneratedMap(contents map[string][]byte, targetBaseDir string, sourceNewerChecker func(filePath, destPath string, destFileInfo os.FileInfo) bool, opts SyncRebaseOptions) error {
	return doSync(func(fn func(path string)) {
		for path := range contents {
			fn(path)
		}
	}, &mapSourcer{
		baseDir: targetBaseDir,
		getContent: func(name string) []byte {
			content, ok := contents[name]
			if !ok {
				panic(fmt.Errorf("unexpected name:%v", name))
			}
			return content
		},
		sourceNewerChecker: sourceNewerChecker,
	}, opts)
}
func SyncGenerated(ranger func(fn func(path string)), contentGetter func(name string) []byte, targetBaseDir string, sourceNewerChecker func(filePath, destPath string, destFileInfo os.FileInfo) bool, opts SyncRebaseOptions) error {
	return doSync(ranger, &mapSourcer{
		baseDir:            targetBaseDir,
		getContent:         contentGetter,
		sourceNewerChecker: sourceNewerChecker,
	}, opts)
}

// SyncRebaseContents implements wise file sync, srcs are all sync into `rebaseDir`
// for generated contents, there is no physical content,
//
func Sync(initPaths []string, sourcer SyncSourcer, opts SyncRebaseOptions) error {
	return doSync(func(fn func(path string)) {
		for _, path := range initPaths {
			fn(path)
		}
	}, sourcer, opts)
}

func doSync(ranger func(fn func(path string)), sourcer SyncSourcer, opts SyncRebaseOptions) error {
	shouldIgnore := newRegexMatcher(opts.Ignores)
	readDestDir := func(dest string) (destFiles []os.FileInfo, destDirMade bool, err error) {
		destFiles, err = ioutil.ReadDir(dest)
		if err == nil {
			destDirMade = true
			return
		}
		if errors.Is(err, os.ErrNotExist) {
			destDirMade = false
			err = nil
			return
		}
		// rare case: is a file
		fs, fsErr := os.Stat(dest)
		if fsErr == nil && !fs.IsDir() {
			// TODO: add an option to indicate overwrite
			err = nil
			rmErr := os.RemoveAll(dest)
			if rmErr != nil {
				err = fmt.Errorf("remove existing dest file error:%v", rmErr)
				return
			}
		}
		// may have err
		return
	}
	// isDir && !isRegular can be true the same time.
	checkFileCopyable := func(srcFile FileInfo) (isFile bool, isDir bool) {
		isDir = srcFile.IsDir()
		if isDir {
			return
		}
		isFile = srcFile.IsFile()
		return
	}
	var totalFiles int64
	var finishedFiles int64
	var copiedFiles int64

	lastStat := false

	onUpdateStats := func() {
		if opts.OnUpdateStats != nil {
			opts.OnUpdateStats(atomic.LoadInt64(&totalFiles), atomic.LoadInt64(&finishedFiles), atomic.LoadInt64(&copiedFiles), lastStat)
		}
	}
	didCopy := func(srcPath, destPath string) {
		if opts.DidCopy != nil {
			opts.DidCopy(srcPath, destPath)
		}
	}
	processDestPath := func(s string) string {
		if opts.ProcessDestPath == nil {
			return s
		}
		return opts.ProcessDestPath(s)
	}

	var waitGroup sync.WaitGroup

	const chSize = 1000
	const gNum = 100 // 400M memory at most

	var mutext sync.Mutex
	var panicErr interface{}

	type info struct {
		path     string
		fileInfo FileInfo
	}

	var res sync.Map
	ch := make(chan info, chSize)
	exhaustCh := func() {
		defer func() {
			if e := recover(); e != nil {
				if panicErr == nil {
					mutext.Lock()
					if panicErr == nil {
						panicErr = e
					}
					mutext.Unlock()
				}
			}
		}()
		var buf []byte
		for metaInfo := range ch {
			srcPath := metaInfo.path
			srcFileInfo := metaInfo.fileInfo
			err := func(srcPath string, srcFileInfo FileInfo) error {
				// fmt.Printf("DEBUG sync file:%v\n", srcPath)
				defer waitGroup.Done()
				if shouldIgnore(srcPath) {
					return nil
				}
				destPath := processDestPath(sourcer.GetDestPath(srcPath))
				if srcFileInfo == nil {
					var err error
					srcFileInfo, err = sourcer.GetSrcFileInfo(srcPath)
					if err != nil {
						return err
					}
					if !srcFileInfo.IsDir() && !srcFileInfo.IsFile() {
						// not a dir nor a file,so nothing to do
						return nil
					}
				}

				// generated file will always go handleFile, no handleDir called
				handleFile := func(srcFileInfo FileInfo, destPath string) error {
					atomic.AddInt64(&totalFiles, 1)
					onUpdateStats()

					destFile, err := os.Stat(destPath)
					needCopy := false
					needDelDest := false
					if err != nil {
						if !errors.Is(err, os.ErrNotExist) {
							return err
						}
						needCopy = true
					} else {
						needDelDest = !destFile.Mode().IsRegular()
						needCopy = needDelDest || (opts.Force || srcFileInfo.NewerThan(destPath, destFile))
					}
					if !needCopy {
						atomic.AddInt64(&finishedFiles, 1)
						onUpdateStats()
						return nil
					}
					if needDelDest {
						err = os.RemoveAll(destPath)
						if err != nil {
							return err
						}
					}
					if buf == nil {
						buf = make([]byte, 0, 4*1024*1024) // 4MB
					}

					// copy file
					didCopy(srcFileInfo.GetPath(), destPath)
					err = copyFile(srcFileInfo, destPath, buf)
					if err != nil {
						return err
					}

					atomic.AddInt64(&copiedFiles, 1)
					atomic.AddInt64(&finishedFiles, 1)
					onUpdateStats()

					return nil
				}
				handleDir := func(srcFileInfo FileInfo, destPath string) error {
					srcPath := srcFileInfo.GetPath()
					destFiles, destDirMade, err := readDestDir(destPath)
					if err != nil {
						return err
					}

					childSrcFiles, err := sourcer.GetSrcChildFiles(srcPath)
					if err != nil {
						return fmt.Errorf("read src dir error:%v", err)
					}

					var destMap map[string]os.FileInfo
					var needDeletes map[string]bool
					if len(destFiles) > 0 {
						destMap = make(map[string]os.FileInfo, len(destFiles))
						needDeletes = make(map[string]bool, len(destFiles))
						for _, destFile := range destFiles {
							// NOTE: very prone to bug: if directly take address of destFile, you will get wrong result
							// d := destFile
							// destMap[destFile.Name()] = &d

							destMap[destFile.Name()] = destFile
							needDeletes[destFile.Name()] = true
						}
					}
					// for each file
					for _, childSrcFile := range childSrcFiles {
						isFile, isDir := checkFileCopyable(childSrcFile)
						if !isFile && !isDir {
							// for non-regular files, do not copy
							// also need delete from dst if any
							continue
						}

						fileName := childSrcFile.GetName()
						// mark no delete
						if _, ok := needDeletes[fileName]; ok {
							needDeletes[fileName] = false
						}
						if isDir {
							continue
						}

						// total stat
						atomic.AddInt64(&totalFiles, 1)
						onUpdateStats()
						if !opts.Force {
							destFile := destMap[fileName]
							if destFile != nil {
								// fmt.Printf("DEBUG file:%v, src modTime:%v, dst modTime:%v, before:%v\n", path.Join(dir, srcFile.Name()), srcFile.ModTime(), destFile.ModTime(), srcFile.ModTime().Before(destFile.ModTime()))

								// mod time is not so accurate, MD5 is a more generalized and stable way.
								// but we actually can use length to
								// we need
								// actually
								if !childSrcFile.NewerThan(path.Join(destPath, fileName), destFile) {
									atomic.AddInt64(&finishedFiles, 1)
									onUpdateStats()
									continue
								}
							}
						}

						// fmt.Printf("DEBUG will copy file:%v\n", path.Join(dir, srcFile.Name()))

						if !destDirMade {
							err = os.MkdirAll(destPath, 0777)
							if err != nil {
								return fmt.Errorf("create dest dir error:%v", err)
							}
							destDirMade = true
						}
						if buf == nil {
							buf = make([]byte, 0, 4*1024*1024) // 4MB
						}

						// copy file
						childDestPath := path.Join(destPath, fileName)
						didCopy(childSrcFile.GetPath(), childDestPath)
						err = copyFile(childSrcFile, childDestPath, buf)
						if err != nil {
							return err
						}
						// copy stat
						atomic.AddInt64(&copiedFiles, 1)
						atomic.AddInt64(&finishedFiles, 1)
						onUpdateStats()
					}

					if opts.DeleteNotFound {
						// remove uncover names
						for name, needDelete := range needDeletes {
							if needDelete {
								err = os.RemoveAll(path.Join(destPath, name))
								if err != nil {
									return fmt.Errorf("remove file error:%v", err)
								}
							}
						}
					}

					// send dir to sub ch
					for _, srcFile := range childSrcFiles {
						if !srcFile.IsDir() {
							continue
						}
						waitGroup.Add(1)
						ch <- info{path: srcFile.GetPath(), fileInfo: srcFile}
					}
					return nil
				}

				if srcFileInfo.IsDir() {
					return handleDir(srcFileInfo, destPath)
				} else if srcFileInfo.IsFile() {
					return handleFile(srcFileInfo, destPath)
				}
				return nil
			}(srcPath, srcFileInfo)
			if err != nil {
				res.Store(srcPath, err)
				return
			}
		}
	}
	// start 10 goroutines to do the work
	for i := 0; i < gNum; i++ {
		go exhaustCh()
	}

	// write initial roots
	ranger(func(path string) {
		waitGroup.Add(1)
		ch <- info{path: path}
	})

	waitGroup.Wait()
	close(ch)

	// fmt.Printf("DEBUG: total files:%d, copied: %d\n", totalFiles, copiedFiles)
	lastStat = true
	onUpdateStats()

	if panicErr != nil {
		return fmt.Errorf("panic:%v", panicErr)
	}

	var errList []string
	res.Range(func(key, value interface{}) bool {
		errList = append(errList, fmt.Sprintf("dir:%v %v", key, value))
		return true
	})

	if len(errList) > 0 {
		return fmt.Errorf("%s", strings.Join(errList, ";"))
	}

	return nil
}

func newRegexMatcher(re []string) func(s string) bool {
	if len(re) == 0 {
		return func(s string) bool {
			return false
		}
	}
	regex := make([]*regexp.Regexp, len(re))
	return func(s string) bool {
		for i, r := range re {
			rg := regex[i]
			if rg == nil {
				rg = regexp.MustCompile(r)
				regex[i] = rg
			}
			if rg.MatchString(s) {
				return true
			}
		}
		return false
	}
}

func copyFile(srcFile FileInfo, destPath string, buf []byte) (err error) {
	// defer func() {
	// 	fmt.Printf("DEBUG copy file DONE:%v\n", srcFile)
	// }()
	// fmt.Printf("DEBUG copy file:%v\n", srcFile)
	// if srcFile != "/Users/x/gopath/src/xxx.log" {
	// 	return nil
	// }
	// fmt.Printf("DEBUG copy %v/%v\n", srcDir, name)
	srcFileIO, err := srcFile.Open()
	// srcFileIO, err := os.OpenFile(srcPath, os.O_RDONLY, 0777)
	if err != nil {
		err = fmt.Errorf("open src file error:%v", err)
		return
	}
	if srcFileIOCloser, ok := srcFileIO.(io.Closer); ok {
		defer srcFileIOCloser.Close()
	}
	// TODO: add a flag to indicate whether mkdir is needed
	err = os.MkdirAll(path.Dir(destPath), 0777)
	if err != nil {
		err = fmt.Errorf("create dir %v error:%v", path.Dir(destPath), err)
		return
	}
	destFileIO, err := os.OpenFile(destPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0777)
	if err != nil {
		err = fmt.Errorf("create dest file error:%v", err)
		return
	}
	defer func() {
		destFileIO.Close()
		if err == nil {
			// fsrc, _ := os.Stat(srcFile)
			// fb, _ := os.Stat(destName)
			// fmt.Printf("DEBUG before modify time:%v -> %v\n", destName, fb.ModTime())
			now := time.Now()
			// now := fsrc.ModTime().Add(24 * time.Hour)
			// fmt.Printf("DEBUG will modify time:%v -> %v\n", destName, now)
			err = os.Chtimes(destPath, now, now)
			// f, _ := os.Stat(destName)
			// fmt.Printf("DEBUG after modify time:%v -> %v\n", destName, f.ModTime())
		}
	}()

	for {
		var n int
		n, err = srcFileIO.Read(buf[0:cap(buf)])
		if n > 0 {
			var m int
			m, err = destFileIO.Write(buf[:n])
			if err != nil {
				err = fmt.Errorf("write dest file error:%v", err)
				return
			}
			if m != n {
				err = fmt.Errorf("copy file error, unexpected written bytes:%d, want:%d", m, n)
				return
			}
		}
		if errors.Is(err, io.EOF) {
			err = nil
			return
		}
		if err != nil {
			return
		}
	}
}

func NewLogger(log func(format string, args ...interface{}), enable bool, enableLast bool, interval time.Duration) func(total, finished, copied int64, lastStat bool) {
	var last time.Time
	return func(total, finished, copied int64, lastStat bool) {
		if !enable {
			if !lastStat || !enableLast {
				return
			}
		}
		now := time.Now()
		if !lastStat && now.Sub(last) < interval {
			return
		}
		last = now
		log("copy %.2f%%  total:%4d, finished:%4d, changed:%4d", float64(finished+1)/float64(total+1)*100, total, finished, copied)
		if lastStat {
			log("copy finished")
		}
	}
}
