# Nova Backlog

## Current

_(nothing — see Pending for next items)_

## Pending

- FEATURE: **Status line via Discord**. Expose Claude's status line
  information (tool use, thinking state, etc.) to Discord users.
  Consider a `/status` slash command or a persistent status embed that
  updates in real time.


## Done

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
