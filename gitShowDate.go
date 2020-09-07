package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/pkg/errors"
)

func gitShowDateCmd(commitID string) error {
	cmd := exec.Command("git", "show", "--format=%ai", commitID)
	out, err := cmd.Output()
	if err != nil {
		return errors.WithStack(err)
	}
	fmt.Printf(string(out))
	return nil
}

func (c *gitCall) gitShowDate(commitID string) error {
	commit, _, err := c.client.Git.GetCommit(c.ctx, c.info.owner, c.info.repo, commitID)
	if err != nil {
		return errors.WithStack(err)
	}
	if commit.Author == nil {
		return errors.New("author not found in commit")
	}

	author := commit.GetAuthor()
	fmt.Printf("%s\n", author.GetDate())

	return nil
}

func gitShowDate(token string) error {
	if len(os.Args) != 2 {
		usageGitShowDate()
		/*NOTREACHED*/
	}

	commitID := os.Args[1]
	info, err := getRepoInfo()
	if err != nil {
		return err
	}

	// 1st, try to use git command. quit if no error
	if err = gitShowDateCmd(commitID); err == nil {
		return nil
	}

	ctx := context.Background()
	client := newGithubClient(ctx, token)
	call := newGitCall(ctx, client, info, token)

	return call.gitShowDate(commitID)
}

func usageGitShowDate() {
	fmt.Printf("Usage: %s commit-id\n", os.Args[0])
	os.Exit(-1)
}
