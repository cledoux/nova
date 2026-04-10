# Nova CLI Spike: Claude Subprocess Management

**Date:** 2026-04-09

## Summary

All required flags exist. Nova can implement hot/cold sessions using a persistent subprocess per active session, with resume capability for cold sessions.

---

## Flags

### Session Resume

```
-r, --resume <session_id>
```

Resumes a prior conversation by UUID. The session is persisted to `~/.claude/projects/<encoded-cwd>/` as a `.jsonl` file. Resuming loads the full history.

### System Prompt Injection

Two options:
- `--system-prompt <prompt>` — replaces the default system prompt entirely
- `--append-system-prompt <prompt>` — appends to the default system prompt

Nova should use `--system-prompt` to inject the agent's role/instructions on first spawn.

### Pre-assign Session ID

```
--session-id <uuid>
```

Nova can generate a UUID before spawning and pass it here. The resulting session will use exactly that UUID, eliminating the need to parse session_id from output. **This is the recommended approach.**

Verified: `--session-id <preset>` → output `session_id` matches preset exactly.

### Non-interactive Print Mode

```
-p, --print
```

Required for subprocess use. Without `--print`, claude opens an interactive TUI that requires a TTY.

### Stream JSON I/O (hot session protocol)

```
--input-format=stream-json --output-format=stream-json --verbose
```

Must be used together with `--print`. Enables a persistent subprocess that:
- Accepts multiple turns as newline-delimited JSON on stdin
- Emits line-by-line JSON events on stdout
- Acts as the "hot" session subprocess

---

## Hot Session Protocol

### Subprocess invocation

```bash
# First spawn (new session)
claude --print \
  --input-format=stream-json \
  --output-format=stream-json \
  --verbose \
  --session-id <preset_uuid> \
  --system-prompt "You are a Nova worker agent. ..."

# Resume (cold → hot)
claude --print \
  --input-format=stream-json \
  --output-format=stream-json \
  --verbose \
  --resume <session_id>
```

### Sending a message (stdin JSON line)

```json
{"type":"user","message":{"role":"user","content":"<user message here>"}}
```

### Reading responses (stdout JSON lines)

Relevant event types:
- `type=assistant` — partial or complete assistant message content
- `type=result` — signals end of turn; contains `result` (final text) and `session_id`
- `type=system, subtype=init` — emitted at startup; contains `session_id`
- `type=rate_limit_event` — informational

**Done sentinel:** a line with `"type":"result"` marks the end of one turn's response.

**Directive interception:** Nova reads stdout line-by-line. Lines containing a valid JSON directive object (from Claude's `content`) are intercepted before being posted to Discord.

---

## Session ID Capture

**Recommended:** Pre-assign with `--session-id <uuid>`. Nova generates the UUID before spawning, stores it in DB immediately, no parsing required.

**Alternative (if needed):** Parse the `"type":"system","subtype":"init"` event from stream-json output; it contains `"session_id"`.

---

## Session Storage

Sessions are stored at:
```
~/.claude/projects/<url-encoded-working-dir>/<session_id>.jsonl
```

The working directory is encoded by replacing `/` with `-`. For example:
```
~/.claude/projects/-home-cledoux-workspace-discord/
```

Nova workers should run with a consistent working directory (their workspace dir) so sessions are grouped under the right project.

---

## Fallback (if flags change)

If `--resume` is removed in a future version:
- Use `--continue` to resume the most recent session in cwd (less precise)
- Use `--session-id` with the same UUID (session file still exists on disk)

If stream-json input is removed:
- Fall back to spawning a new `claude --print --resume <id>` process per message turn
- This is less efficient but functionally equivalent

---

## Implementation Notes for Task 5 (Session Subprocess)

1. **Spawn:** `exec.Command("claude", "--print", "--input-format=stream-json", "--output-format=stream-json", "--verbose", "--session-id", id, "--system-prompt", prompt)` with `cmd.Stdin = stdinPipe`, `cmd.Stdout = stdoutPipe`
2. **Send message:** write `{"type":"user","message":{"role":"user","content":"<msg>"}}\n` to stdin pipe
3. **Read response:** read stdout lines; accumulate `assistant` message content; detect `result` type as done sentinel; intercept directives
4. **Terminate (hot → cold):** close stdin pipe and wait for process to exit; session persists on disk automatically
5. **Resume (cold → hot):** same spawn command but with `--resume <id>` instead of `--session-id <id>` + `--system-prompt`
