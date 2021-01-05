package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/google/go-github/v33/github"
	"github.com/pkg/errors"
)

func (c *gitCall) gitLog(commitID string) error {
	checkedSHA := make(map[string]bool)
	count := 0
	commits := []*github.Commit{}
	for commits = append(commits, &github.Commit{SHA: &commitID}); len(commits) > 0 && count < 1; count++ {
		parents := []*github.Commit{}
		for _, pc := range commits {
		retry:
			commit, resp, err := c.client.Repositories.GetCommit(c.ctx, c.info.owner, c.info.repo, pc.GetSHA())
			if err != nil {
				if isRateLimitThenWait(resp, err) {
					goto retry
				}
				return errors.WithStack(err)
			}

			fmt.Printf("%s ", pc.GetSHA())
			for _, p := range commit.Parents {
				fmt.Printf("%s ", p.GetSHA())
			}
			fmt.Printf("\n")

			checkedSHA[pc.GetSHA()] = true

			for _, p := range commit.Parents {
				if checkedSHA[p.GetSHA()] {
					continue
				}
				checkedSHA[p.GetSHA()] = true
				parents = append(parents, p)
			}
		}
		commits = parents
	}

	return nil
}

func gitLogCmd(commitID string) error {
	cmd := exec.Command("git", "log", "--format=%H %P", "-1", commitID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.WithStack(err)
	}
	fmt.Printf(string(out))
	return nil
}

func gitLog(token string) error {
	if len(os.Args) != 2 {
		usageGitLog()
		/*NOTREACHED*/
	}

	commitID := os.Args[1]
	info, err := getRepoInfo()
	if err != nil {
		return err
	}

	// 1st, try to use git command. quit if no error
	if err = gitLogCmd(commitID); err == nil {
		return nil
	}

	ctx := context.Background()
	client := newGithubClient(ctx, token)
	call := newGitCall(ctx, client, info, token)

	return call.gitLog(commitID)
}

func usageGitLog() {
	fmt.Fprintf(os.Stderr, "Usage: %s commit-id\n", os.Args[0])
	os.Exit(-1)
}
