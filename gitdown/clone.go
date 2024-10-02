package gitdown

import (
	"fmt"
	"net/url"
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/sideband"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

type CloneDownloader struct {
	gitLocation  DownloadLocation
	dataLocation DownloadLocation

	progress    sideband.Progress
	blocking    bool
	authStorage *AuthStorage
}

func NewCloneDownloader(gitLocation DownloadLocation, dataLocation DownloadLocation) (*CloneDownloader, error) {
	return &CloneDownloader{
		gitLocation:  gitLocation,
		dataLocation: dataLocation,
		progress:     os.Stdout,
	}, nil
}

func (cd *CloneDownloader) SetProgress(p sideband.Progress) {
	cd.progress = p
}

func (cd *CloneDownloader) Download(repoURL string) (*Repo, error) {
	storeFS, err := createStorage(cd.gitLocation)
	if err != nil {
		return nil, err
	}

	workFS, err := createStorage(cd.dataLocation)
	if err != nil {
		return nil, err
	}

	var auth transport.AuthMethod

	if cd.authStorage != nil {
		u, err := url.Parse(repoURL)
		if err != nil {
			return nil, err
		}

		authData := cd.authStorage.GetSiteAuth(u.Host)
		if authData != nil {
			auth = &http.TokenAuth{
				Token: authData.Value,
			}
		}

	}

	_, err = git.Clone(
		filesystem.NewStorage(storeFS.Filesystem(), cache.NewObjectLRUDefault()),
		workFS.Filesystem(),
		&git.CloneOptions{
			URL:      repoURL,
			Depth:    1,
			Progress: cd.progress,
			Auth:     auth,
		})

	if err != nil {
		return nil, fmt.Errorf("error cloning: %s", err)
	}

	return &Repo{
		workFS:  workFS,
		storeFS: storeFS,
	}, nil
}

func (cd *CloneDownloader) SetBlocking(b bool) {
	cd.blocking = b
}

func (cd *CloneDownloader) SetAuthStorage(s *AuthStorage) {
	cd.authStorage = s
}
