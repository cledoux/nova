# Beans — Setup & Usage Notes

Issue tracker for Nova: flat Markdown files, pure Go, no CGO, agent-native.
Repo: https://github.com/hmans/beans — v0.4.2 as of April 2026.

## Installation

**Local:** binary at `/home/agent/.local/bin/beans` (already installed).

**Docker:** Dockerfile fetches latest release from GitHub API at build time:
```dockerfile
RUN curl -fsSL $(curl -fsSL https://api.github.com/repos/hmans/beans/releases/latest \
        | python3 -c "import sys,json; print(next(a['browser_download_url'] for a in json.load(sys.stdin)['assets'] if 'Linux_x86_64' in a['name']))") \
    | tar -xz -C /home/agent/.local/bin beans \
    && chmod +x /home/agent/.local/bin/beans
```

## Initialisation

Already done — `.beans/` and `.beans.yml` exist in the repo root. Both should be committed to git.

Config is committed to the repo. Prefix is `nova-`.

## Workflow

1. **Before starting work** — create a bean:
   ```sh
   beans create "Title" -t task -d "Description..." -s in-progress
   ```

2. **While working** — check off todo items in the bean body as you go:
   ```sh
   beans update <id> --body-replace-old "- [ ] Step" --body-replace-new "- [x] Step"
   ```

3. **When done** — mark completed and append a summary:
   ```sh
   beans update <id> -s completed --body-append "## Summary of Changes\n\n..."
   ```

4. **Commit bean files alongside code** — `.beans/` is plain Markdown in git.

## Key Commands

```sh
beans list --json --ready          # what's available to start
beans list --json -s in-progress   # what's in flight
beans show <id>                    # full detail on a bean
beans show --json <id>             # machine-readable
beans tui                          # interactive browser
beans prime                        # print full agent usage guide
```

## Issue Types

| Type | Purpose |
|------|---------|
| milestone | Target release or checkpoint |
| epic | Thematic container for related work; has child beans |
| feature | User-facing capability or enhancement |
| task | Concrete piece of work (chore, sub-task) |
| bug | Something broken that needs fixing |

## Statuses

`todo` · `in-progress` · `draft` · `completed` · `scrapped`

## Priorities

`critical` · `high` · `normal` · `low` · `deferred`

## Relationships

```sh
beans update <id> --parent <other-id>       # hierarchy: epic → task
beans update <id> --blocking <other-id>     # this blocks another
beans update <id> --blocked-by <other-id>   # this is blocked by another
```

## GraphQL

Beans exposes a GraphQL API for precise agent queries:

```sh
beans query --schema   # view full schema
beans query --json '{ beans(filter: { excludeStatus: ["completed","scrapped"], isBlocked: false }) { id title status type } }'
beans query --json '{ bean(id: "<id>") { title body parent { title } children { id title status } } }'
```

## Agent Access Pattern

**Decision: GraphQL exclusively** (`beans graphql '<query>'`).

Rationale: single call can traverse relationships (parent, children, blockedBy, blocking);
precise field selection keeps token overhead low; full-text search built in; mutations
available through the same interface. Crucially, `beans graphql` is not a separate server —
it's the same CLI subprocess with a richer query language. No port, no lifecycle to manage.

Use the CLI (`beans create`, `beans update`) only for writes where the GraphQL mutation
syntax would be more cumbersome than the flag-based equivalent.

## What's Next

- [ ] Create initial beans for backlog items in `BACKLOG.md`
