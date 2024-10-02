package gitdown

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"runtime"
	"strings"
	"syscall"
	"time"
)

type ZipDownloader struct {
	storageLocation DownloadLocation

	blocking    bool
	authStorage *AuthStorage
}

func NewZipDownloader(storageLocation DownloadLocation) *ZipDownloader {
	return &ZipDownloader{
		storageLocation: storageLocation,
	}
}

func (d *ZipDownloader) Download(repoURL string) (*Repo, error) {
	info := syscall.Sysinfo_t{}
	syscall.Sysinfo(&info)
	maxRamCore := (info.Totalram / 10 * 7) / uint64(runtime.NumCPU())

	maxZipFileSize := maxRamCore / 5
	maxZipUncompressedSize := maxZipFileSize * 4

	zipFile, releaser, err := d.downloadZip(repoURL, maxZipFileSize)
	if err != nil {
		if releaser != nil {
			releaser()
		}

		return nil, err
	}

	var size uint64
	for _, entry := range zipFile.File {
		size += entry.UncompressedSize64
	}

	if size > maxZipUncompressedSize {
		d.storageLocation = InFilesystem
	} else {
		d.storageLocation = InMemory
	}

	storage, err := createStorage(d.storageLocation)
	if err != nil {
		if releaser != nil {
			releaser()
		}

		return nil, err
	}

	workFS := storage.Filesystem()

	for _, entry := range zipFile.File {
		if strings.HasSuffix(entry.Name, "/") {
			if _, err := workFS.Stat(entry.Name); err != nil {
				err = workFS.MkdirAll(entry.Name, os.ModeDir)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error mapping zip to billy, could not create dir: %s\n", err)
					continue
				}
			}
			continue
		}

		baseDir := path.Dir(entry.Name)
		if baseDir != "." && baseDir != "/" {
			if _, err := workFS.Stat(baseDir); err != nil {
				err = workFS.MkdirAll(baseDir, os.ModeDir)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error mapping zip to billy, could not create parent dir: %s\n", err)
					continue
				}
			}
		}

		f, err := workFS.Create(entry.Name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error mapping zip to billy, could not create file: %s\n", err)
			continue
		}

		entryFd, err := entry.Open()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error mapping zip to billy, could not open entry: %s\n", err)
			continue
		}

		_, err = io.Copy(f, entryFd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error mapping zip to billy, could not copydata: %s\n", err)
			continue
		}

		entryFd.Close()
		f.Close()
	}

	return &Repo{
		workFS:   storage,
		releaser: releaser,
	}, nil
}

func (cd *ZipDownloader) SetBlocking(b bool) {
	cd.blocking = b
}

func (cd *ZipDownloader) SetAuthStorage(s *AuthStorage) {
	cd.authStorage = s
}

func (d *ZipDownloader) downloadZip(zipURL string, maxInMemory uint64) (*zip.Reader, func(), error) {
	for {
		req, err := http.NewRequest(http.MethodGet, zipURL, nil)
		if err != nil {
			return nil, nil, err
		}

		if d.authStorage != nil {
			u, err := url.Parse(zipURL)
			if err != nil {
				return nil, nil, err
			}

			authData := d.authStorage.GetSiteAuth(u.Host)
			if authData != nil {
				req.Header.Add(authData.Name, authData.Value)
			}
		}

		r, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, nil, err
		}

		defer r.Body.Close()

		if r.StatusCode == http.StatusForbidden {
			if d.blocking {
				log.Printf("[%s] Zip download forbidden, waiting 20 seconds\n", zipURL)
				time.Sleep(20 * time.Second)
				continue
			}
		}

		safeBuffer := make([]byte, maxInMemory)

		nread, err := io.ReadFull(r.Body, safeBuffer)

		firstChunkReader := bytes.NewReader(safeBuffer)

		if err == io.ErrUnexpectedEOF {
			zipReader, err := zip.NewReader(firstChunkReader, int64(nread))

			if err != nil {
				return nil, nil, err
			}

			return zipReader, nil, nil
		}

		log.Printf("[%s] Zip does not fit in memory, dumping to disk\n", zipURL)

		fd, err := ioutil.TempFile("/tmp", "gitmon_zip_")
		if err != nil {
			return nil, nil, err
		}

		releaser := func() {
			fd.Close()
			os.Remove(fd.Name())
		}

		totalSize := int64(0)

		copied, err := io.Copy(fd, firstChunkReader)
		if err != nil {
			return nil, releaser, err
		}

		totalSize += copied

		copied, err = io.Copy(fd, r.Body)
		if err != nil {
			return nil, releaser, err
		}

		totalSize += copied

		newPos, err := fd.Seek(0, 0)
		if err != nil || newPos != 0 {
			return nil, releaser, fmt.Errorf("error seeking, pos = %d: %s", newPos, err)
		}

		zipReader, err := zip.NewReader(fd, totalSize)
		if err != nil {
			return nil, releaser, err
		}

		return zipReader, releaser, nil
	}
}
