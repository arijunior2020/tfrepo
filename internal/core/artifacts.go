package core

import (
	"time"

	"github.com/arijunior2020/tfrepo/internal/provider"
)

// ProviderRef identifies a provider and namespace within an artifact's
// source/target metadata.
type ProviderRef struct {
	Provider  string `json:"provider"` // "github" | "gitlab"
	Namespace string `json:"namespace"`
}

// RepositoryInventory is the snapshot of a single repository recorded in
// inventory.json. Unlike provider.RepositoryDetails, it has no Namespace
// field — the namespace is implied by the enclosing InventoryNamespace.
type RepositoryInventory struct {
	Name          string              `json:"name"`
	DefaultBranch string              `json:"defaultBranch"`
	Visibility    provider.Visibility `json:"visibility"`
	SizeKB        int64               `json:"sizeKb"`
	Branches      []string            `json:"branches"`
	Tags          []string            `json:"tags"`
}

// InventoryNamespace groups the repositories scanned for one namespace.
type InventoryNamespace struct {
	Slug         string                 `json:"slug"`
	Name         string                 `json:"name"`
	Kind         provider.NamespaceKind `json:"kind"`
	Repositories []RepositoryInventory  `json:"repositories"`
}

// Inventory is the contents of inventory.json, produced by `tfrepo scan`.
type Inventory struct {
	GeneratedAt time.Time            `json:"generatedAt"`
	Source      ProviderRef          `json:"source"`
	Namespaces  []InventoryNamespace `json:"namespaces"`
}

// TaskEndpoint identifies a repository within a source or target namespace.
type TaskEndpoint struct {
	Namespace string `json:"namespace"`
	Repo      string `json:"repo"`
}

// MigrationTask describes the migration of a single repository from source
// to target.
type MigrationTask struct {
	ID       string       `json:"id"`
	Source   TaskEndpoint `json:"source"`
	Target   TaskEndpoint `json:"target"`
	Branches []string     `json:"branches"`
	Tags     []string     `json:"tags"`
}

// MigrationPlan is the contents of migration-plan.json, produced by
// `tfrepo plan`.
type MigrationPlan struct {
	GeneratedAt time.Time       `json:"generatedAt"`
	Source      ProviderRef     `json:"source"`
	Target      ProviderRef     `json:"target"`
	Tasks       []MigrationTask `json:"tasks"`
}

// MigrationResult is the outcome of migrating a single repository.
type MigrationResult struct {
	ID         string    `json:"id"`
	Status     string    `json:"status"` // "success" | "failed" | "dry-run"
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	Error      string    `json:"error,omitempty"` // always passed through security.Redact() before being set
}

// MigrationReport is the contents of migration-report.json, produced by
// `tfrepo migrate`.
type MigrationReport struct {
	GeneratedAt time.Time         `json:"generatedAt"`
	Results     []MigrationResult `json:"results"`
}
