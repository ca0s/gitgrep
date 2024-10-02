package grep

import (
	"fmt"
	"io/fs"
	"regexp"
)

type ReGrepper struct {
	res []*regexp.Regexp
}

func NewReGrepper(matches []*regexp.Regexp) *ReGrepper {
	return &ReGrepper{
		res: matches,
	}
}

func (g ReGrepper) Grep(fss interface{}, options ...GrepOption) ([]Result, error) {
	var results []Result

	err := Walk(fss, func(path string, info fs.FileInfo, cberr error) error {
		if cberr != nil {
			return cberr
		}

		if info.IsDir() {
			return nil
		}

		for _, option := range options {
			if option.SkipFile(path) {
				return nil
			}
		}

		content, err := ReadFile(fss, path)
		if err != nil {
			return err
		}

		for _, option := range options {
			if option.SkipFileContent(content) {
				return nil
			}
		}

		for _, m := range g.res {
			findings := m.FindAll(content, -1)

			for _, f := range findings {
				results = append(results, Result{
					Pattern: m.String(),
					Path:    path,
					Content: string(f),
					Comment: fmt.Sprintf("%s: %s", path, f),
				})
			}
		}

		return nil
	})

	return results, err
}

func (g ReGrepper) Release() {}
