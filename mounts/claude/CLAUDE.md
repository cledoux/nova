# Claude Code Personal Preferences

## Git

- ALWAYS ask before using git commit --amend. Prefer to create new commits
instead of edit history.
- Commit messages are the primary record of historical context and progress.
  Record the why as well as the what — key decisions, rejected alternatives,
  and non-obvious constraints are all good examples of the why.
- Workflow lessons and operational knowledge (e.g. how to restart a service,
  tool behavior quirks) belong in CLAUDE.md, not in commit messages. Commit
  messages should be about the specific change, not general process notes.
- At the start of a session, use `git log` to reconstruct context before
  asking the user to re-explain history.
- Bundle documentation updates (README, CLAUDE.md, etc.) in the same commit
  as the code change they describe — not in a separate follow-up commit.

## Go

- Do not use assert libraries for testing (e.g., no `testify/assert`); use standard `testing` package comparisons and `t.Errorf`/`t.Fatalf`
- After any Go changes, run `goall` to format and lint:
    - `goall` expands to: `gofind | xargs goimports -w && gofind | xargs gofmt -s -w && glaze ...`
    - where `gofind` is: `find -type f -name '*.go'`
