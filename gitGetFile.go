package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"

	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"
)

func getContent(blob *github.Blob) ([]byte, error) {
	switch blob.GetEncoding() {
	case "base64":
		b64d, err := base64.StdEncoding.DecodeString(blob.GetContent())
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return b64d, nil
	default:
		return nil, errors.New(fmt.Sprintf("unsupported encoding: %s\n", blob.GetEncoding()))
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
			// ignore error - dont care about it
			//fmt.Fprintf(os.Stderr, "can't remove file: %s\n", line)
		}
	}

	return nil
}

func (c *gitCall) gitGetFile(commitID, filepath string) ([]byte, error) {
retry:
	tree, resp, err := c.client.Git.GetTree(c.ctx, c.info.owner, c.info.repo, commitID, true)
	if err != nil {
		if isRateLimitThenWait(resp, err) {
			goto retry
		}
		return nil, errors.WithStack(err)
	}
	for _, entry := range tree.Entries {
		if entry.GetType() == "tree" {
			continue
		}
		if entry.GetPath() == filepath {
		retryGetBlob:
			blob, resp, err := c.client.Git.GetBlob(c.ctx, c.info.owner, c.info.repo, entry.GetSHA())
			if err != nil {
				if isRateLimitThenWait(resp, err) {
					goto retryGetBlob
				}
				return nil, errors.WithStack(err)
			}
			return getContent(blob)
		}
	}
	return nil, errors.New("not found")
}

func gitGetFile(token string) error {
	if len(os.Args) != 3 {
		usageGitGetFile()
		/*NOTREACHED*/
	}

	commitID := os.Args[1]
	filepath := os.Args[2]
	info, err := getRepoInfo()
	if err != nil {
		return err
	}

	ctx := context.Background()
	client := newGithubClient(ctx, token)
	call := newGitCall(ctx, client, info, token)

	content, err := call.gitGetFile(commitID, filepath)
	if err != nil {
		return err
	}
	fmt.Printf(string(content))
	return nil
}

func usageGitGetFile() {
	fmt.Fprintf(os.Stderr, "Usage: %s commit-id filepath\n", os.Args[0])
	os.Exit(-1)
}
