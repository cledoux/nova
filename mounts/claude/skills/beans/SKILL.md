---
name: beans
description: Work management with the Beans issue tracker. Use instead of TodoWrite for all task tracking. Auto-load when planning work, breaking down features, recording discovered work, or checking what to work on next. Covers creating beans, querying via GraphQL, updating progress, and committing bean files alongside code.
allowed-tools: Bash(beans *)
---

# Beans Work Management

This project uses **beans** — a flat-file Markdown issue tracker — for all work management.
Run `beans prime` for the full agent usage guide.

**Never use TodoWrite. Use beans for all task and work tracking.**

## When Planning Work

When the user asks to plan, break work down, or structure a feature:

1. Create a milestone or epic for the overall goal
2. Create feature/task beans as children
3. Set blocking relationships where order matters
4. Use `draft` for items needing refinement; `todo` for ready work

```bash
beans create "Epic title" -t epic -d "Description" -s todo
beans create "Task title" -t task -d "Description" -s todo --parent <epic-id>
```

## When Discovering Work

When you spot a bug, follow-up, refactor need, or deferred idea during a task:

- **Create a bean immediately** — don't let it get lost
- `todo` for actionable items; `draft` if it needs more thought first
- Link to the current bean if relevant (`--blocked-by` or `--blocking`)
- Mention it to the user so they know it was recorded

## Before Starting Any Task

1. Check for an existing bean: `beans list --json --ready`
2. If none, create one and set it `in-progress`
3. Check off todo items as you go: `- [ ]` → `- [x]`
4. When done: status `completed`, append `## Summary of Changes`
5. Commit `.beans/` files in the same commit as the code

## Querying (GraphQL preferred)

```bash
# What's ready to work on?
beans query --json '{ beans(filter: { excludeStatus: ["completed","scrapped","draft"], isBlocked: false }) { id title type status priority } }'

# What's in flight?
beans list --json -s in-progress

# Full view of a bean with relationships
beans query --json '{ bean(id: "<id>") { title body status children { id title status } blockedBy { id title } } }'

# Search by keyword
beans query --json '{ beans(filter: { search: "keyword" }) { id title status } }'
```

Use `beans query` (GraphQL) over `beans list` whenever you need relationships or precise
field selection. It's the same subprocess — no server, no overhead.

## Committing

Always include `.beans/` files in the same commit as the code they track. Bean files
are plain Markdown — they diff cleanly and belong in git history alongside the work.
