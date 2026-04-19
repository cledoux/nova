---
name: create-skill
description: Create a new Claude Code skill (SKILL.md). Use when the user wants to teach Claude a new reusable workflow, checklist, or procedure, or says "create a skill", "write a skill", or "make a /command".
argument-hint: [skill-name] [brief description]
---

You are creating a new Claude Code skill. A skill is a `SKILL.md` file that adds a reusable workflow or playbook to Claude's toolkit, invocable as `/skill-name`.

## Step 1 — Clarify intent

If `$ARGUMENTS` is empty or vague, ask the user:
- What should the skill be named? (lowercase, hyphens only, max 64 chars)
- What does it do in one sentence?
- Should users invoke it manually, or should Claude load it automatically when relevant?
- Does it have side effects (deploy, commit, send message)? If so, `disable-model-invocation: true` is appropriate.
- Personal skill (`~/.claude/skills/`) or project skill (`.claude/skills/`)?

If `$ARGUMENTS` gives enough context, proceed without asking.

## Step 2 — Choose the right location

| Scope    | Path                                          |
|----------|-----------------------------------------------|
| Personal | `~/.claude/skills/<skill-name>/SKILL.md`      |
| Project  | `.claude/skills/<skill-name>/SKILL.md`        |

Default to personal unless the skill is tightly coupled to a specific project.

## Step 3 — Write the SKILL.md

Use this structure:

```
---
name: <skill-name>
description: <What it does and when to use it. Front-load the key use case. Be specific — Claude uses this to decide when to auto-invoke.>
# Optional fields — include only what's needed:
# argument-hint: [arg1] [arg2]
# disable-model-invocation: true    # manual-only (deploys, commits, destructive ops)
# user-invocable: false              # background knowledge only, no slash command
# allowed-tools: Bash(git *) Read   # pre-approved tools, no per-use prompt
# context: fork                     # run in isolated subagent
# agent: Explore                    # subagent type when context: fork
---

<Clear instructions. Write as standing guidance, not one-time steps — content stays in context for the whole session.>
```

### Frontmatter rules

- `name`: lowercase letters, numbers, hyphens only, max 64 chars
- `description`: most important field — Claude uses it to decide when to auto-load. Aim for 1–3 keyword-rich sentences, front-loaded.
- `disable-model-invocation: true`: use for anything with side effects so Claude never triggers it automatically.
- `allowed-tools`: space-separated; grants those tools without per-use approval when the skill is active.
- `context: fork`: isolates the skill in a subagent — good for heavy research or tasks that shouldn't see conversation history.

### Content rules

- Keep `SKILL.md` under 500 lines. Move large reference material to sibling files and reference them.
- Use `$ARGUMENTS` to receive user input (`/fix-issue 123` → `$ARGUMENTS` = `"123"`).
- Use `$0`, `$1`, `$ARGUMENTS[N]` for positional args.
- Use `` !`command` `` to inject live shell output before Claude sees the prompt.

## Step 4 — Create the file

1. Run `mkdir -p <skill-directory>` to create the directory.
2. Write the `SKILL.md` file using the Write tool.
3. Confirm the path and explain how to invoke it (`/skill-name` or auto-triggered).

## Step 5 — Offer supporting files

For complex skills, ask whether it needs:
- `examples.md` — sample inputs/outputs
- `reference.md` — detailed docs or API specs
- `scripts/` — helper scripts Claude can run

Reference any supporting files from `SKILL.md` so Claude knows they exist.

## What makes a good description

**Too vague:** "Helps with deployments."
**Good:** "Deploy the application to production. Use when the user runs /deploy or asks to ship, release, or push to prod. Runs tests, builds, and pushes to the deployment target."

**Too vague:** "Explains code."
**Good:** "Explains how code works using analogies and ASCII diagrams. Use when explaining a codebase, teaching a concept, or when the user asks 'how does this work?' or 'walk me through this'."
