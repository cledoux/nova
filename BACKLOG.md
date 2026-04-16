# Nova Backlog

## Current

_(nothing — see Pending for next items)_

## Pending

_(nothing — see Done for completed items)_


## Done

- FEATURE: **Thinking thread**. When Claude produces thinking blocks
  (extended thinking), they are posted as a Discord thread reply on the
  answer message. The `assistant` stream-json event is parsed for
  `type:"thinking"` content blocks; blocks are accumulated during the
  turn and passed to `OnContent`, which spawns a goroutine to create the
  thread via `discord.PostThread` after the answer is posted.

- FEATURE: **`/nova stats`**. Shows context window usage, rate limit
  status, and last-turn cost for the session in the current channel.
  Data comes from `rate_limit_event` and `result` stream-json events
  (not the statusline, which doesn't fire in pipe mode). Context window %
  is computed from token counts vs. the model's `contextWindow` field.

- FEATURE: **Resume-check prompt on restart**. When Nova restarts and
  revives a session, it now sends a prompt asking Claude to check git
  log/status for interrupted work and resume if needed, rather than
  coming back online silently.

- FEATURE: **Acknowledgement indicator**. Implemented via Discord's native
  typing indicator (`ChannelTyping`). Fires when a message hits the write
  loop, refreshes every 8s while Claude is working, stops when the response
  is posted. No message clutter.

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
