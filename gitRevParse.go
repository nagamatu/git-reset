package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/pkg/errors"
)

func (c *gitCall) gitRevParse(commitID string) error {
retry:
	commit, resp, err := c.client.Repositories.GetCommit(c.ctx, c.info.owner, c.info.repo, commitID)
	if err != nil {
		if isRateLimitThenWait(resp, err) {
			goto retry
		}
		return errors.WithStack(err)
	}
	fmt.Printf("%s\n", commit.GetSHA())
	return nil
}

func gitRevParseCmd(commitID string) error {
	cmd := exec.Command("git", "rev-parse", commitID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.WithStack(err)
	}
	fmt.Printf(string(out))
	return nil
}

func gitRevParse(token string) error {
	if len(os.Args) != 2 {
		usageGitRevParse()
		/*NOTREACHED*/
	}

	commitID := os.Args[1]
	info, err := getRepoInfo()
	if err != nil {
		return err
	}

	// 1st, try to use git command. quit if no error
	if err = gitRevParseCmd(commitID); err == nil {
		return nil
	}

	ctx := context.Background()
	client := newGithubClient(ctx, token)
	call := newGitCall(ctx, client, info, token)

	return call.gitRevParse(commitID)
}

func usageGitRevParse() {
	fmt.Fprintf(os.Stderr, "Usage: %s commit-id\n", os.Args[0])
	os.Exit(-1)
}
