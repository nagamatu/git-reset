package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	//"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"
)

func (c *gitCall) gitGetPRMergeCommit(id int) error {
retry:
	pr, resp, err := c.client.PullRequests.Get(c.ctx, c.info.owner, c.info.repo, id)
	if err != nil {
		if isRateLimitThenWait(resp, err) {
			goto retry
		}
		return errors.WithStack(err)
	}
	fmt.Printf("%s\n", pr.GetMergeCommitSHA())

	return nil
}

func gitGetPRMergeCommit(token string) error {
	if len(os.Args) != 2 {
		usageGitGetPRMergeCommit()
		/*NOTREACHED*/
	}

	info, err := getRepoInfo()
	if err != nil {
		return err
	}

	idStr := os.Args[1]
	ctx := context.Background()
	client := newGithubClient(ctx, token)
	call := newGitCall(ctx, client, info, token)

	id, err := strconv.Atoi(idStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid PR ID: %v\n", err)
		os.Exit(-1)
	}

	return call.gitGetPRMergeCommit(id)
}

func usageGitGetPRMergeCommit() {
	fmt.Fprintf(os.Stderr, "Usage: %s pull-request-id\n", os.Args[0])
	os.Exit(-1)
}
