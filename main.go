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
	"sort"
	"strings"

	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
)

func newGithubClient(ctx context.Context, token string) *github.Client {
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

func gitResetHardCmd(commitID string) error {
	cmd := exec.Command("git", "reset", "--hard", commitID)
	_, err := cmd.Output()
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func gitDiffNumstatCmd(baseCommitID, commitID string) error {
	cmd := exec.Command("git", "diff", "--numstat", baseCommitID, commitID)
	out, err := cmd.Output()
	if err != nil {
		return errors.WithStack(err)
	}
	fmt.Printf(string(out))
	return nil
}

type gitCall struct {
	info   *repoInfo
	ctx    context.Context
	client *github.Client
}

func newGitCall(ctx context.Context, client *github.Client, info *repoInfo, token string) *gitCall {
	return &gitCall{info: info, ctx: ctx, client: client}
}

func (c *gitCall) gitReset(commitID string) error {
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

type numStat struct {
	filepath  string
	additions int
	deletions int
}

func newNumStat(f *github.CommitFile) *numStat {
	return &numStat{filepath: f.GetFilename(), additions: f.GetAdditions(), deletions: f.GetDeletions()}
}

func (n *numStat) String() string {
	return fmt.Sprintf("%d\t%d\t%s", n.additions, n.deletions, n.filepath)
}

func (n *numStat) Add(f *github.CommitFile) *numStat {
	n.additions += f.GetAdditions()
	n.deletions += f.GetDeletions()
	return n
}

func (c *gitCall) gitDiffNumstat(baseCommitID, commitID string) error {
	numStatMap := make(map[string]*numStat)
	checkedSHA := make(map[string]bool)
	found := false
	parents := []*github.Commit{}
	maxDepth := 100
	for parents = append(parents, &github.Commit{SHA: &commitID}); !found && len(parents) > 0 && maxDepth > 0; maxDepth-- {
		nextParents := []*github.Commit{}
		for _, pc := range parents {
			checkedSHA[pc.GetSHA()] = true
			commit, _, err := c.client.Repositories.GetCommit(c.ctx, c.info.owner, c.info.repo, pc.GetSHA())
			if err != nil {
				return errors.WithStack(err)
			}

			for _, commitFile := range commit.Files {
				numStat := numStatMap[commitFile.GetFilename()]
				if numStat == nil {
					numStatMap[commitFile.GetFilename()] = newNumStat(commitFile)
				} else {
					numStatMap[commitFile.GetFilename()] = numStat.Add(commitFile)
				}
			}

			checkedSHA[commitID] = true

			for _, p := range commit.Parents {
				if p.GetSHA() == baseCommitID {
					found = true
					break
				}
				if checkedSHA[p.GetSHA()] {
					continue
				}
				checkedSHA[p.GetSHA()] = true
				nextParents = append(nextParents, p)
			}

			if found {
				break
			}
		}
		parents = nextParents
	}

	if !found {
		return errors.New("base commit not found")
	}

	printStatNum(numStatMap)
	return nil
}

func printStatNum(numStatMap map[string]*numStat) {
	keys := []string{}
	for key := range numStatMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Println(numStatMap[key].String())
	}
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

func usageGitReset() {
	fmt.Printf("Usage: %s commit-id\n", os.Args[0])
	os.Exit(-1)
}

func usageGitDiffNumstat() {
	fmt.Printf("Usage: %s base-commit-id commit-id\n", os.Args[0])
	os.Exit(-1)
}

func usage() {
	fmt.Printf("Must have a name \"git-reset\" or \"git-diff-numstat\".\n")
	os.Exit(-1)
}

func gitReset(token string) error {
	if len(os.Args) != 2 {
		usageGitReset()
		/*NOTREACHED*/
	}

	commitID := os.Args[1]
	info, err := getRepoInfo()
	if err != nil {
		return err
	}

	// 1st, try to use git command. quit if no error
	if err = gitResetHardCmd(commitID); err == nil {
		return nil
	}

	if info.saas != "github.com" {
		return errors.New(fmt.Sprintf("%s is not supported", info.saas))
	}

	if err = removeTrackingFiles(); err != nil {
		return err
	}

	ctx := context.Background()
	client := newGithubClient(ctx, token)
	call := newGitCall(ctx, client, info, token)

	return call.gitReset(commitID)
}

func gitDiffNumstat(token string) error {
	if len(os.Args) != 3 {
		usageGitDiffNumstat()
		/*NOTREACHED*/
	}

	baseCommitID := os.Args[1]
	commitID := os.Args[2]
	info, err := getRepoInfo()
	if err != nil {
		return err
	}

	// 1st, try to use git command. quit if no error
	if err = gitDiffNumstatCmd(baseCommitID, commitID); err == nil {
		return nil
	}

	ctx := context.Background()
	client := newGithubClient(ctx, token)
	call := newGitCall(ctx, client, info, token)

	return call.gitDiffNumstat(baseCommitID, commitID)
}

func main() {
	token, ok := os.LookupEnv("AUTH_TOKEN")
	if !ok {
		panic(errors.New("AUTH_TOKEN must be defined"))
	}

	var err error
	switch filepath.Base(os.Args[0]) {
	case "git-reset":
		err = gitReset(token)
	case "git-diff-numstat":
		err = gitDiffNumstat(token)
	default:
		usage()
		/*NOTREACHED*/
	}
	if err != nil {
		fmt.Printf("\nerror: %+v\n", err)
	}
}
