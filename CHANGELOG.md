# Changelog

Notable changes to cutie. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- Review mode per PR you review: copilot drafts the review and waits for your OK, autopilot posts it with a verdict and opens follow-up issues by itself. `m` switches it, even mid-review. Kept in `modes.json`.
- `{{.Posting}}` in the review prompts carries the mode's rule on posting. Custom `review-start.md` and `review-followup.md` must contain it, or cutie refuses to start.
- Review queue: `a` queues a PR you review, and the queue starts one review at a time. The Agents panel lists it.
- The Issues tab shows each issue's linked PRs.
- `cutie --version`.
- Release binaries for macOS and Linux on every tag.

### Changed

- A watcher on a PR you review sends a re-request to the review queue instead of starting an agent straight away.
- `enter`, `a` and the queue only act on PRs that still request your review (`to review`, `re-review`).
- Re-requesting a reviewer hands their threads and feedback back to them, so your PR stops showing `needs work` for them.
- With several Claude accounts in omp, `CUTIE_GUARD` follows the account with the most room left, and a usage report older than its window's reset counts as unused.

## [0.1.3] - 2026-11-16

### Added

- Issues tab: open issues assigned to you; `enter` starts an agent on one.

## [0.1.2] - 2026-11-16

### Changed

- Clones are found directly in `CUTIE_ROOT` or one folder below it.

## [0.1.1] - 2026-11-16

### Fixed

- Agents that stop at a startup question are waited on instead of being sent the prompt too early.

## [0.1.0] - 2026-11-16

### Added

- First release: the PR dashboard with herdr agents, Claude usage from omp, watchers, configurable prompts and agent kind, demo mode and CI.

[Unreleased]: https://github.com/rohansharmasitoula/cutie/compare/v0.1.3...HEAD
[0.1.3]: https://github.com/rohansharmasitoula/cutie/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/rohansharmasitoula/cutie/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/rohansharmasitoula/cutie/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/rohansharmasitoula/cutie/releases/tag/v0.1.0
