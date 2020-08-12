package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
)

func getGithubClient(ctx context.Context, token string) *github.Client {
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

func fileUpdate(path, contents, encoding string) error {
	switch encoding {
	case "base64":
		b64d, err := base64.StdEncoding.DecodeString(contents)
		if err != nil {
			return errors.WithStack(err)
		}
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return errors.WithStack(err)
		}
		if err = ioutil.WriteFile(path, b64d, 0644); err != nil {
			return errors.WithStack(err)
		}
	default:
		return errors.New(fmt.Sprintf("%s: unsupported encoding: %s\n", path, encoding))
	}
	return nil
}

func gitResetHard(commitID string) error {
	cmd := exec.Command("git", "reset", "--hard", commitID)
	_, err := cmd.Output()
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

type gitResetCall struct {
	info   *repoInfo
	ctx    context.Context
	client *github.Client
}

func newGitResetCall(ctx context.Context, client *github.Client, info *repoInfo, token string) *gitResetCall {
	return &gitResetCall{info: info, ctx: ctx, client: client}
}

func (c *gitResetCall) gitReset(commitID string) error {
	tree, _, err := c.client.Git.GetTree(c.ctx, c.info.owner, c.info.repo, commitID, true)
	if err != nil {
		return errors.WithStack(err)
	}

	for _, entry := range tree.Entries {
		if entry.GetType() == "tree" {
			continue
		}
		blob, _, err := c.client.Git.GetBlob(c.ctx, c.info.owner, c.info.repo, entry.GetSHA())
		if err != nil {
			return errors.WithStack(err)
		}
		if err = fileUpdate(entry.GetPath(), blob.GetContent(), blob.GetEncoding()); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func removeTrackingFiles() error {
	cmd := exec.Command("git", "ls-tree", "-r", "master", "--name-only")
	out, err := cmd.Output()
	if err != nil {
		return errors.WithStack(err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		line := scanner.Text()
		if err = os.Remove(line); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

func main() {
	token, ok := os.LookupEnv("AUTH_TOKEN")
	if !ok {
		panic(errors.New("AUTH_TOKEN must be defined"))
	}

	if len(os.Args) != 2 {
		fmt.Printf("Usage: %s commit-id\n", os.Args[0])
		return
	}

	commitID := os.Args[1]
	info, err := getRepoInfo()
	if err != nil {
		panic(err)
	}

	// 1st, try to use git command. quit if no error
	if err = gitResetHard(commitID); err == nil {
		return
	}

	if info.saas != "github.com" {
		fmt.Printf("%s is not supported\n", info.saas)
		return
	}

	if err = removeTrackingFiles(); err != nil {
		panic(err)
	}

	ctx := context.Background()
	client := getGithubClient(ctx, token)
	call := newGitResetCall(ctx, client, info, token)

	if err = call.gitReset(commitID); err != nil {
		panic(err)
	}
}
