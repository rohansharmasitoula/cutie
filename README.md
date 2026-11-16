# cutie

A terminal control center for your GitHub PRs, the omp agents working on them, and your Claude usage. From any PR it starts an omp agent in a herdr pane, shows what that agent is doing and what it has cost, and can watch PRs to wake agents on new activity.

## Run

```sh
go install github.com/rohansharmasitoula/cutie@latest   # installs into $(go env GOPATH)/bin
cutie                                                   # from any folder
```

From a clone: `go build -o ~/.local/bin/cutie .` (also how to rebuild after changes).

Run it in a herdr pane, or in its own terminal window next to one running `herdr`.

Needs `gh` (logged in), `omp` and a running herdr server.

| Env | Default | Meaning |
|---|---|---|
| `CUTIE_ROOT` | `~/Development` | Folder holding one clone per repo, named after the repo |
| `CUTIE_INTERVAL` | `60` | GitHub refresh, in seconds (minimum 10) |
| `CUTIE_GUARD` | `85` | Pause watchers when the Claude 5 hour window passes this percentage |
| `CUTIE_WATCH_MINUTES` | `30` | Minutes between watcher checks (the list itself still refreshes every `CUTIE_INTERVAL`) |

## Layout

| Pane | Shows |
|---|---|
| Header | Claude 5 hour and 7 day windows with reset countdowns, 24h spend, working and blocked agents |
| PRs (left) | Tabs **Mine** and **Review**, grouped by repo; stacked PRs indented under their parent |
| Detail (top right) | Selected PR: branch, status, checks (failing ones named), reviewers, unresolved threads with file:line and the comment, and its agent with cost and live output |
| Agents | Every agent herdr sees, blocked first, with session cost (subagents included) |
| Claude · 24h | Plan windows, spend and tokens, hourly token sparkline, model split, costliest folders |
| Activity | New feedback, pushes, CI changes, approvals, merges, agents finishing or waiting for you, watchers firing |

Data sources: GitHub GraphQL through `gh` (a 1-point list poll each refresh; full details only for PRs whose `updatedAt` moved or whose CI is still running; polling pauses below 300 points left until the hourly reset, shown as `gh N left` in the header), `herdr agent list`/`agent read`, `omp usage --json` (plan windows, every minute), `omp stats --json` (24h, every 5 minutes), and the agents' omp session files for per-agent cost.

## PR status

**Mine**: `needs work` (an unresolved thread where you were not the last to reply, or human feedback newer than your last push; threads you replied to show as `N replied` and wait on the reviewer), `approved`, `in review`, `draft`.
**Review** (`review-requested:@me` plus open PRs you reviewed): `to review`, `new commits` (head moved since your review), `reviewed`.

Row columns: watch `◉`, status, CI, number and title, agent state and cost, open threads, author (review tab), time since the last push (yellow after 1 day, red after 3).

A PR is shown as stacked when the same author's PR branch is its base; shared branches (`main`, `staging`, `development`, …) never count.

## Keys

| Key | Action |
|---|---|
| `↑` `↓` / `j` `k` / wheel / click | Move (`pgup` `pgdn` `home` `end` too) |
| `enter` / click the selected row | PR: jump to its agent, or start one (`needs work`, `to review`, `new commits`). Repo: fold it |
| `p` | Type a prompt for the selected PR's agent |
| `x` `x` | Close the selected PR's agent pane, ending its omp session. The first `x` asks (and warns if the agent is working); any other key keeps it |
| `w` | Watch or unwatch the PR |
| `o` | Open the PR in the browser |
| `tab` / `1` / `2` | Mine / Review |
| `/` | Filter by text (`esc` clears) |
| `s` | Sort: urgent → updated → waiting |
| `g` | Group by repo on or off |
| `space` | Fold or unfold the repo |
| `b` / `d` | Hide or show bot PRs (hidden by default) / drafts |
| `r` | Refresh now |
| `R` | Force refresh: refresh everything and run the watchers now instead of waiting for the next watcher check (the spending guard still applies) |
| Drag a panel border | Resize: list width, detail height, agents/Claude split, activity height. Saved in `sizes.json` next to `watches.json` |
| `=` | Reset panel sizes |
| `?` | Help |
| `q` | Quit |

## Agents

Starting an agent for `owner/repo#N`:

1. Finds the herdr workspace labelled `repo`, or creates it at `$CUTIE_ROOT/repo`.
2. Finds the tab labelled `fix #N` (your PR) or `review #N` (someone else's), and a pane in it without an agent, or creates the tab.
3. Runs `herdr agent start fix-repo-N --kind omp` (`rev-repo-N` for reviews), then prompts it to spawn an `orchestrator` or `reviewer` task agent. The prompts forbid pushing and posting to GitHub.

The agent name ties the pane to the PR.

## Watchers and notifications

`w` saves the PR's current state as a baseline in `~/Library/Application Support/cutie/watches.json`. Every `CUTIE_WATCH_MINUTES` (30 by default), a watched PR with newer activity (new reviewer feedback on yours, new commits on one you review) gets its agent prompted to follow up, or a new agent started; feedback that piles up in between wakes the agent once. A watcher waits while its agent is working or blocked, and all watchers pause while the 5 hour Claude window is above `CUTIE_GUARD`. Watches on merged or closed PRs are dropped. Watching only runs while cutie is open.

herdr notifications (with sound) fire when any agent starts waiting for you and when a watched PR of yours gets new feedback.
