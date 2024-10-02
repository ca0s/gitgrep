package grep

import (
	"bytes"
	"fmt"
	"io/fs"

	"github.com/flier/gohs/hyperscan"
)

type HyperscanGrepper struct {
	hsDb       hyperscan.BlockDatabase
	hsScratch  *hyperscan.Scratch
	patternMap map[uint]string
}

func NewHyperscanGrepper(matches []string) (*HyperscanGrepper, error) {
	var patterns []*hyperscan.Pattern
	patternMap := make(map[uint]string)

	compileflag, err := hyperscan.ParseCompileFlag("L")
	if err != nil {
		return nil, err
	}

	for i, m := range matches {
		p := hyperscan.NewPattern(m, compileflag)

		if _, err := p.Info(); err != nil {
			return nil, fmt.Errorf("expression '%s' is not valid: %s", m, err)
		}

		p.Id = i
		patternMap[uint(p.Id)] = m
		patterns = append(patterns, p)
	}

	hsDb, err := hyperscan.NewBlockDatabase(patterns...)
	if err != nil {
		return nil, err
	}

	hsScratch, err := hyperscan.NewScratch(hsDb)
	if err != nil {
		return nil, fmt.Errorf("error creating HS scratch: %s", err)
	}

	return &HyperscanGrepper{
		hsDb:       hsDb,
		hsScratch:  hsScratch,
		patternMap: patternMap,
	}, nil
}

func (hsg HyperscanGrepper) Grep(fss interface{}, options ...GrepOption) ([]Result, error) {
	var results []Result

	type scanCtx struct {
		inputData []byte
		fileName  string
	}

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
			// to-do: log this
			return nil
		}

		for _, option := range options {
			if option.SkipFileContent(content) {
				return nil
			}
		}

		handler := hyperscan.MatchHandler(func(id uint, from, to uint64, flags uint, context interface{}) error {
			const maxLineDelta = uint64(32)
			ctx := context.(*scanCtx)

			inputData := ctx.inputData

			var lineStart uint64
			var lineEnd uint64

			_lineStart := bytes.LastIndexByte(inputData[:from], '\n')
			_lineEnd := bytes.IndexByte(inputData[to:], '\n')

			if _lineStart == -1 {
				lineStart = 0
			} else {
				lineStart = uint64(_lineStart) + 1
			}

			if _lineEnd == -1 {
				lineEnd = uint64(len(inputData)) - 1
			} else {
				lineEnd = to + uint64(_lineEnd)
			}

			if from-lineStart > maxLineDelta {
				if from-maxLineDelta < 0 {
					lineStart = 0
				} else {
					lineStart = from - maxLineDelta
				}
			}

			if lineEnd-to > maxLineDelta {
				if to+maxLineDelta >= uint64(len(inputData)) {
					lineEnd = uint64(len(inputData)) - 1
				} else {
					lineEnd = to + maxLineDelta
				}
			}

			line := inputData[lineStart:lineEnd]
			pattern, ok := hsg.patternMap[id]
			if !ok {
				pattern = "<unknown>"
			}

			results = append(results, Result{
				PatternID: id,
				Pattern:   pattern,
				Path:      path,
				Content:   string(inputData[from:to]),
				Comment:   fmt.Sprintf("%s: [%s] %s", ctx.fileName, inputData[from:to], line),
			})

			return nil
		})

		err = hsg.hsDb.Scan(
			content,
			hsg.hsScratch,
			handler,
			&scanCtx{
				inputData: content,
				fileName:  path,
			},
		)

		return err
	})

	var tmp []Result

	for i, match := range results {
		if i == (len(results)-1) || results[i+1].PatternID != match.PatternID || (len(results[i+1].Content) < len(match.Content) && results[i+1].PatternID == match.PatternID) {
			tmp = append(tmp, match)
		}
	}

	results = tmp

	return results, err
}

func (hsg HyperscanGrepper) Release() {
	hsg.hsScratch.Free()
	hsg.hsDb.Close()
}
