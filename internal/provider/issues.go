package provider

// Issue is the provider-neutral representation of a repository issue.
// State is always "open" or "closed" regardless of the source provider.
// Labels contains label names (not colors).
// MilestoneExternalID, when set, is the source provider's milestone ExternalID
// (for GitHub: milestone number; for GitLab: milestone.ID). The core layer
// translates this to the target provider's ID before calling CreateIssue.
type Issue struct {
	ExternalID          int64    `json:"externalId"`
	Title               string   `json:"title"`
	Body                string   `json:"body"`
	State               string   `json:"state"` // "open" | "closed"
	Labels              []string `json:"labels,omitempty"`
	MilestoneExternalID *int64   `json:"milestoneExternalId,omitempty"`
}
