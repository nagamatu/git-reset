package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pkg/errors"
)

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
		s, err := os.Stat(path)
		if !os.IsNotExist(err) {
			if s.IsDir() {
				if err := os.RemoveAll(path); err != nil {
					return errors.WithStack(err)
				}
			}
		}
		if err = ioutil.WriteFile(path, b64d, 0644); err != nil {
			return errors.WithStack(err)
		}
	default:
		return errors.New(fmt.Sprintf("%s: unsupported encoding: %s\n", path, encoding))
	}
	return nil
}

func (c *gitCall) gitReset(commitID string) error {
retry:
	tree, resp, err := c.client.Git.GetTree(c.ctx, c.info.owner, c.info.repo, commitID, true)
	if err != nil {
		if isRateLimitThenWait(resp, err) {
			goto retry
		}
		return errors.WithStack(err)
	}

	for _, entry := range tree.Entries {
		if entry.GetType() == "tree" {
			continue
		}
	retryGetBlob:
		blob, resp, err := c.client.Git.GetBlob(c.ctx, c.info.owner, c.info.repo, entry.GetSHA())
		if err != nil {
			if isRateLimitThenWait(resp, err) {
				goto retryGetBlob
			}
			return errors.WithStack(err)
		}
		if err = fileUpdate(entry.GetPath(), blob.GetContent(), blob.GetEncoding()); err != nil {
			return errors.WithStack(err)
		}
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

func gitFetchCheckoutCmd(commitID string) error {
	cmd := exec.Command("git", "fetch", "origin", commitID)
	_, err := cmd.Output()
	if err != nil {
		return errors.WithStack(err)
	}
	cmd = exec.Command("git", "checkout", commitID)
	_, err = cmd.Output()
	if err != nil {
		return errors.WithStack(err)
	}
	return nil

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

	if err = gitFetchCheckoutCmd(commitID); err == nil {
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

func usageGitReset() {
	fmt.Fprintf(os.Stderr, "Usage: %s commit-id\n", os.Args[0])
	os.Exit(-1)
}
