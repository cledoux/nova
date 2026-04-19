# Claude Code Personal Preferences

## User Profile

- **Name**: Charles
- **Strong in**: Go, Python, Bash/shell
- **Weak in**: Frontend (HTML/CSS/JS, UI frameworks)
- **Communication**: Professional and concise. Humor, sarcasm, and emojis are fine in moderation.
- **Context**: Side projects.
- **Workflow**: Dive in without asking for plan approval. Ask only when there are genuine tradeoffs that aren't clear-cut.
- **Testing**: Follow standard best practices for the language/framework in use.

## Work Management

Use **beans** (`~/.local/bin/beans`) for all work tracking — never TodoWrite.

- **When asked to plan work**: break it down into beans (milestone/epic/feature/task hierarchy), set blocking relationships, commit `.beans/` files alongside code.
- **When work is discovered** (bugs, follow-ups, deferred ideas found during a task): create a bean immediately so nothing gets lost, then tell the user it was recorded.

Run `beans prime` for the full agent usage guide. The `/beans` skill has workflow details.


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
