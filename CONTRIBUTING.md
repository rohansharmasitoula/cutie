# Contributing to cutie

Thanks for helping. Bug reports, ideas and pull requests are all welcome.

## Before you start

- **Bugs and ideas:** open an [issue](https://github.com/rohansharmasitoula/cutie/issues/new/choose) first, so we can agree on the fix before you write it.
- **Small fixes** (typos, docs, an obvious bug) can go straight to a pull request.
- **Security problems:** don't open a public issue; see [SECURITY.md](SECURITY.md).

## Set up

You need Go 1.26+ (see `go.mod`). `gh` and herdr are only needed to run cutie against real PRs; demo mode needs neither.

```sh
git clone https://github.com/rohansharmasitoula/cutie
cd cutie
go build -o cutie .
CUTIE_DEMO=1 ./cutie
```

Demo mode (`CUTIE_DEMO=1`) shows made-up PRs, agents and usage, and never calls GitHub, herdr or omp. Use it for UI work; the data lives in `internal/ui/demo.go`. `CUTIE_DEMO=herdr` keeps the made-up data but opens real herdr panes running scripted agents (`internal/ui/demoherdr.go`); run it inside a throwaway session (`herdr --session cutie-demo`) so it stays out of your own workspaces. The README's [Project layout](README.md#project-layout) says which package owns what.

## Checks

CI runs these on every pull request, and they must pass before merge. Run them first:

```sh
gofmt -l .        # must print nothing
go vet ./...
go build ./...
go test ./...
```

## Pull requests

1. Branch from `main` and keep one change per pull request.
2. Write commit messages as a short sentence saying what changed, e.g. `Review queue: skip PRs that no longer need you`.
3. Update the docs in the same pull request:
   - `README.md` for anything a user sees: keys, settings, statuses, files.
   - `CHANGELOG.md` under **Unreleased**, in the section that fits (Added, Changed, Fixed, Removed).
   - `docs/demo.gif` if the UI changed visibly: install [vhs](https://github.com/charmbracelet/vhs) and herdr, then run `env -u NO_COLOR -u CI vhs docs/demo.tape` from the repo root. It records in a throwaway herdr session and removes it afterwards.
4. Fill in the pull request template. `main` is protected: merging needs a pull request with green CI and resolved conversations, and pull requests are squash-merged.

## Releases (maintainer)

1. Move the **Unreleased** entries in `CHANGELOG.md` under a new version heading and update the compare links.
2. Tag and push: `git tag v0.2.0 && git push origin v0.2.0`.
3. The release workflow builds macOS and Linux binaries with GoReleaser and publishes a GitHub release with notes grouped by pull request label (see `.github/release.yml`).

## Code of conduct

Everyone taking part follows the [Code of Conduct](CODE_OF_CONDUCT.md).
