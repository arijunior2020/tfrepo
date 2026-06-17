package provider

import "time"

// MilestoneState indicates whether a milestone is open or closed.
type MilestoneState string

const (
	MilestoneStateOpen   MilestoneState = "open"
	MilestoneStateClosed MilestoneState = "closed"
)

// Label is a provider-neutral repository label.
// Color is a hex string WITHOUT the '#' prefix (e.g. "e11d48").
// Providers that require '#' (GitLab) add/strip it internally.
type Label struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
}

// Milestone is a provider-neutral repository milestone.
// ExternalID is the provider-assigned integer used to reference this
// milestone in issues: the milestone number on GitHub, the milestone ID
// on GitLab.
type Milestone struct {
	ExternalID  int64          `json:"externalId"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	State       MilestoneState `json:"state"`
	DueDate     *time.Time     `json:"dueDate,omitempty"`
}
