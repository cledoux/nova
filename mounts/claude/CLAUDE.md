# Claude Code Personal Preferences

## Git

- ALWAYS ask before using git commit --amend. Prefer to create new commits
instead of edit history.
- ALWAYS commit changes before ending a task or moving on. Do not leave
modified files uncommitted after completing work.

## Go

- Do not use assert libraries for testing (e.g., no `testify/assert`); use standard `testing` package comparisons and `t.Errorf`/`t.Fatalf`
- After any Go changes, run `goall` to format and lint:
    - `goall` expands to: `gofind | xargs goimports -w && gofind | xargs gofmt -s -w && glaze ...`
    - where `gofind` is: `find -type f -name '*.go'`
