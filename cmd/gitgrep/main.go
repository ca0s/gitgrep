package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/ca0s/gitgrep/gitdown"
	"github.com/ca0s/gitgrep/grep"
	"github.com/ca0s/gitgrep/measure"

	"github.com/flier/gohs/hyperscan"
	"github.com/pkg/profile"
)

type (
	MatchList struct {
		lookForInitializations bool
		strMatches             []string
		matches                []*regexp.Regexp
		hsMatchers             []*hyperscan.Pattern
	}

	MatchFile struct {
		Matches *MatchList
	}

	URLList struct {
		urls []string
	}
)

func (ml *MatchList) Set(value string) error {
	if !ml.lookForInitializations {
		return ml.doSet(value)
	}

	expressions := []string{
		`{EXP}\s*[:=]\s*`,
		`\[{EXP}\]\s*[:=]`,
	}

	for _, e := range expressions {
		expr := strings.Replace(e, "{EXP}", value, -1)
		err := ml.doSet(expr)
		if err != nil {
			return err
		}
	}

	return nil
}

func (ml *MatchList) doSet(value string) error {

	r, err := regexp.Compile(value)
	if err != nil {
		return err
	}

	ml.strMatches = append(ml.strMatches, value)
	ml.matches = append(ml.matches, r)

	hp := hyperscan.NewPattern(value, hyperscan.Caseless|hyperscan.MultiLine|hyperscan.SomLeftMost)
	ml.hsMatchers = append(ml.hsMatchers, hp)

	return nil
}

func (ml *MatchList) String() string {
	if ml != nil {
		return strings.Join(ml.strMatches, ", ")
	}
	return ""
}

func (mf *MatchFile) Set(value string) error {
	fd, err := os.Open(value)
	if err != nil {
		return err
	}

	defer fd.Close()

	scanner := bufio.NewScanner(fd)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		line := scanner.Text()
		err = mf.Matches.Set(line)
		if err != nil {
			return err
		}
	}

	return nil
}

func (mf *MatchFile) String() string {
	return mf.Matches.String()
}

func (ul *URLList) Set(value string) error {
	ul.urls = append(ul.urls, value)
	return nil
}

func (ul *URLList) String() string {
	return strings.Join(ul.urls, ", ")
}

func main() {
	var (
		gitLocation  string
		dataLocation string

		downloadMode string
		matchMode    string

		repoURLs URLList

		matches MatchList = MatchList{
			lookForInitializations: true,
		}
		matchFile MatchFile = MatchFile{Matches: &matches}

		enablePerf             bool
		doEvaluation           bool
		evaluationShowFindings bool

		downloader gitdown.GitDownloader
		grepper    grep.Grepper

		err error
	)

	flag.StringVar(&downloadMode, "mode", "clone", "Method for downloading the repo. Valid values are clone and zip")
	flag.StringVar(&matchMode, "matcher", "hs", "Method for matching. Valid values are hs and re")
	flag.StringVar(&gitLocation, "git-location", "mem", "Storage for the .git data. Valid values are fs and mem")
	flag.StringVar(&dataLocation, "data-location", "mem", "Storage for the repository contents. Valid values are fs and mem")
	flag.Var(&repoURLs, "repo", "Repository URLs")
	flag.BoolVar(&enablePerf, "perf", false, "Measure execution performance")
	flag.BoolVar(&doEvaluation, "evaluate", false, "Run an evaluation of performance combining all download + match modes")
	flag.BoolVar(&evaluationShowFindings, "evaluation-findings", false, "Show findings when evaluating modes")

	flag.Var(&matches, "match", "Regexps to look for in repository")
	flag.Var(&matchFile, "match-file", "File to load regexps from")
	flag.Parse()

	if enablePerf {
		defer profile.Start(profile.MemProfileHeap, profile.MemProfileRate(1)).Stop()
	}

	if doEvaluation {
		evaluateCombinations(repoURLs.urls, matches, evaluationShowFindings)
		return
	}

	switch downloadMode {
	case "clone":
		downloader, err = gitdown.NewCloneDownloader(gitdown.InMemory, gitdown.InMemory)
	case "zip":
		downloader = gitdown.NewZipDownloader(gitdown.InMemory)
	default:
		flag.Usage()
		return
	}

	switch matchMode {
	case "hs":
		grepper, err = grep.NewHyperscanGrepper(matches.strMatches)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error initializing HyperScan grepper: %s\n", err)
		}
	case "re":
		grepper = grep.NewReGrepper(matches.matches)
	default:
		flag.Usage()
		return
	}

	if err != nil {
		fmt.Fprint(os.Stderr, err)
		return
	}

	for _, repoURL := range repoURLs.urls {
		fmt.Printf("Downloading repo: %s, method = %s\n", repoURL, downloadMode)

		ms := measure.TimeMeasure{}
		ms.Start()

		repo, err := downloader.Download(repoURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error cloning: %s\n", err)
			return
		}

		fmt.Printf("%+v\n", repo)

		ms.End()
		fmt.Printf("\ttook %s\n", ms.Ellpsed())

		fmt.Printf("Grepping repo, method=%s\n", matchMode)

		ms.Start()

		results, err := grepper.Grep(repo.Filesystem())
		if err != nil {
			fmt.Fprintf(os.Stderr, "error grepping: %s\n", err)
			return
		}

		ms.End()
		fmt.Printf("\ttook %s\n", ms.Ellpsed())

		for _, result := range results {
			fmt.Printf("%s: %s\n", result.Path, result.Comment)
		}

		repo.Close()
	}
}

func evaluateCombinations(repos []string, matches MatchList, showFindings bool) {
	type urlGenerator func(url string) string

	cloneUrlGenerator := func(url string) string {
		return url
	}

	zipUrlGenerator := func(url string) string {
		suffixes := []string{
			"/archive/refs/heads/master.zip",
			"/archive/refs/heads/main.zip",
		}

		result := ""

		for _, s := range suffixes {
			zipURL := fmt.Sprintf("%s%s", url, s)

			r, err := http.Head(zipURL)
			if err != nil {
				continue
			}

			io.Copy(io.Discard, r.Body)
			r.Body.Close()

			result = zipURL
			break
		}

		return result
	}

	type combination struct {
		name       string
		downloader gitdown.GitDownloader
		grepper    grep.Grepper
		urlgen     urlGenerator
	}

	cloneMemoryDownloader, err := gitdown.NewCloneDownloader(gitdown.InMemory, gitdown.InMemory)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not create clone+memory downloader: %s\n", err)
		return
	}
	cloneMemoryDownloader.SetProgress(nil)

	cloneFsDownloader, err := gitdown.NewCloneDownloader(gitdown.InFilesystem, gitdown.InFilesystem)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not create clone+fs downloader: %s\n", err)
		return
	}
	cloneFsDownloader.SetProgress(nil)

	zipMemoryDownloader := gitdown.NewZipDownloader(gitdown.InMemory)
	zipFsDownloader := gitdown.NewZipDownloader(gitdown.InFilesystem)

	hsGrepper, err := grep.NewHyperscanGrepper(matches.strMatches)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error initializing HyperScan grepper: %s\n", err)
	}

	reGrepper := grep.NewReGrepper(matches.matches)

	var combinations []combination = []combination{
		{
			name:       "clone / re / fs",
			downloader: cloneFsDownloader,
			grepper:    reGrepper,
			urlgen:     cloneUrlGenerator,
		},
		{
			name:       "clone / re / mem",
			downloader: cloneMemoryDownloader,
			grepper:    reGrepper,
			urlgen:     cloneUrlGenerator,
		},
		{
			name:       "clone / hyperscan / fs",
			downloader: cloneFsDownloader,
			grepper:    hsGrepper,
			urlgen:     cloneUrlGenerator,
		},
		{
			name:       "clone / hyperscan / mem",
			downloader: cloneMemoryDownloader,
			grepper:    hsGrepper,
			urlgen:     cloneUrlGenerator,
		},
		{
			name:       "zip / re / fs",
			downloader: zipFsDownloader,
			grepper:    reGrepper,
			urlgen:     zipUrlGenerator,
		},
		{
			name:       "zip / re / mem",
			downloader: zipMemoryDownloader,
			grepper:    reGrepper,
			urlgen:     zipUrlGenerator,
		},
		{
			name:       "zip / hyperscan / fs",
			downloader: zipFsDownloader,
			grepper:    hsGrepper,
			urlgen:     zipUrlGenerator,
		},
		{
			name:       "zip / hyperscan / mem",
			downloader: zipMemoryDownloader,
			grepper:    hsGrepper,
			urlgen:     zipUrlGenerator,
		},
	}

	ms := &measure.TimeMeasure{}

	for _, c := range combinations {
		for _, repoURL := range repos {
			fmt.Printf("---------- %s\n", repoURL)
			fmt.Printf("measuring combination: %s\n", c.name)

			downloadRepoUrl := c.urlgen(repoURL)
			if downloadRepoUrl == "" {
				fmt.Fprintf(os.Stderr, "could not determine download url for %s\n", repoURL)
				continue
			}

			downloadStart := ms.Start()

			repo, err := c.downloader.Download(downloadRepoUrl)
			if err != nil {
				fmt.Fprintf(os.Stderr, "could not download repo: %s\n", err)
				continue
			}

			downloadEnd := ms.End()
			downloadTime := ms.Ellpsed()

			grepStart := ms.Start()

			findings, err := c.grepper.Grep(repo.Filesystem())
			if err != nil {
				fmt.Fprintf(os.Stderr, "could not grep: %s\n", err)
				continue
			}

			grepEnd := ms.End()
			grepTime := ms.Ellpsed()

			totalTime := downloadTime + grepTime

			fmt.Printf("-- download started at %s, finished at %s, took %s\n", downloadStart, downloadEnd, downloadTime)
			fmt.Printf("-- grepping started at %s, finished at %s, took %s\n", grepStart, grepEnd, grepTime)
			fmt.Printf("-- task     started at %s, finished at %s, took %s\n", downloadStart, grepEnd, totalTime)
			fmt.Printf("-- findings: %d\n", len(findings))

			if showFindings {
				for _, f := range findings {
					fmt.Printf("   - %s\n", f.Comment)
				}
			}

			fmt.Printf("\n\n")

			repo.Close()
		}
	}
}
