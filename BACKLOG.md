# Nova Backlog

## Current

- CLEANUP: **Roll back the swarm features**. We were too ambitious by
  trying to get swarming working right off the bat. Roll back the swarm
  features so that we can focus on getting a single agent workflow
  working well.

## Pending

- BUG: **Use existing nova channel**. Everytime I reset the database,
  nova creates a new #nova channel in the server instead of using the
  channel named #nova that already exists. I want to just use the
  existing one.

- FEATURE: **Reply to @ mentions in-channel**. When Nova is @-mentioned in
  any channel, reply directly to that message in that channel rather
  than always routing responses through the control channel.

- FEATURE: **Status line via Discord**. Expose Claude's status line
  information (tool use, thinking state, etc.) to Discord users.
  Consider a `/status` slash command or a persistent status embed that
  updates in real time.


## Done

- FEATURE: **Working local setup in docker image**. Get a local claude
  instance working from the server. As a human user sitting in front of
  the terminal, I should be able to interact with claude running in yolo
  mode inside the docker container.
