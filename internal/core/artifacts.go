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
