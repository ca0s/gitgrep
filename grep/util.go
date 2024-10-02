package grep

// taken from go-billy @ main. these utils have not made it yet to a release

import (
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/util"
)

func walk(fs billy.Filesystem, path string, info os.FileInfo, walkFn filepath.WalkFunc) error {
	if !info.IsDir() {
		return walkFn(path, info, nil)
	}

	names, err := readdirnames(fs, path)
	err1 := walkFn(path, info, err)
	// If err != nil, walk can't walk into this directory.
	// err1 != nil means walkFn want walk to skip this directory or stop walking.
	// Therefore, if one of err and err1 isn't nil, walk will return.
	if err != nil || err1 != nil {
		// The caller's behavior is controlled by the return value, which is decided
		// by walkFn. walkFn may ignore err and return nil.
		// If walkFn returns SkipDir, it will be handled by the caller.
		// So walk should return whatever walkFn returns.
		return err1
	}

	for _, name := range names {
		filename := filepath.Join(path, name)
		fileInfo, err := fs.Lstat(filename)
		if err != nil {
			if err := walkFn(filename, fileInfo, err); err != nil && err != filepath.SkipDir {
				return err
			}
		} else {
			err = walk(fs, filename, fileInfo, walkFn)
			if err != nil {
				if !fileInfo.IsDir() || err != filepath.SkipDir {
					return err
				}
			}
		}
	}
	return nil
}

// Walk walks the file tree rooted at root, calling fn for each file or
// directory in the tree, including root. All errors that arise visiting files
// and directories are filtered by fn: see the WalkFunc documentation for
// details.
//
// The files are walked in lexical order, which makes the output deterministic
// but requires Walk to read an entire directory into memory before proceeding
// to walk that directory. Walk does not follow symbolic links.
//
// Function adapted from https://github.com/golang/go/blob/3b770f2ccb1fa6fecc22ea822a19447b10b70c5c/src/path/filepath/path.go#L500
func WalkBilly(fs billy.Filesystem, root string, walkFn filepath.WalkFunc) error {
	info, err := fs.Lstat(root)
	if err != nil {
		err = walkFn(root, nil, err)
	} else {
		err = walk(fs, root, info, walkFn)
	}

	if err == filepath.SkipDir {
		return nil
	}

	return err
}

func WalkFS(fss fs.FS, root string, walkFn filepath.WalkFunc) error {
	err := fs.WalkDir(fss, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		return walkFn(path, info, err)
	})

	return err
}

var ErrInvalidFS = fmt.Errorf("invalid filesystem implementation")

func Walk(fss interface{}, walkFn filepath.WalkFunc) error {
	switch fss.(type) {
	case billy.Filesystem:
		return WalkBilly(fss.(billy.Filesystem), "/", walkFn)
	case fs.FS:
		return WalkFS(fss.(fs.FS), ".", walkFn)
	default:
		return ErrInvalidFS
	}
}

func readdirnames(fs billy.Filesystem, dir string) ([]string, error) {
	files, err := fs.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, file := range files {
		names = append(names, file.Name())
	}

	return names, nil
}

func MinInt(a, b int) int {
	if a <= b {
		return a
	}

	return b
}

func ReadFile(fss interface{}, path string) ([]byte, error) {
	if bfss, ok := fss.(billy.Basic); ok {
		return util.ReadFile(bfss, path)
	}

	if ffss, ok := fss.(fs.FS); ok {
		fd, err := ffss.Open(path)
		if err != nil {
			return nil, err
		}

		return ioutil.ReadAll(fd)
	}

	return nil, ErrInvalidFS
}
