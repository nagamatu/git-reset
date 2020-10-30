package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"

	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"
)

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

func (c *gitCall) addNumstatForCommit(p *github.Commit, numStatMap *map[string]*numStat) (*github.RepositoryCommit, error) {
retry:
	commit, resp, err := c.client.Repositories.GetCommit(c.ctx, c.info.owner, c.info.repo, p.GetSHA())
	if err != nil {
		if isRateLimitThenWait(resp, err) {
			goto retry
		}
		return nil, errors.WithStack(err)
	}

	for _, commitFile := range commit.Files {
		numStat := (*numStatMap)[commitFile.GetFilename()]
		if numStat == nil {
			(*numStatMap)[commitFile.GetFilename()] = newNumStat(commitFile)
		} else {
			(*numStatMap)[commitFile.GetFilename()] = numStat.Add(commitFile)
		}
	}
	return commit, nil
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
			commit, err := c.addNumstatForCommit(pc, &numStatMap)
			if err != nil {
				return errors.WithStack(err)
			}

			checkedSHA[commitID] = true

			for _, p := range commit.Parents {
				if p.GetSHA() == baseCommitID {
					if _, err := c.addNumstatForCommit(p, &numStatMap); err != nil {
						return errors.WithStack(err)
					}
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

func gitDiffNumstatCmd(baseCommitID, commitID string) error {
	cmd := exec.Command("git", "diff", "-c", "--numstat", baseCommitID, commitID)
	out, err := cmd.Output()
	if err != nil {
		return errors.WithStack(err)
	}
	fmt.Printf(string(out))
	return nil
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

func usageGitDiffNumstat() {
	fmt.Fprintf(os.Stderr, "Usage: %s base-commit-id commit-id\n", os.Args[0])
	os.Exit(-1)
}
