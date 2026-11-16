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
	return i
}
