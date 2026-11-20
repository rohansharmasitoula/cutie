# cutie

[![CI](https://github.com/rohansharmasitoula/cutie/actions/workflows/ci.yml/badge.svg)](https://github.com/rohansharmasitoula/cutie/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/rohansharmasitoula/cutie.svg)](https://pkg.go.dev/github.com/rohansharmasitoula/cutie)
[![Go version](https://img.shields.io/github/go-mod/go-version/rohansharmasitoula/cutie)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![GitHub stars](https://img.shields.io/github/stars/rohansharmasitoula/cutie)](https://github.com/rohansharmasitoula/cutie/stargazers)
[![Buy Me A Coffee](https://img.shields.io/badge/Buy%20Me%20A%20Coffee-support-orange?logo=buy-me-a-coffee)](https://buymeacoffee.com/sitoularohansharma)

**A terminal dashboard for your pull requests and the AI coding agents working on them.**

cutie lists your open PRs and the PRs waiting for your review, tells you which ones need you, and with one key starts a coding agent on a PR in its own [herdr](https://herdr.dev) pane. It then shows what each agent is doing and what it has cost, and can watch PRs to wake the agent when reviewers reply or authors ask for another review.

![cutie demo](docs/demo.gif)

## Features

| | |
|---|---|
| 📋 **One inbox for PRs and issues** | Your open PRs, the ones waiting for your review and the issues assigned to you, in three tabs grouped by repo. Stacked PRs nest under the PR they build on. |
| 🎯 **Only what needs you** | A PR is flagged when a reviewer is waiting on your reply or commented after your last push. Threads you've already answered stay quiet. |
| ⚡ **An agent in one keystroke** | Press `enter` on a PR or an issue: cutie opens the repo in herdr, starts a coding agent in its own tab and hands it the work. Press it again to jump there. |
| 👀 **See every agent at work** | Each agent's status, cost and latest output, right beside the PR's checks, reviewers and open threads. |
| 🔔 **Watchers** | Watch a PR and its agent wakes up by itself when reviewers reply to yours, or when the author re-requests your review on one you're reviewing. |
| 💸 **Claude usage, always visible** | Your 5 hour and 7 day limits with reset timers, 24h spend, tokens per hour and model split. Watchers pause before you run out. |
| 🪶 **Light on GitHub** | One tiny query a minute; full details only for PRs that changed, so your API budget lasts. |
| 🖱️ **Mouse or keyboard** | Click rows, drag borders to resize panels, filter, sort and fold repos. |

## Requirements

| Tool | Why | Required |
|---|---|---|
| [`gh`](https://cli.github.com), logged in | Reads your PRs | ✅ |
| [herdr](https://herdr.dev), server running | Workspaces, panes and agents | ✅ |
| A coding agent herdr supports (`omp`, `claude`, `codex`, …) | Works on the PRs | ✅ |
| [`omp`](https://github.com/can1357/oh-my-pi) | Claude usage panel and agent costs | Optional |

macOS and Linux. Go 1.24+ to build.

## Install

```sh
go install github.com/rohansharmasitoula/cutie@latest
```

Or from a clone:

```sh
git clone https://github.com/rohansharmasitoula/cutie
cd cutie
go build -o ~/.local/bin/cutie .
```

## Quick start

1. Keep your clones in one folder, each named after its repo, either directly (`~/Development/api`) or one folder down (`~/Development/acme/api`). Point cutie at it with `CUTIE_ROOT` if it isn't `~/Development`.
2. Start herdr in one terminal and `cutie` in another, or run cutie inside a herdr pane.
3. Press `enter` on a PR marked `needs work` or `to review`, or on an issue in the Issues tab.

Try it without GitHub or herdr, on made-up data:

```sh
CUTIE_DEMO=1 cutie
```

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `CUTIE_ROOT` | `~/Development` | Folder holding your clones, directly or one folder down, named after the repo |
| `CUTIE_AGENT` | `omp` | herdr agent kind to start: `omp`, `claude`, `codex`, … |
| `CUTIE_INTERVAL` | `60` | Seconds between GitHub refreshes (minimum 10) |
| `CUTIE_WATCH_MINUTES` | `30` | Minutes between watcher checks |
| `CUTIE_GUARD` | `85` | Pause watchers when the Claude 5 hour window passes this percentage |
| `CUTIE_DEMO` | unset | `1` shows made-up data and calls nothing |

### Prompts

cutie sends an agent one of four messages. The built-in ones work with any agent: they ask it to read the PR with `gh`, fix or review it, commit locally, and **never push or post to GitHub**.

To use your own, put a file with the same name in `~/Library/Application Support/cutie/prompts/` (macOS) or `~/.config/cutie/prompts/` (Linux):

| File | Sent when |
|---|---|
| `fix-start.md` | Starting an agent on your PR |
| `fix-followup.md` | A watcher sees new review feedback on your PR |
| `review-start.md` | Starting an agent on someone else's PR |
| `review-followup.md` | Re-reviewing: a watcher sees a re-request, or the queue reaches a re-requested PR whose agent is still open |
| `issue-start.md` | Starting an agent on an issue assigned to you |

PR prompts are Go templates with `{{.URL}}`, `{{.Repo}}`, `{{.Number}}`, `{{.Branch}}`, `{{.Base}}`, `{{.Author}}` and `{{.HeadShort}}`; the issue prompt has `{{.URL}}`, `{{.Repo}}`, `{{.Number}}`, `{{.Title}}` and `{{.Author}}`. For example, to hand the work to your own subagent:

```text
Spawn an `orchestrator` task agent to address the review feedback on {{.URL}} (branch {{.Branch}}). Commit locally; do not push.
```

The defaults are in [`internal/agents/prompts`](internal/agents/prompts).

## Keys

| Key | Action |
|---|---|
| `↑` `↓` / `j` `k` / wheel / click | Move (`pgup` `pgdn` `home` `end` too) |
| `enter` / click the selected row | Jump to the row's agent, or start one. On a repo row: fold it |
| `p` | Send a prompt to the row's agent |
| `x` `x` | Close the row's agent pane (the first `x` asks) |
| `w` | Watch or unwatch the PR |
| `a` | Add the PR you review to the review queue, or take it out |
| `o` | Open the PR or issue in the browser |
| `tab` / `1` / `2` / `3` | Mine / Review / Issues |
| `/` | Filter (`esc` clears) |
| `s` | Sort: urgent → updated → waiting |
| `g` | Group by repo on or off |
| `space` | Fold or unfold the repo |
| `b` / `d` | Hide or show bot PRs / drafts |
| `r` / `R` | Refresh / refresh and run the watchers now |
| drag a border / `=` | Resize panels / reset sizes |
| `?` | Help |
| `q` | Quit |

## How it works

### PR status

| Tab | Status | Meaning |
|---|---|---|
| Mine | `needs work` | A review thread waits on your reply, or a reviewer wrote after your last push |
| Mine | `in review` / `approved` / `draft` | As on GitHub |
| Review | `to review` | You are requested as a reviewer and haven't reviewed it |
| Review | `re-review` | You reviewed it and the author requested your review again |
| Review | `new commits` | The author pushed after your last review but hasn't re-requested you; nothing to do yet |
| Review | `reviewed` | You reviewed the current head |

`enter`, `a` and the review queue only act on `to review` and `re-review`: a PR you review needs you only while GitHub lists you (or your team) as a requested reviewer.

A PR is drawn as stacked when the same author's PR branch is its base. Shared branches (`main`, `staging`, `development`, …) never count.

### Agents

For `owner/repo#N`, cutie uses the herdr workspace labelled `repo` (created in the repo's clone if missing), a tab labelled `fix #N`, `review #N` or `issue #N`, and an agent named `fix-repo-N`, `rev-repo-N` or `iss-repo-N`. The name ties the agent to the PR or issue, so cutie can show its state and jump back to it. The Issues tab lists open issues assigned to you; they come with the same cheap poll as the PR list.

### Watchers

`w` remembers the PR's current state. Every `CUTIE_WATCH_MINUTES`, a watched PR with newer activity gets its agent woken with the follow-up prompt, or a new agent started. Newer activity is review feedback on your PR, or a re-request on a PR you review (pushes alone don't count). A watcher waits while its agent is working or waiting for you, and all watchers pause above `CUTIE_GUARD`. Watches end when the PR is merged or closed, and only run while cutie is open. The Activity panel logs every check.

herdr notifications fire when an agent starts waiting for you and when a watched PR of yours gets new feedback.

### Review queue

`a` on a PR in the Review tab puts it at the end of the review queue instead of starting an agent now. The queue starts one review at a time: the next PR gets its agent only when no review agent is starting, working or waiting for you. A PR that was merged, closed or no longer needs your review leaves the queue when its turn comes. The queue holds above `CUTIE_GUARD`, like the watchers, survives restarts, and only runs while cutie is open. The top of the Agents panel lists the queue in order and says what it is waiting for.

### Files

cutie keeps `watches.json`, `queue.json`, `sizes.json` and `prompts/` in `~/Library/Application Support/cutie/` (macOS) or `~/.config/cutie/` (Linux).

## Project layout

```text
main.go                  entry point: config, prompts, state → ui.Run
internal/
  config/                CUTIE_* settings
  github/                PR queries through gh, status rules, rate limit
  herdr/                 workspaces, panes, agents, notifications
  omp/                   Claude plan windows, 24h stats, per-session cost
  agents/                agent names, watch triggers, prompt templates
  state/                 watches, review queue and panel sizes on disk
  ui/                    Bubble Tea model, views, demo data
  utils/                 running CLIs, formatting, text layout
docs/demo.tape           recording script for docs/demo.gif
```

## ☕ Support

If cutie saves you time, you can support its development:

[![Buy Me A Coffee](https://img.shields.io/badge/Buy%20Me%20A%20Coffee-support%20development-orange?style=for-the-badge&logo=buy-me-a-coffee)](https://buymeacoffee.com/sitoularohansharma)

## License

[MIT](LICENSE)
