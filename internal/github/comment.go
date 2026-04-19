// Package github handles posting and updating PR comments with DNS change plans.
package github

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	gh "github.com/google/go-github/v60/github"
	"github.com/ags4no/dnsync/internal/plan"
)

// Client wraps the GitHub API client for PR comment operations.
type Client struct {
	client *gh.Client
	owner  string
	repo   string
}

// NewClient creates a GitHub client from a token and repository slug (owner/repo).
func NewClient(token, repoSlug string) (*Client, error) {
	parts := strings.SplitN(repoSlug, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repository slug %q, expected owner/repo", repoSlug)
	}

	client := gh.NewClient(nil).WithAuthToken(token)

	return &Client{
		client: client,
		owner:  parts[0],
		repo:   parts[1],
	}, nil
}

// NewClientFromEnv creates a GitHub client using GITHUB_TOKEN and GITHUB_REPOSITORY env vars.
func NewClientFromEnv() (*Client, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN is not set")
	}
	repo := os.Getenv("GITHUB_REPOSITORY")
	if repo == "" {
		return nil, fmt.Errorf("GITHUB_REPOSITORY is not set")
	}
	return NewClient(token, repo)
}

// UpsertPlanComment creates or updates a PR comment containing the DNS change plan.
// It looks for an existing comment with the dnsync marker and updates it, or creates a new one.
func (c *Client) UpsertPlanComment(ctx context.Context, prNumber int, body string) error {
	// Look for existing comment
	existingID, err := c.findExistingComment(ctx, prNumber)
	if err != nil {
		return fmt.Errorf("searching for existing comment: %w", err)
	}

	if existingID != 0 {
		_, _, err = c.client.Issues.EditComment(ctx, c.owner, c.repo, existingID, &gh.IssueComment{
			Body: gh.String(body),
		})
		if err != nil {
			return fmt.Errorf("updating comment: %w", err)
		}
		return nil
	}

	_, _, err = c.client.Issues.CreateComment(ctx, c.owner, c.repo, prNumber, &gh.IssueComment{
		Body: gh.String(body),
	})
	if err != nil {
		return fmt.Errorf("creating comment: %w", err)
	}
	return nil
}

func (c *Client) findExistingComment(ctx context.Context, prNumber int) (int64, error) {
	opts := &gh.IssueListCommentsOptions{
		ListOptions: gh.ListOptions{PerPage: 100},
	}

	for {
		comments, resp, err := c.client.Issues.ListComments(ctx, c.owner, c.repo, prNumber, opts)
		if err != nil {
			return 0, err
		}

		for _, comment := range comments {
			if comment.Body != nil && strings.Contains(*comment.Body, plan.CommentMarker) {
				return *comment.ID, nil
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return 0, nil
}

// GetPRNumber extracts the PR number from the GITHUB_REF env var or a provided string.
func GetPRNumber() (int, error) {
	ref := os.Getenv("GITHUB_REF")
	if ref == "" {
		return 0, fmt.Errorf("GITHUB_REF is not set")
	}
	// Format: refs/pull/<number>/merge
	parts := strings.Split(ref, "/")
	if len(parts) < 3 || parts[1] != "pull" {
		return 0, fmt.Errorf("GITHUB_REF %q is not a pull request ref", ref)
	}
	n, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, fmt.Errorf("parsing PR number from %q: %w", ref, err)
	}
	return n, nil
}
