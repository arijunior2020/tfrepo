package provider

// PullRequest is the provider-neutral representation of a pull request (GitHub)
// or merge request (GitLab). Only open PRs are migrated; closed and merged are
// historical artifacts preserved in git history.
// State is always "open".
// SourceBranch is the head branch; TargetBranch is the base branch.
type PullRequest struct {
	ExternalID   int64  `json:"externalId"`
	Title        string `json:"title"`
	Body         string `json:"body"`
	State        string `json:"state"` // always "open"
	SourceBranch string `json:"sourceBranch"`
	TargetBranch string `json:"targetBranch"`
}
