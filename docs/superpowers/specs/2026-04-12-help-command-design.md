# Help Command Design

**Date:** 2026-04-12

## Summary

Add `/nova help` as a new subcommand that responds with a static ephemeral message listing all available `/nova` subcommands and their descriptions.

## Approach

Static hardcoded string (Option A). The command list is small and stable; runtime derivation would add complexity without benefit.

## Changes

All changes are confined to `bot/commands/commands.go`:

1. **Command definition** — add a `help` subcommand entry to the `Options` slice in `novaCommand()`:
   ```go
   {Type: sub, Name: "help", Description: "Show available commands"},
   ```

2. **Router** — add a `case "help"` branch in `onInteraction`'s switch:
   ```go
   case "help":
       h.handleHelp(s, i)
   ```

3. **Handler** — add `handleHelp` that calls `respondEphemeral` with a formatted code block listing every subcommand and a one-liner description.

## Help Text Content

The response will cover all current subcommands:

| Command | Description |
|---|---|
| `/nova spawn` | Spawn a new Claude session |
| `/nova list` | List active sessions |
| `/nova kill` | Terminate a session |
| `/nova resume` | Force-warm a cold session |
| `/nova status` | Show session status |
| `/nova clean` | Delete workspaces of terminated sessions |
| `/nova broadcast` | Send a message to all sessions in a swarm |
| `/nova swarm create` | Create a swarm |
| `/nova swarm dissolve` | Dissolve a swarm |
| `/nova help` | Show available commands |

## Response Format

Ephemeral Discord message, visible only to the invoking user. Uses a Discord code block for alignment.

## Testing

No unit test needed — there is no logic to test, only a string constant. Manual verification: run `/nova help` in Discord and confirm the message appears and is correct.
