package ui

import (
	"time"

	"github.com/rohansharmasitoula/cutie/internal/agents"
	"github.com/rohansharmasitoula/cutie/internal/github"
	"github.com/rohansharmasitoula/cutie/internal/herdr"
	"github.com/rohansharmasitoula/cutie/internal/omp"
)

// Demo mode (CUTIE_DEMO=1) shows made-up PRs, agents and usage, and calls neither
// GitHub, herdr nor omp. It exists for screenshots and for trying cutie out.

func demoPRs() (mine, review []github.PR) {
	now := time.Now()
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	pr := func(repo string, n int, title, author, branch, base string, st github.Status, mineSide bool) github.PR {
		return github.PR{
			Repo: "acme/" + repo, RepoName: repo, Number: n, Title: title, Author: author,
			URL: "https://github.com/acme/" + repo + "/pull/", Branch: branch, Base: base,
			HeadOID: "4f2c9a1e", Status: st, Mine: mineSide, CI: "SUCCESS",
			UpdatedAt: ago(time.Duration(n%9+1) * time.Hour), LastCommit: ago(time.Duration(n%5+1) * 7 * time.Hour),
		}
	}
	checks := []github.Check{{Name: "lint", State: github.CheckPass}, {Name: "unit", State: github.CheckPass}, {Name: "e2e", State: github.CheckFail}}

	auth := pr("api", 412, "Feat | Auth | rotate refresh tokens on every use", "you", "you/412-rotate-tokens", "main", github.StatusNeedsWork, true)
	auth.CI, auth.Checks = "FAILURE", checks
	auth.Decision = "CHANGES_REQUESTED"
	auth.Reviewers = []github.Reviewer{{Login: "maya", State: "CHANGES_REQUESTED"}, {Login: "leo", State: "REQUESTED"}}
	auth.Threads = []github.Thread{
		{Author: "maya", Path: "auth/refresh.go", Line: 88, Body: "concern: the old token stays valid until expiry, so a stolen one keeps working after rotation."},
		{Author: "maya", Path: "auth/refresh_test.go", Line: 12, Body: "suggestion: add a test that replays a rotated token and expects 401."},
		{Author: "leo", Path: "docs/auth.md", Line: 40, Body: "question: should the docs mention the new grace window?", Replied: true},
	}
	stack1 := pr("api", 418, "Feat | Billing | usage meter read model", "you", "you/418-usage-meter", "main", github.StatusInReview, true)
	stack2 := pr("api", 421, "Feat | Billing | invoices read from the usage meter", "you", "you/421-invoices", "you/418-usage-meter", github.StatusInReview, true)
	stack2.CI = "PENDING"
	web := pr("web", 233, "Fix | Dashboard | empty state flickers on slow networks", "you", "you/233-empty-state", "main", github.StatusApproved, true)
	draft := pr("infra", 97, "Chore | CI | cache Go modules between jobs", "you", "you/97-go-cache", "main", github.StatusDraft, true)
	mine = []github.PR{auth, stack1, stack2, web, draft}

	r1 := pr("web", 240, "Feat | Settings | team roles page", "maya", "maya/240-roles", "main", github.StatusToReview, false)
	r2 := pr("api", 409, "Perf | Search | batch index writes", "leo", "leo/409-batch-index", "main", github.StatusReviewedUpdated, false)
	r2.Threads = []github.Thread{{Author: "you", Path: "search/index.go", Line: 51, Body: "the batch size should come from config"}}
	r3 := pr("infra", 102, "Feat | Terraform | staging Redis cluster", "sam", "sam/102-redis", "main", github.StatusReviewed, false)
	review = []github.PR{r1, r2, r3}
	return mine, review
}

func demoAgents(mine, review []github.PR, issues []github.Issue) []herdr.Agent {
	agent := func(name, status string, cost float64) herdr.Agent {
		return herdr.Agent{Name: name, Kind: "omp", Status: status, PaneID: "w2:" + name, Cost: cost}
	}
	return []herdr.Agent{
		agent(agents.Name(mine[0]), "working", 3.12),
		agent(agents.Name(review[1]), "blocked", 1.48),
		agent(agents.Name(mine[1]), "done", 6.35),
		agent(agents.IssueName(issues[0]), "working", 0.87),
		{Kind: "omp", Status: "idle", PaneID: "w1:p1", Title: "π main: planning the Q3 migration", Cost: 0.92},
	}
}

func demoIssues() []github.Issue {
	now := time.Now()
	issue := func(repo string, n int, title, author string, labels []string, comments int, age time.Duration, body string) github.Issue {
		return github.Issue{
			Repo: "acme/" + repo, RepoName: repo, Number: n, Title: title, Author: author, Labels: labels,
			URL: "https://github.com/acme/" + repo + "/issues/", Comments: comments, Body: body,
			CreatedAt: now.Add(-4 * age), UpdatedAt: now.Add(-age),
		}
	}
	return []github.Issue{
		issue("api", 430, "Rate-limit the public search endpoint", "maya", []string{"backend", "security"}, 4, 3*time.Hour,
			"Search has no limit per API key, so one client can starve the others.\n\nAcceptance:\n- 60 requests a minute per key, configurable\n- 429 with a Retry-After header\n- a metric for rejected requests"),
		issue("web", 251, "Dark mode for the settings pages", "leo", []string{"frontend"}, 1, 26*time.Hour,
			"The settings pages ignore the theme toggle and stay light."),
		issue("infra", 110, "Alert when the job queue backs up", "sam", []string{"ops"}, 0, 72*time.Hour,
			"Page on-call when the queue is over 1,000 jobs for 10 minutes."),
	}
}

func demoTail() []string {
	return []string{
		"Reading the unresolved review threads",
		"Fixing auth/refresh.go: revoke the previous token on rotation",
		"Added TestRefreshReplayRejected",
		"go test ./auth/... ok (2.1s)",
		"Committed 2 changes on the PR branch",
		"Drafting a reply for each thread…",
	}
}

func demoUsage() ([]omp.Limit, omp.Stats, github.RateLimit) {
	now := time.Now()
	limits := []omp.Limit{
		{Window: "5h", Used: 0.42, ResetsAt: now.Add(2*time.Hour + 14*time.Minute)},
		{Window: "7d", Used: 0.63, ResetsAt: now.Add(3*24*time.Hour + 5*time.Hour)},
	}
	hourly := []int64{2, 1, 1, 0, 0, 1, 3, 9, 14, 22, 18, 25, 31, 27, 19, 24, 28, 17, 12, 8, 5, 9, 15, 21}
	for i := range hourly {
		hourly[i] *= 1_000_000
	}
	stats := omp.Stats{
		Cost: 48.2, Tokens: 331_000_000, HourlyTokens: hourly,
		Models:  []omp.Share{{Name: "opus-4", Cost: 41.0}, {Name: "sonnet-4", Cost: 6.7}, {Name: "haiku-4", Cost: 0.5}},
		Folders: []omp.Share{{Name: "api", Cost: 29.4}, {Name: "web", Cost: 12.1}, {Name: "infra", Cost: 6.7}},
	}
	return limits, stats, github.RateLimit{Remaining: 4870, Limit: 5000, ResetAt: now.Add(41 * time.Minute)}
}

func demoEvents() []Event {
	now := time.Now()
	return []Event{
		{now.Add(-2 * time.Minute), EventBad, "new review feedback on api #412"},
		{now.Add(-6 * time.Minute), EventBad, "rev-api-409 is waiting for you"},
		{now.Add(-11 * time.Minute), EventGood, "fix-api-418 finished"},
		{now.Add(-19 * time.Minute), EventGood, "web #233 approved"},
		{now.Add(-25 * time.Minute), EventInfo, "watchers checked 2 PR(s) · 1 woken"},
	}
}
