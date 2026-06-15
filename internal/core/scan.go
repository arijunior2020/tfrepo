package core

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/arijunior2020/tfrepo/internal/provider"
)

// Scan builds an Inventory for namespace by listing its repositories and
// fetching branch/tag details for each one, using up to concurrency
// goroutines. Values of concurrency below 1 are treated as 1.
func Scan(ctx context.Context, p provider.RepositoryProvider, namespace string, concurrency int) (Inventory, error) {
	if concurrency < 1 {
		concurrency = 1
	}

	namespaces, err := p.ListNamespaces(ctx)
	if err != nil {
		return Inventory{}, err
	}

	var target *provider.Namespace
	for i := range namespaces {
		if namespaces[i].Slug == namespace {
			target = &namespaces[i]
			break
		}
	}
	if target == nil {
		return Inventory{}, fmt.Errorf("namespace %q not found for provider %q", namespace, p.Name())
	}

	repositories, err := p.ListRepositories(ctx, namespace)
	if err != nil {
		return Inventory{}, err
	}

	details := make([]provider.RepositoryDetails, len(repositories))

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	for i, repo := range repositories {
		g.Go(func() error {
			d, err := p.GetRepositoryDetails(gctx, namespace, repo.Name)
			if err != nil {
				return err
			}
			details[i] = d
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return Inventory{}, err
	}

	repositoryInventories := make([]RepositoryInventory, len(details))
	for i, d := range details {
		repositoryInventories[i] = RepositoryInventory{
			Name:          d.Name,
			DefaultBranch: d.DefaultBranch,
			Visibility:    d.Visibility,
			SizeKB:        d.SizeKB,
			Branches:      d.Branches,
			Tags:          d.Tags,
		}
	}

	return Inventory{
		GeneratedAt: time.Now().UTC(),
		Source:      ProviderRef{Provider: p.Name(), Namespace: namespace},
		Namespaces: []InventoryNamespace{
			{
				Slug:         target.Slug,
				Name:         target.Name,
				Kind:         target.Kind,
				Repositories: repositoryInventories,
			},
		},
	}, nil
}
