package main

import (
	"context"
	"fmt"
	"os"

	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"
)

func (c *gitCall) gitCreateReference(branch, commitID string) error {
	r := fmt.Sprintf("refs/heads/%s", branch)
	ref := &github.Reference{Ref: &r, Object: &github.GitObject{SHA: &commitID}}
retry:
	ref, resp, err := c.client.Git.CreateRef(c.ctx, c.info.owner, c.info.repo, ref)
	if err != nil {
		if isRateLimitThenWait(resp, err) {
			goto retry
		}
		fmt.Printf("%v\n", resp)
		return errors.WithStack(err)
	}

	fmt.Printf("%v\n", ref)
	return nil
}

func gitCreateReference(token string) error {
	if len(os.Args) != 3 {
		usageGitCreateReference()
		/*NOTREACHED*/
	}

	branch := os.Args[1]
	commitID := os.Args[2]
	info, err := getRepoInfo()
	if err != nil {
		return err
	}

	ctx := context.Background()
	client := newGithubClient(ctx, token)
	call := newGitCall(ctx, client, info, token)

	return call.gitCreateReference(branch, commitID)
}

func usageGitCreateReference() {
	fmt.Printf("Usage: %s branch commit-id\n", os.Args[0])
	os.Exit(-1)
}
