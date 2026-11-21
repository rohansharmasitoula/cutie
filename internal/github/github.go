// Package github loads your open PRs, the PRs you review and the issues assigned to
// you through the gh CLI.
package github

import (
	"encoding/json"
	"fmt"
	"github.com/rohansharmasitoula/cutie/internal/utils"
	"strings"
	"time"
)

// Status is the dashboard's verdict on a PR.
type Status int

const (
	StatusDraft Status = iota
	StatusInReview
	StatusNeedsWork
	StatusApproved
	StatusToReview        // review section: you are requested and have not reviewed it yet
	StatusReRequested     // review section: you reviewed it and were requested again
	StatusReviewedUpdated // review section: new commits since your last review, no new request
	StatusReviewed        // review section: you reviewed the current head, no new request
)

func (s Status) Label() string {
	switch s {
	case StatusDraft:
		return "draft"
	case StatusInReview:
		return "in review"
	case StatusNeedsWork:
		return "needs work"
	case StatusApproved:
		return "approved"
	case StatusToReview:
		return "to review"
	case StatusReRequested:
		return "re-review"
	case StatusReviewedUpdated:
		return "new commits"
	case StatusReviewed:
		return "reviewed"
	}
	return "?"
}

// Actionable reports whether Enter should start an agent for a PR in this status. A
// PR you review only needs you while GitHub lists you as a requested reviewer.
func (s Status) Actionable() bool {
	return s == StatusNeedsWork || s == StatusToReview || s == StatusReRequested
}

// Urgency orders statuses for the "urgent" sort: lower comes first.
func (s Status) Urgency() int {
	switch s {
	case StatusNeedsWork, StatusReRequested:
		return 0
	case StatusToReview:
		return 1
	case StatusInReview:
		return 2
	case StatusApproved:
		return 3
	case StatusReviewedUpdated:
		return 4
	case StatusReviewed:
		return 5
	}
	return 6 // draft
}

// CheckState is a CI check result normalized across CheckRun and StatusContext.
type CheckState int

const (
	CheckPass CheckState = iota
	CheckFail
	CheckPending
	CheckSkip
)

type Check struct {
	Name  string
	State CheckState
}

type Reviewer struct {
	Login string
	State string // APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED, REQUESTED
}

type Thread struct {
	Author string
	Path   string
	Line   int
	Body   string
	// OnReviewer: the next move is the reviewer's, because the PR author wrote the
	// thread's last comment or re-requested a review from whoever did.
	OnReviewer bool
}

type PR struct {
	Repo      string // owner/name
	RepoName  string // name
	Number    int
	Title     string
	URL       string
	Author    string
	Branch    string
	Base      string
	HeadOID   string
	IsDraft   bool
	Decision  string // APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED or ""
	CI        string // rollup: SUCCESS, FAILURE, PENDING, ERROR, EXPECTED or ""
	Checks    []Check
	Reviewers []Reviewer
	Threads   []Thread // unresolved only
	UpdatedAt time.Time
	// LastCommit is when the head commit was made: how long the PR has waited since the last push.
	LastCommit time.Time
	// LastFeedback is the newest review or comment by a human other than the author.
	LastFeedback time.Time
	// MyLastReview is when you last submitted a review; only for PRs you review.
	MyLastReview time.Time
	Status       Status
	Mine         bool
	IsBot        bool // author is a bot (dependabot, renovate, …)
}

// Key identifies a PR across refreshes.
func (p PR) Key() string { return fmt.Sprintf("%s#%d", p.Repo, p.Number) }

// Unresolved counts every open thread; Open counts only those waiting on the PR author.
func (p PR) Unresolved() int { return len(p.Threads) }

func (p PR) Open() int {
	n := 0
	for _, t := range p.Threads {
		if !t.OnReviewer {
			n++
		}
	}
	return n
}

const prFields = `
fragment F on PullRequest {
  id number title url isDraft updatedAt headRefName baseRefName headRefOid reviewDecision
  repository { nameWithOwner name }
  author { login __typename }
  reviewRequests(first: 10) { nodes { requestedReviewer { __typename ... on User { login } ... on Team { name } } } }
  latestReviews(first: 20) { nodes { state author { login } } }
  reviewThreads(first: 60) { nodes { isResolved path line
    comments(first: 1) { nodes { author { login } body } }
    last: comments(last: 1) { nodes { author { login } } } } }
  commits(last: 1) { nodes { commit { committedDate statusCheckRollup { state
    contexts(first: 50) { nodes { __typename
      ... on CheckRun { name status conclusion }
      ... on StatusContext { context state } } } } } } }
  comments(last: 50) { nodes { createdAt author { login __typename } } }
  reviews(last: 50) { nodes { submittedAt state commit { oid } author { login __typename } } }
}`

// listQuery is the cheap poll (a few rate-limit points): which PRs are open and when
// each last changed, plus your assigned issues in full. PR details are fetched only
// for PRs that changed.
const listQuery = `query {
  viewer { login }
  rateLimit { remaining limit resetAt }
  mine: search(query: "is:pr is:open author:@me archived:false", type: ISSUE, first: 50) { nodes { ...L } }
  requested: search(query: "is:pr is:open review-requested:@me archived:false", type: ISSUE, first: 50) { nodes { ...L } }
  reviewed: search(query: "is:pr is:open reviewed-by:@me -author:@me archived:false", type: ISSUE, first: 50) { nodes { ...L } }
  issues: search(query: "is:issue is:open assignee:@me archived:false", type: ISSUE, first: 100) { nodes { ...I } }
}
fragment L on PullRequest { id updatedAt }` + issueFields

// detailQuery loads full PR data for a batch of node IDs (about 2 points per PR).
const detailQuery = `query($ids: [ID!]!) {
  rateLimit { remaining limit resetAt }
  nodes(ids: $ids) { ...F }
}` + prFields

// detailBatch keeps each detail query well under GitHub's per-query node limits.
const detailBatch = 20

type actor struct {
	Login    string `json:"login"`
	Typename string `json:"__typename"`
}

func (a *actor) human() bool {
	return a != nil && a.Typename != "Bot" && !strings.HasSuffix(a.Login, "[bot]")
}

type rawPR struct {
	ID             string    `json:"id"`
	Number         int       `json:"number"`
	Title          string    `json:"title"`
	URL            string    `json:"url"`
	IsDraft        bool      `json:"isDraft"`
	UpdatedAt      time.Time `json:"updatedAt"`
	HeadRefName    string    `json:"headRefName"`
	BaseRefName    string    `json:"baseRefName"`
	HeadRefOid     string    `json:"headRefOid"`
	ReviewDecision string    `json:"reviewDecision"`
	Repository     struct {
		NameWithOwner string `json:"nameWithOwner"`
		Name          string `json:"name"`
	} `json:"repository"`
	Author         *actor `json:"author"`
	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer *struct {
				Login string `json:"login"`
				Name  string `json:"name"`
			} `json:"requestedReviewer"`
		} `json:"nodes"`
	} `json:"reviewRequests"`
	LatestReviews struct {
		Nodes []struct {
			State  string `json:"state"`
			Author *actor `json:"author"`
		} `json:"nodes"`
	} `json:"latestReviews"`
	ReviewThreads struct {
		Nodes []struct {
			IsResolved bool   `json:"isResolved"`
			Path       string `json:"path"`
			Line       int    `json:"line"`
			Comments   struct {
				Nodes []struct {
					Author *actor `json:"author"`
					Body   string `json:"body"`
				} `json:"nodes"`
			} `json:"comments"`
			Last struct {
				Nodes []struct {
					Author *actor `json:"author"`
				} `json:"nodes"`
			} `json:"last"`
		} `json:"nodes"`
	} `json:"reviewThreads"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				CommittedDate     time.Time `json:"committedDate"`
				StatusCheckRollup *struct {
					State    string `json:"state"`
					Contexts struct {
						Nodes []struct {
							Typename   string `json:"__typename"`
							Name       string `json:"name"`
							Status     string `json:"status"`
							Conclusion string `json:"conclusion"`
							Context    string `json:"context"`
							State      string `json:"state"`
						} `json:"nodes"`
					} `json:"contexts"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
	Comments struct {
		Nodes []struct {
			CreatedAt time.Time `json:"createdAt"`
			Author    *actor    `json:"author"`
		} `json:"nodes"`
	} `json:"comments"`
	Reviews struct {
		Nodes []struct {
			SubmittedAt time.Time `json:"submittedAt"`
			State       string    `json:"state"`
			Commit      *struct {
				OID string `json:"oid"`
			} `json:"commit"`
			Author *actor `json:"author"`
		} `json:"nodes"`
	} `json:"reviews"`
}

// RateLimit is the GraphQL budget left this hour (5,000 points for a user).
type RateLimit struct {
	Remaining int       `json:"remaining"`
	Limit     int       `json:"limit"`
	ResetAt   time.Time `json:"resetAt"`
}

type gqlError struct {
	Message string `json:"message"`
}

// graphql runs a query through the gh CLI (its auth) and decodes `.data` into out.
func graphql(out any, query string, args ...string) error {
	stdout, err := utils.Run("gh", append([]string{"api", "graphql", "-f", "query=" + query}, args...)...)
	var resp struct {
		Data   json.RawMessage `json:"data"`
		Errors []gqlError      `json:"errors"`
	}
	// gh prints the response body on stdout even when the query fails.
	if json.Unmarshal(stdout, &resp) == nil && len(resp.Errors) > 0 {
		return fmt.Errorf("GitHub: %s", resp.Errors[0].Message)
	}
	if err != nil {
		return err
	}
	if resp.Data == nil {
		return fmt.Errorf("GitHub: response without data")
	}
	return json.Unmarshal(resp.Data, out)
}

// FetchRateLimit reads the GraphQL budget over REST, which does not spend GraphQL points
// and still answers when the GraphQL budget is exhausted.
func FetchRateLimit() (RateLimit, error) {
	var resp struct {
		Resources struct {
			GraphQL struct {
				Limit     int   `json:"limit"`
				Remaining int   `json:"remaining"`
				Reset     int64 `json:"reset"`
			} `json:"graphql"`
		} `json:"resources"`
	}
	if err := utils.RunJSON(&resp, "gh", "api", "rate_limit"); err != nil {
		return RateLimit{}, err
	}
	g := resp.Resources.GraphQL
	return RateLimit{Remaining: g.Remaining, Limit: g.Limit, ResetAt: time.Unix(g.Reset, 0)}, nil
}

// Fetcher keeps full PR data between polls and re-fetches only PRs that changed.
// It is used by one poll at a time.
type Fetcher struct {
	cache map[string]rawPR // node ID → last full data
}

func NewFetcher() *Fetcher { return &Fetcher{cache: map[string]rawPR{}} }

type listNode struct {
	ID        string    `json:"id"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Snapshot is one poll's result.
type Snapshot struct {
	Login  string
	Mine   []PR // your open PRs
	Review []PR // open PRs you review
	Issues []Issue
	Rate   RateLimit
}

// Fetch returns your login, your open PRs, the open PRs you review, your assigned
// issues and the rate limit. A PR is re-fetched when it is new, its updatedAt moved,
// or its CI was still running (check results do not move updatedAt).
func (f *Fetcher) Fetch() (Snapshot, error) {
	var list struct {
		Viewer struct {
			Login string `json:"login"`
		} `json:"viewer"`
		RateLimit RateLimit                  `json:"rateLimit"`
		Mine      struct{ Nodes []listNode } `json:"mine"`
		Requested struct{ Nodes []listNode } `json:"requested"`
		Reviewed  struct{ Nodes []listNode } `json:"reviewed"`
		Issues    struct{ Nodes []rawIssue } `json:"issues"`
	}
	if err := graphql(&list, listQuery); err != nil {
		return Snapshot{}, err
	}
	s := Snapshot{Login: list.Viewer.Login, Rate: list.RateLimit}
	for _, r := range list.Issues.Nodes {
		if r.Number > 0 { // search can return non-issue nodes as empty objects
			s.Issues = append(s.Issues, convertIssue(r))
		}
	}

	var stale []string
	open := map[string]bool{}
	for _, nodes := range [][]listNode{list.Mine.Nodes, list.Requested.Nodes, list.Reviewed.Nodes} {
		for _, n := range nodes {
			if n.ID == "" || open[n.ID] { // search can return non-PR nodes as empty objects
				continue
			}
			open[n.ID] = true
			c, ok := f.cache[n.ID]
			if !ok || !c.UpdatedAt.Equal(n.UpdatedAt) || ciRunning(c) {
				stale = append(stale, n.ID)
			}
		}
	}
	for start := 0; start < len(stale); start += detailBatch {
		args := []string{}
		for _, id := range stale[start:min(start+detailBatch, len(stale))] {
			args = append(args, "-f", "ids[]="+id) // gh builds a JSON array from repeated key[]
		}
		var detail struct {
			RateLimit RateLimit `json:"rateLimit"`
			Nodes     []rawPR   `json:"nodes"`
		}
		if err := graphql(&detail, detailQuery, args...); err != nil {
			return Snapshot{Rate: s.Rate}, err
		}
		s.Rate = detail.RateLimit
		for _, r := range detail.Nodes {
			if r.ID != "" {
				f.cache[r.ID] = r
			}
		}
	}
	for id := range f.cache {
		if !open[id] { // merged or closed
			delete(f.cache, id)
		}
	}

	for _, n := range list.Mine.Nodes {
		if r, ok := f.cache[n.ID]; ok {
			s.Mine = append(s.Mine, convert(r, s.Login, true, false))
		}
	}
	// The requested search also finds requests to your teams, which the PR's own
	// reviewRequests cannot tie to you, so a PR is requested when that search lists it.
	seen := map[string]bool{}
	for i, nodes := range [][]listNode{list.Requested.Nodes, list.Reviewed.Nodes} {
		for _, n := range nodes {
			r, ok := f.cache[n.ID]
			if !ok || seen[n.ID] {
				continue
			}
			seen[n.ID] = true
			s.Review = append(s.Review, convert(r, s.Login, false, i == 0))
		}
	}
	return s, nil
}

func ciRunning(r rawPR) bool {
	if n := r.Commits.Nodes; len(n) > 0 && n[0].Commit.StatusCheckRollup != nil {
		s := n[0].Commit.StatusCheckRollup.State
		return s == "PENDING" || s == "EXPECTED"
	}
	return false
}

func checkState(typename, status, conclusion, state string) CheckState {
	if typename == "StatusContext" {
		switch state {
		case "SUCCESS":
			return CheckPass
		case "PENDING", "EXPECTED":
			return CheckPending
		}
		return CheckFail
	}
	if status != "COMPLETED" {
		return CheckPending
	}
	switch conclusion {
	case "SUCCESS":
		return CheckPass
	case "SKIPPED", "NEUTRAL":
		return CheckSkip
	}
	return CheckFail
}

// convert builds the dashboard's PR. requested: GitHub lists you as a reviewer it waits
// on (a first request, or a re-request after your review); only for PRs you review.
func convert(r rawPR, me string, mine, requested bool) PR {
	p := PR{
		Repo:      r.Repository.NameWithOwner,
		RepoName:  r.Repository.Name,
		Number:    r.Number,
		Title:     r.Title,
		URL:       r.URL,
		Branch:    r.HeadRefName,
		Base:      r.BaseRefName,
		HeadOID:   r.HeadRefOid,
		IsDraft:   r.IsDraft,
		Decision:  r.ReviewDecision,
		UpdatedAt: r.UpdatedAt,
		Mine:      mine,
	}
	if r.Author != nil {
		p.Author = r.Author.Login
		p.IsBot = !r.Author.human()
	}
	// asked holds the users GitHub still lists as requested reviewers. Submitting a review
	// clears the request, so a reviewer listed here who already commented was re-requested:
	// their threads and feedback wait on them, not on you.
	asked := map[string]bool{}
	for _, rq := range r.ReviewRequests.Nodes {
		if who := rq.RequestedReviewer; who != nil && who.Login != "" {
			asked[who.Login] = true
		}
	}
	for _, t := range r.ReviewThreads.Nodes {
		if t.IsResolved {
			continue
		}
		th := Thread{Path: t.Path, Line: t.Line}
		if c := t.Comments.Nodes; len(c) > 0 {
			if c[0].Author != nil {
				th.Author = c[0].Author.Login
			}
			th.Body = c[0].Body
		}
		if l := t.Last.Nodes; len(l) > 0 && l[0].Author != nil {
			th.OnReviewer = l[0].Author.Login == p.Author || asked[l[0].Author.Login]
		}
		p.Threads = append(p.Threads, th)
	}
	for _, rv := range r.LatestReviews.Nodes {
		if rv.Author != nil {
			p.Reviewers = append(p.Reviewers, Reviewer{Login: rv.Author.Login, State: rv.State})
		}
	}
	for _, rq := range r.ReviewRequests.Nodes {
		if who := rq.RequestedReviewer; who != nil {
			p.Reviewers = append(p.Reviewers, Reviewer{Login: who.Login + who.Name, State: "REQUESTED"})
		}
	}
	if n := r.Commits.Nodes; len(n) > 0 {
		p.LastCommit = n[0].Commit.CommittedDate
		if roll := n[0].Commit.StatusCheckRollup; roll != nil {
			p.CI = roll.State
			for _, c := range roll.Contexts.Nodes {
				name := c.Name + c.Context // exactly one is set
				p.Checks = append(p.Checks, Check{Name: name, State: checkState(c.Typename, c.Status, c.Conclusion, c.State)})
			}
		}
	}
	// approved holds the reviewers whose latest verdict is an approval. Their comments do
	// not mean work: replying to threads and then approving leaves a COMMENTED review just
	// before the APPROVED one, and many reviewers post an "LGTM" comment after approving.
	approved := map[string]bool{}
	for _, rv := range r.Reviews.Nodes { // oldest first, so the last verdict wins
		switch rv.State {
		case "APPROVED":
			approved[rv.Author.Login] = true
		case "CHANGES_REQUESTED", "DISMISSED":
			approved[rv.Author.Login] = false
		}
	}
	var waiting time.Time // newest feedback that still waits on you
	note := func(who string, at time.Time) {
		if at.After(p.LastFeedback) {
			p.LastFeedback = at
		}
		if !approved[who] && !asked[who] && at.After(waiting) {
			waiting = at
		}
	}
	for _, c := range r.Comments.Nodes {
		if c.Author.human() && c.Author.Login != p.Author {
			note(c.Author.Login, c.CreatedAt)
		}
	}
	myReviewedOID, iReviewed := "", false
	for _, rv := range r.Reviews.Nodes {
		if rv.State == "PENDING" || !rv.Author.human() {
			continue
		}
		feedback := rv.State == "COMMENTED" || rv.State == "CHANGES_REQUESTED"
		if feedback && rv.Author.Login != p.Author {
			note(rv.Author.Login, rv.SubmittedAt)
		}
		if rv.Author.Login == me {
			iReviewed = true
			p.MyLastReview = rv.SubmittedAt
			if rv.Commit != nil {
				myReviewedOID = rv.Commit.OID // reviews are oldest first, so the last one wins
			}
		}
	}

	switch {
	case !mine && requested && !iReviewed:
		p.Status = StatusToReview
	case !mine && requested:
		p.Status = StatusReRequested
	case !mine && myReviewedOID != p.HeadOID:
		p.Status = StatusReviewedUpdated
	case !mine:
		p.Status = StatusReviewed
	case p.IsDraft:
		p.Status = StatusDraft
	// A CHANGES_REQUESTED decision alone stays set after you push the fixes, until the
	// reviewer looks again; it only means work when a thread waits on you (you were not
	// the last to reply and did not re-request its reviewer) or feedback from someone
	// who has not approved and is not re-requested is newer than your last push.
	case p.Open() > 0 || waiting.After(p.LastCommit):
		p.Status = StatusNeedsWork
	case p.Decision == "APPROVED":
		p.Status = StatusApproved
	default:
		p.Status = StatusInReview
	}
	return p
}
