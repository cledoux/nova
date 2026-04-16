# Nova Backlog

## Current

_(nothing — see Pending for next items)_

## Pending

- FEATURE: **Live thinking stream**. Surface Claude's extended thinking
  in Discord as it happens — not just the final response. Options: stream
  thinking text into an ephemeral or thread message that updates in real
  time, or post thinking blocks as a collapsible thread reply before the
  final answer. Requires enabling extended thinking in the Claude session
  and plumbing the thinking deltas through the stream parser.

- FEATURE: **Acknowledgement indicator**. When the bot receives a message
  and begins processing, give the user immediate feedback that the request
  was received. Options: Discord typing indicator (`ChannelTyping`), a
  reaction on the message (e.g. 🔄), or a short "thinking…" reply that
  is edited/deleted once the real response arrives.

- FEATURE: **Status line via Discord**. Expose Claude's status line
  information (tool use, thinking state, etc.) to Discord users.
  Consider a `/status` slash command or a persistent status embed that
  updates in real time.


## Done

- FEATURE: **Boot-time orientation prompt**. On fresh session spawn, Nova
  sends Claude an initial message instructing it to read the git log in
  `cfg.RepoPath` and orient itself. Claude replies with `{"type":"done"}`
  so nothing is posted to Discord — orientation is silent.

- CLEANUP: **Roll back the swarm features**. Removed swarm package,
  multi-agent orchestration, Discord categories, broadcast commands,
  and swarm DB tables. Single-agent workflow only.

- BUG: **Use existing nova channel**. Fixed EnsureChannel to match by
  name regardless of category when categoryID is empty, so #nova is
  reused across DB resets instead of duplicated.

- FEATURE: **Reply to @ mentions in-channel**. When Nova is @-mentioned
  in any channel, a session is spawned (or revived) bound to that channel
  so replies go directly there instead of routing through #nova.

- FEATURE: **Working local setup in docker image**. Get a local claude
  instance working from the server. As a human user sitting in front of
  the terminal, I should be able to interact with claude running in yolo
  mode inside the docker container.
