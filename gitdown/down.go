package gitdown

import (
	"fmt"
	"os"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/osfs"
)

type DownloadLocation int

const (
	InMemory DownloadLocation = iota
	InFilesystem
)

var ErrInvalidLocation = fmt.Errorf("invalid location")

type GitDownloader interface {
	Download(string) (*Repo, error)
	SetBlocking(bool)
	SetAuthStorage(*AuthStorage)
}

type Repo struct {
	storeFS  *Storage
	workFS   *Storage
	releaser func()
}

func (r *Repo) Filesystem() billy.Filesystem {
	return r.workFS.Filesystem()
}

func (r *Repo) Close() {
	if r.storeFS != nil {
		r.storeFS.Close()
	}

	if r.workFS != nil {
		r.workFS.Close()
	}

	if r.releaser != nil {
		r.releaser()
	}
}

type Storage struct {
	fs       billy.Filesystem
	location DownloadLocation
	path     string
}

func (s *Storage) Filesystem() billy.Filesystem {
	return s.fs
}

func (s *Storage) Close() {
	if s.location == InFilesystem && s.path != "" {
		os.RemoveAll(s.path)
	}
}

func createStorage(location DownloadLocation) (*Storage, error) {
	var storage billy.Filesystem
	var tmpPath string
	var err error

	switch location {
	case InFilesystem:
		tmpPath, err = os.MkdirTemp("/tmp", "gitmon_*")
		if err != nil {
			return nil, err
		}

		storage = osfs.New(tmpPath)

	case InMemory:
		storage = memfs.New()

	default:
		return nil, ErrInvalidLocation
	}

	return &Storage{
		fs:       storage,
		location: location,
		path:     tmpPath,
	}, nil
}
