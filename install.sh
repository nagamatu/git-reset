#!/bin/bash
go install
for cmd in git-diff-numstat git-get-file git-log git-merge git-show-date; do
    cmdpath="${GOPATH}/bin/${cmd}"
    if [ -f "${cmdpath}" ]; then
        continue
    fi
    ln -s git-reset "${cmdpath}"
    pwd
done
