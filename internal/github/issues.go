package github

import (
	"fmt"
	"time"
)

// Issue is an open issue assigned to you.
type Issue struct {
	Repo      string // owner/name
	RepoName  string // name
	Number    int
	Title     string
	URL       string
	Author    string
	Body      string
	Labels    []string
	Comments  int
	CreatedAt time.Time
	UpdatedAt time.Time
	// Linked are the PRs GitHub links to the issue (its Development section, or
	// "Closes #N" in a PR), oldest first. Closed unmerged PRs are left out.
	Linked []LinkedPR
}

// LinkedPR is a PR linked to an issue.
type LinkedPR struct {
	Repo    string // owner/name
	Number  int
	URL     string
	State   string // OPEN or MERGED
	IsDraft bool
}

// Key identifies an issue across refreshes; it never collides with a PR key because
// issues and PRs share one number sequence per repo.
func (i Issue) Key() string { return fmt.Sprintf("%s#%d", i.Repo, i.Number) }

// issueFields is fetched straight from the list query: issues are few and small, so
// they need no change-detection cache like PRs.
const issueFields = `
fragment I on Issue {
  number title url body createdAt updatedAt
  repository { nameWithOwner name }
  author { login }
  labels(first: 10) { nodes { name } }
  comments { totalCount }
  closedByPullRequestsReferences(first: 5, includeClosedPrs: true) {
    nodes { number url state isDraft repository { nameWithOwner } }
  }
}`

type rawIssue struct {
	Number     int       `json:"number"`
	Title      string    `json:"title"`
	URL        string    `json:"url"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
		Name          string `json:"name"`
	} `json:"repository"`
	Author *actor `json:"author"`
	Labels struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`
	Comments struct {
		TotalCount int `json:"totalCount"`
	} `json:"comments"`
	ClosedBy struct {
		Nodes []struct {
			Number     int    `json:"number"`
			URL        string `json:"url"`
			State      string `json:"state"`
			IsDraft    bool   `json:"isDraft"`
			Repository struct {
				NameWithOwner string `json:"nameWithOwner"`
			} `json:"repository"`
		} `json:"nodes"`
	} `json:"closedByPullRequestsReferences"`
}

func convertIssue(r rawIssue) Issue {
	i := Issue{
		Repo:      r.Repository.NameWithOwner,
		RepoName:  r.Repository.Name,
		Number:    r.Number,
		Title:     r.Title,
		URL:       r.URL,
		Body:      r.Body,
		Comments:  r.Comments.TotalCount,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
	if r.Author != nil {
		i.Author = r.Author.Login
	}
	for _, l := range r.Labels.Nodes {
		i.Labels = append(i.Labels, l.Name)
	}
	for _, n := range r.ClosedBy.Nodes {
		if n.Number > 0 && n.State != "CLOSED" {
			i.Linked = append(i.Linked, LinkedPR{Repo: n.Repository.NameWithOwner, Number: n.Number, URL: n.URL, State: n.State, IsDraft: n.IsDraft})
		}
	}
	return i
}
