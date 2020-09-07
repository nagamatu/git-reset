package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"
)

func (c *gitCall) gitLog(commitID string) error {
	fmt.Printf("%s\n", commitID)
	checkedSHA := make(map[string]bool)
	count := 0
	parents := []*github.Commit{}
L:
	for parents = append(parents, &github.Commit{SHA: &commitID}); len(parents) > 0 && count < 20; {
		nextParents := []*github.Commit{}
		for _, pc := range parents {
			commit, _, err := c.client.Repositories.GetCommit(c.ctx, c.info.owner, c.info.repo, pc.GetSHA())
			if err != nil {
				return errors.WithStack(err)
			}

			checkedSHA[pc.GetSHA()] = true

			for _, p := range commit.Parents {
				if checkedSHA[p.GetSHA()] {
					continue
				}
				checkedSHA[p.GetSHA()] = true
				nextParents = append(nextParents, p)
				fmt.Printf("%s\n", p.GetSHA())
				count++
				if count >= 100 {
					break L
				}
			}
		}
		parents = nextParents
	}

	return nil
}

func gitLogCmd(commitID string) error {
	cmd := exec.Command("git", "log", "--format=%H", "-100", commitID)
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
	fmt.Printf("Usage: %s commit-id\n", os.Args[0])
	os.Exit(-1)
}
