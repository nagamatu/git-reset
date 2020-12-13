package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
)

func newGithubClient(ctx context.Context, token string) *github.Client {
	if token == "" {
		missingToken()
		/*NOTREACHED*/
	}
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(tc)
}

type repoInfo struct {
	saas  string
	owner string
	repo  string
}

func parseKeyValue(s string) (string, string, error) {
	ss := strings.Split(s, "=")
	if len(ss) != 2 {
		return "", "", errors.New("toml syntax error")
	}
	return strings.TrimSpace(ss[0]), strings.TrimSpace(ss[1]), nil
}

var tableRegex = regexp.MustCompile("\\[([^\\]]*)\\]")

func parseRemoteURL(data []byte) (*url.URL, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Split(bufio.ScanLines)

	urlStr := ""
	table := ""
	for scanner.Scan() {
		line := scanner.Text()
		ss := tableRegex.FindStringSubmatch(line)
		if len(ss) == 2 {
			table = ss[1]
		} else {
			key, value, err := parseKeyValue(line)
			if err != nil {
				return nil, err
			}
			if strings.HasPrefix(table, "remote") && strings.ToLower(key) == "url" {
				urlStr = value
			}
		}
	}

	return url.Parse(urlStr)
}

func getRepoInfo() (*repoInfo, error) {
	f, err := os.Open(".git/config")
	if err != nil {
		return nil, errors.New("not a git repository (or any of the parent directories): .git")
	}
	defer f.Close()

	tomlData, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	u, err := parseRemoteURL(tomlData)
	if err != nil {
		return nil, err
	}

	ss := strings.Split(u.Path, "/")
	if len(ss) < 2 {
		return nil, errors.New(fmt.Sprintf(".git/config has invalid remote URL: %s", u.Path))
	}
	return &repoInfo{saas: u.Host, owner: ss[1], repo: ss[2]}, nil
}

func isRateLimitThenWait(resp *github.Response, err error) bool {
	if rateLimitErr, ok := err.(*github.RateLimitError); ok {
		dur := rateLimitErr.Rate.Reset.Time.Sub(time.Now()) + 30*time.Second
		fmt.Fprintf(os.Stderr, "rate limit: waiting for %v\n", dur)
		time.Sleep(dur)
		return true
	}

	return false
}

type gitCall struct {
	info   *repoInfo
	ctx    context.Context
	client *github.Client
}

func newGitCall(ctx context.Context, client *github.Client, info *repoInfo, token string) *gitCall {
	return &gitCall{info: info, ctx: ctx, client: client}
}

func usage(name string) {
	fmt.Fprintf(os.Stderr, "Must have a name \"%s\".\n", name)
	os.Exit(-1)
}

type gitCommand struct {
	name string
	f    func(string) error
}

var commands = []gitCommand{
	{"git-create-reference", gitCreateReference},
	{"git-diff-numstat", gitDiffNumstat},
	{"git-get-file", gitGetFile},
	{"git-log", gitLog},
	{"git-reset", gitReset},
	{"git-show-date", gitShowDate},
	{"git-rev-parse", gitRevParse},
	{"git-get-pr-merge-commit", gitGetPRMergeCommit},
}

var errNoSuchCommand = errors.New("no such command")

func missingToken() {
	fmt.Fprintf(os.Stderr, "AUTH_TOKEN must be defined\n")
	os.Exit(-1)
}

func main() {
	token, _ := os.LookupEnv("AUTH_TOKEN")

	name := path.Base(os.Args[0])
	err := errNoSuchCommand
	for _, cmd := range commands {
		if cmd.name == name {
			err = cmd.f(token)
			break
		}
	}

	if err != nil {
		if err == errNoSuchCommand {
			usage(name)
			/*NOTREACHED*/
		} else {
			fmt.Fprintf(os.Stderr, "\nerror: %+v\n", err)
			os.Exit(-1)
		}
	}
}
