# Security policy

## Supported versions

Only the latest release gets security fixes. Update with `go install github.com/rohansharmasitoula/cutie@latest` or download the newest binary from [Releases](https://github.com/rohansharmasitoula/cutie/releases).

## Reporting a vulnerability

Please **don't open a public issue**. Report it privately through GitHub instead: [Security → Report a vulnerability](https://github.com/rohansharmasitoula/cutie/security/advisories/new).

Include what you found, how to reproduce it and what an attacker could do with it. You'll get a reply within a week; once a fix is released, the advisory is published with credit to you unless you'd rather stay anonymous.

## Scope

cutie runs `gh`, herdr and omp on your machine with your own credentials, and sends prompts to coding agents that can commit and, in review autopilot, post to GitHub. Issues worth reporting include anything that lets PR content (titles, comments, branch names) run commands, change what a prompt tells an agent to do, or make an agent post or push when its mode says it shouldn't.
