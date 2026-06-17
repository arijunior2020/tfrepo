# tfrepo migrate-prs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `tfrepo migrate-prs` — migra pull requests (apenas abertos) dos repositórios de origem para o destino e grava `prs-report.json`.

**Architecture:** Segue exatamente o padrão do Plan 9 (`migrate-issues`): tipo neutro `PullRequest` em `internal/provider/pull_requests.go`, dois métodos novos na interface `RepositoryProvider`, implementações GitHub/GitLab, função core `MigratePullRequests`, e comando Cobra `migrate-prs`. Apenas PRs abertos são migrados — PRs fechados e mesclados são artefatos históricos preservados no git.

**Tech Stack:** Go 1.24, `github.com/google/go-github/v74`, `gitlab.com/gitlab-org/api/client-go v1.46.0`, Cobra v1.10.2, `github.com/arijunior2020/tfrepo/internal/security`.

---

## File Map

| Arquivo | Ação |
|---|---|
| `internal/provider/pull_requests.go` | Criar — tipo `PullRequest` |
| `internal/provider/pull_requests_test.go` | Criar — JSON round-trip |
| `internal/provider/types.go` | Modificar — 2 métodos na interface |
| `internal/provider/github.go` | Modificar — `ListPullRequests`, `CreatePullRequest` |
| `internal/provider/github_test.go` | Modificar — 2 novos testes |
| `internal/provider/gitlab.go` | Modificar — `ListPullRequests`, `CreatePullRequest` |
| `internal/provider/gitlab_test.go` | Modificar — 2 novos testes |
| `internal/core/artifacts.go` | Modificar — `PRsMigrateResult`, `PRsReport` |
| `internal/core/pull_requests.go` | Criar — `MigratePullRequests` |
| `internal/core/pull_requests_test.go` | Criar — testes da lógica core |
| `internal/cli/migrate_prs.go` | Criar — comando Cobra |
| `internal/cli/migrate_prs_test.go` | Criar — testes CLI |
| `internal/cli/destroy.go` | Modificar — adicionar `prsReportPath` |
| `internal/cli/destroy_test.go` | Modificar — 6→7 arquivos |
| `internal/cli/root.go` | Modificar — registrar comando |
| `README.md` | Modificar — pipeline + seção + tabela |
| Todos `*_test.go` com fake providers | Modificar — 2 stubs novos cada |

---

## Contexto crítico para o implementador

### Padrões do projeto (NUNCA violar)

- `*int exitCode` em todos os comandos; signal handling em `newXxxCommand`, NÃO em `runXxx`
- `--config/-c` é flag **persistente** no root. Subcomandos lêem via `cmd.Flags().GetString(configFlagName)` dentro de `RunE` — NUNCA registrar flag local própria
- Erros em structs de relatório SEMPRE passam por `security.Redact(err.Error())` antes de entrar em `Errors []string`
- Mensagens de erro em inglês em todos os providers
- `listPerPage = 100` (constante já definida em `gitlab.go`)

### Escopo de migração

Apenas PRs com `State == "open"` são migrados. PRs fechados e mesclados têm branches frequentemente deletadas e não podem ser recriados de forma útil.

### APIs — GitHub Pull Requests

```go
// Listar (apenas abertos):
opts := &github.PullRequestListOptions{
    State:       "open",
    ListOptions: github.ListOptions{PerPage: 100},
}
page, resp, err := p.client.PullRequests.List(ctx, namespace, repo, opts)
// ExternalID: int64(pr.GetNumber())
// SourceBranch: pr.GetHead().GetRef()  (branch de origem)
// TargetBranch: pr.GetBase().GetRef()  (branch de destino, ex: "main")
// State: pr.GetState() → "open"

// Criar:
created, _, err := p.client.PullRequests.Create(ctx, namespace, repo, &github.NewPullRequest{
    Title: github.Ptr(pr.Title),
    Body:  github.Ptr(pr.Body),
    Head:  github.Ptr(pr.SourceBranch),
    Base:  github.Ptr(pr.TargetBranch),
})
// ExternalID: int64(created.GetNumber())
```

### APIs — GitLab Merge Requests

```go
// Listar (apenas abertos — "opened" na API):
state := "opened"
opts := &gitlab.ListProjectMergeRequestsOptions{
    State:       &state,
    ListOptions: gitlab.ListOptions{PerPage: listPerPage},
}
page, resp, err := p.client.MergeRequests.ListProjectMergeRequests(pid, opts, gitlab.WithContext(ctx))
// ExternalID: int64(mr.IID)   (IID = número do projeto, NÃO mr.ID global)
// SourceBranch: mr.SourceBranch
// TargetBranch: mr.TargetBranch
// State: "open"  (normalizar "opened" → "open")

// Criar:
created, _, err := p.client.MergeRequests.CreateMergeRequest(pid, &gitlab.CreateMergeRequestOptions{
    Title:        gitlab.Ptr(pr.Title),
    Description:  gitlab.Ptr(pr.Body),
    SourceBranch: gitlab.Ptr(pr.SourceBranch),
    TargetBranch: gitlab.Ptr(pr.TargetBranch),
}, gitlab.WithContext(ctx))
// ExternalID: int64(created.IID)
// Nota: se SourceBranch == TargetBranch, API retorna erro — trate como create error
```

### Helpers de conversão (evitar duplicação)

```go
// em github.go:
func githubPRtoPullRequest(pr *github.PullRequest) PullRequest { ... }

// em gitlab.go:
func gitlabMRtoPullRequest(mr *gitlab.MergeRequest) PullRequest { ... }
```

Reutilizar em `ListPullRequests` e no retorno de `CreatePullRequest`.

### Fakes em arquivos de teste

Após adicionar os 2 métodos à interface, `go build ./...` falhará. Encontrar todos com:

```bash
go build ./... 2>&1 | grep "does not implement\|missing method"
```

Arquivos tipicamente afetados:
- `internal/core/migrate_test.go` — `fakeMigrateProvider`
- `internal/core/validate_test.go` — `fakeValidateProvider`
- `internal/core/scan_test.go` — `fakeScanProvider`
- `internal/core/issues_test.go` — `fakeIssuesProvider`
- `internal/cli/migrate_test.go` — `fakeMigrateCliProvider`
- `internal/cli/scan_test.go` — `fakeScanProvider`
- `internal/cli/validate_test.go` — `fakeValidateCliProvider`
- `internal/cli/migrate_issues_test.go` — `fakeMigrateIssuesProvider`

Adicionar em cada um:

```go
func (f *fakeXxxProvider) ListPullRequests(_ context.Context, _, _ string) ([]provider.PullRequest, error) {
    panic("not implemented")
}

func (f *fakeXxxProvider) CreatePullRequest(_ context.Context, _, _ string, _ provider.PullRequest) (provider.PullRequest, error) {
    panic("not implemented")
}
```

---

## Task 1: Tipo `PullRequest` + Interface + Stubs

**Files:**
- Create: `internal/provider/pull_requests.go`
- Create: `internal/provider/pull_requests_test.go`
- Modify: `internal/provider/types.go`
- Modify: todos os arquivos `*_test.go` que falham em `go build ./...`

- [ ] **Step 1: Escrever o teste de JSON round-trip**

`internal/provider/pull_requests_test.go`:
```go
package provider_test

import (
	"encoding/json"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/provider"
)

func TestPullRequestJSONRoundTrip(t *testing.T) {
	orig := provider.PullRequest{
		ExternalID:   int64(5),
		Title:        "Add feature X",
		Body:         "This PR adds feature X",
		State:        "open",
		SourceBranch: "feature/x",
		TargetBranch: "main",
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got provider.PullRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.ExternalID != orig.ExternalID {
		t.Errorf("ExternalID = %d, want %d", got.ExternalID, orig.ExternalID)
	}
	if got.Title != orig.Title {
		t.Errorf("Title = %q, want %q", got.Title, orig.Title)
	}
	if got.SourceBranch != orig.SourceBranch {
		t.Errorf("SourceBranch = %q, want %q", got.SourceBranch, orig.SourceBranch)
	}
	if got.TargetBranch != orig.TargetBranch {
		t.Errorf("TargetBranch = %q, want %q", got.TargetBranch, orig.TargetBranch)
	}
	if got.State != orig.State {
		t.Errorf("State = %q, want %q", got.State, orig.State)
	}
}

func TestPullRequestJSONEmptyBody(t *testing.T) {
	orig := provider.PullRequest{
		ExternalID:   int64(1),
		Title:        "No body",
		State:        "open",
		SourceBranch: "feat",
		TargetBranch: "main",
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got provider.PullRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Body != "" {
		t.Errorf("Body = %q, want empty string", got.Body)
	}
}
```

- [ ] **Step 2: Rodar para verificar falha**

```bash
go test ./internal/provider/ -run TestPullRequest -v
```

Esperado: `FAIL` — `provider.PullRequest` não existe.

- [ ] **Step 3: Criar `internal/provider/pull_requests.go`**

```go
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
```

- [ ] **Step 4: Rodar para verificar que testes passam**

```bash
go test ./internal/provider/ -run TestPullRequest -v
```

Esperado: `PASS`.

- [ ] **Step 5: Adicionar métodos à interface em `internal/provider/types.go`**

Localizar o bloco de issues (adicionado no Plan 9) e adicionar após:

```go
	// Pull Requests (tfrepo migrate-prs)
	ListPullRequests(ctx context.Context, namespace, repo string) ([]PullRequest, error)
	CreatePullRequest(ctx context.Context, namespace, repo string, pr PullRequest) (PullRequest, error)
```

- [ ] **Step 6: Encontrar todos os fakes quebrados**

```bash
go build ./... 2>&1 | grep "does not implement\|missing method"
```

- [ ] **Step 7: Adicionar stubs em cada fake encontrado**

Para cada fake provider que falha, substituindo `fakeXxxProvider` pelo nome real do struct:

```go
func (f *fakeXxxProvider) ListPullRequests(_ context.Context, _, _ string) ([]provider.PullRequest, error) {
    panic("not implemented")
}

func (f *fakeXxxProvider) CreatePullRequest(_ context.Context, _, _ string, _ provider.PullRequest) (provider.PullRequest, error) {
    panic("not implemented")
}
```

- [ ] **Step 8: Verificar compilação limpa**

```bash
go build ./...
go test ./...
```

Esperado: compila sem erros; todos os testes existentes passam.

- [ ] **Step 9: Commit**

```bash
git add internal/provider/pull_requests.go internal/provider/pull_requests_test.go internal/provider/types.go
git add internal/core/migrate_test.go internal/core/validate_test.go internal/core/scan_test.go internal/core/issues_test.go
git add internal/cli/migrate_test.go internal/cli/scan_test.go internal/cli/validate_test.go internal/cli/migrate_issues_test.go
git commit -m "feat(provider): add PullRequest type and ListPullRequests/CreatePullRequest interface methods"
```

---

## Task 2: Implementação GitHub Provider

**Files:**
- Modify: `internal/provider/github.go`
- Modify: `internal/provider/github_test.go`

- [ ] **Step 1: Escrever os testes**

Adicionar em `internal/provider/github_test.go`:

```go
func TestGitHubProviderListPullRequests(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/myorg/myrepo/pulls", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != "open" {
			t.Errorf("state = %q, want %q", r.URL.Query().Get("state"), "open")
		}
		writeJSON(t, w, []*github.PullRequest{
			{
				Number: github.Ptr(3),
				Title:  github.Ptr("Add feature"),
				Body:   github.Ptr("body text"),
				State:  github.Ptr("open"),
				Head:   &github.PullRequestBranch{Ref: github.Ptr("feature/add")},
				Base:   &github.PullRequestBranch{Ref: github.Ptr("main")},
			},
		})
	})

	p, _ := newGitHubTestProvider(t, mux.ServeHTTP)
	prs, err := p.ListPullRequests(t.Context(), "myorg", "myrepo")
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("len(prs) = %d, want 1", len(prs))
	}
	got := prs[0]
	if got.ExternalID != 3 {
		t.Errorf("ExternalID = %d, want 3", got.ExternalID)
	}
	if got.Title != "Add feature" {
		t.Errorf("Title = %q, want %q", got.Title, "Add feature")
	}
	if got.SourceBranch != "feature/add" {
		t.Errorf("SourceBranch = %q, want feature/add", got.SourceBranch)
	}
	if got.TargetBranch != "main" {
		t.Errorf("TargetBranch = %q, want main", got.TargetBranch)
	}
	if got.State != "open" {
		t.Errorf("State = %q, want open", got.State)
	}
}

func TestGitHubProviderCreatePullRequest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/myorg/myrepo/pulls", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, &github.PullRequest{
			Number: github.Ptr(10),
			Title:  github.Ptr("Created PR"),
			Body:   github.Ptr("body"),
			State:  github.Ptr("open"),
			Head:   &github.PullRequestBranch{Ref: github.Ptr("feature/new")},
			Base:   &github.PullRequestBranch{Ref: github.Ptr("main")},
		})
	})

	p, _ := newGitHubTestProvider(t, mux.ServeHTTP)
	result, err := p.CreatePullRequest(t.Context(), "myorg", "myrepo", PullRequest{
		Title:        "Created PR",
		Body:         "body",
		State:        "open",
		SourceBranch: "feature/new",
		TargetBranch: "main",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}
	if result.ExternalID != 10 {
		t.Errorf("ExternalID = %d, want 10", result.ExternalID)
	}
	if result.SourceBranch != "feature/new" {
		t.Errorf("SourceBranch = %q, want feature/new", result.SourceBranch)
	}
	if result.State != "open" {
		t.Errorf("State = %q, want open", result.State)
	}
}
```

- [ ] **Step 2: Rodar para verificar falha**

```bash
go test ./internal/provider/ -run TestGitHubProviderListPullRequests -v
go test ./internal/provider/ -run TestGitHubProviderCreatePullRequest -v
```

Esperado: `FAIL` — métodos não implementados.

- [ ] **Step 3: Implementar em `internal/provider/github.go`**

```go
func githubPRtoPullRequest(pr *github.PullRequest) PullRequest {
	return PullRequest{
		ExternalID:   int64(pr.GetNumber()),
		Title:        pr.GetTitle(),
		Body:         pr.GetBody(),
		State:        pr.GetState(), // "open"
		SourceBranch: pr.GetHead().GetRef(),
		TargetBranch: pr.GetBase().GetRef(),
	}
}

func (p *GitHubProvider) ListPullRequests(ctx context.Context, namespace, repo string) ([]PullRequest, error) {
	var prs []PullRequest
	opts := &github.PullRequestListOptions{
		State:       "open",
		ListOptions: github.ListOptions{PerPage: 100},
	}
	for {
		page, resp, err := p.client.PullRequests.List(ctx, namespace, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("list pull requests for %s/%s: %w", namespace, repo, err)
		}
		for _, pr := range page {
			prs = append(prs, githubPRtoPullRequest(pr))
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return prs, nil
}

func (p *GitHubProvider) CreatePullRequest(ctx context.Context, namespace, repo string, pr PullRequest) (PullRequest, error) {
	created, _, err := p.client.PullRequests.Create(ctx, namespace, repo, &github.NewPullRequest{
		Title: github.Ptr(pr.Title),
		Body:  github.Ptr(pr.Body),
		Head:  github.Ptr(pr.SourceBranch),
		Base:  github.Ptr(pr.TargetBranch),
	})
	if err != nil {
		return PullRequest{}, fmt.Errorf("create pull request %q in %s/%s: %w", pr.Title, namespace, repo, err)
	}
	return githubPRtoPullRequest(created), nil
}
```

- [ ] **Step 4: Rodar para verificar que testes passam**

```bash
go test ./internal/provider/ -run TestGitHubProvider -v
```

Esperado: todos os testes GitHub passam.

- [ ] **Step 5: Rodar toda a suite**

```bash
go test ./...
```

Esperado: `ok` em todos os pacotes (exceto falhas pre-existentes de git bareRepo).

- [ ] **Step 6: Commit**

```bash
git add internal/provider/github.go internal/provider/github_test.go
git commit -m "feat(provider/github): implement ListPullRequests and CreatePullRequest"
```

---

## Task 3: Implementação GitLab Provider

**Files:**
- Modify: `internal/provider/gitlab.go`
- Modify: `internal/provider/gitlab_test.go`

- [ ] **Step 1: Escrever os testes**

Adicionar em `internal/provider/gitlab_test.go`:

```go
func TestGitLabProviderListPullRequests(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/mygroup%2Fmyrepo/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != "opened" {
			t.Errorf("state = %q, want opened", r.URL.Query().Get("state"))
		}
		writeJSON(t, w, []*gitlab.MergeRequest{
			{
				IID:          1,
				Title:        "Open MR",
				Description:  "desc",
				State:        "opened",
				SourceBranch: "feature/y",
				TargetBranch: "main",
			},
		})
	})

	p, _ := newGitLabTestServer(t, "test-token", mux.ServeHTTP)
	prs, err := p.ListPullRequests(t.Context(), "mygroup", "myrepo")
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("len(prs) = %d, want 1", len(prs))
	}
	got := prs[0]
	if got.ExternalID != 1 {
		t.Errorf("ExternalID = %d, want 1 (IID, não ID global)", got.ExternalID)
	}
	if got.State != "open" {
		t.Errorf("State = %q, want open (deve normalizar 'opened')", got.State)
	}
	if got.SourceBranch != "feature/y" {
		t.Errorf("SourceBranch = %q, want feature/y", got.SourceBranch)
	}
	if got.TargetBranch != "main" {
		t.Errorf("TargetBranch = %q, want main", got.TargetBranch)
	}
}

func TestGitLabProviderCreatePullRequest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/mygroup%2Fmyrepo/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, &gitlab.MergeRequest{
			IID:          7,
			Title:        "New MR",
			State:        "opened",
			SourceBranch: "feat/z",
			TargetBranch: "main",
		})
	})

	p, _ := newGitLabTestServer(t, "test-token", mux.ServeHTTP)
	result, err := p.CreatePullRequest(t.Context(), "mygroup", "myrepo", PullRequest{
		Title:        "New MR",
		Body:         "body",
		State:        "open",
		SourceBranch: "feat/z",
		TargetBranch: "main",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}
	if result.ExternalID != 7 {
		t.Errorf("ExternalID = %d, want 7 (IID)", result.ExternalID)
	}
	if result.State != "open" {
		t.Errorf("State = %q, want open", result.State)
	}
	if result.SourceBranch != "feat/z" {
		t.Errorf("SourceBranch = %q, want feat/z", result.SourceBranch)
	}
}
```

- [ ] **Step 2: Rodar para verificar falha**

```bash
go test ./internal/provider/ -run TestGitLabProviderListPullRequests -v
go test ./internal/provider/ -run TestGitLabProviderCreatePullRequest -v
```

Esperado: `FAIL` — métodos não implementados.

- [ ] **Step 3: Implementar em `internal/provider/gitlab.go`**

```go
func gitlabMRtoPullRequest(mr *gitlab.MergeRequest) PullRequest {
	state := "open"
	if mr.State == "closed" || mr.State == "merged" {
		state = "closed"
	}
	return PullRequest{
		ExternalID:   int64(mr.IID),
		Title:        mr.Title,
		Body:         mr.Description,
		State:        state,
		SourceBranch: mr.SourceBranch,
		TargetBranch: mr.TargetBranch,
	}
}

func (p *GitLabProvider) ListPullRequests(ctx context.Context, namespace, repo string) ([]PullRequest, error) {
	pid := gitlabPID(namespace, repo)
	state := "opened"
	var prs []PullRequest
	opts := &gitlab.ListProjectMergeRequestsOptions{
		State:       &state,
		ListOptions: gitlab.ListOptions{PerPage: listPerPage},
	}
	for {
		page, resp, err := p.client.MergeRequests.ListProjectMergeRequests(pid, opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list merge requests for %s: %w", pid, err)
		}
		for _, mr := range page {
			prs = append(prs, gitlabMRtoPullRequest(mr))
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return prs, nil
}

func (p *GitLabProvider) CreatePullRequest(ctx context.Context, namespace, repo string, pr PullRequest) (PullRequest, error) {
	pid := gitlabPID(namespace, repo)
	created, _, err := p.client.MergeRequests.CreateMergeRequest(pid, &gitlab.CreateMergeRequestOptions{
		Title:        gitlab.Ptr(pr.Title),
		Description:  gitlab.Ptr(pr.Body),
		SourceBranch: gitlab.Ptr(pr.SourceBranch),
		TargetBranch: gitlab.Ptr(pr.TargetBranch),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return PullRequest{}, fmt.Errorf("create merge request %q in %s: %w", pr.Title, pid, err)
	}
	return gitlabMRtoPullRequest(created), nil
}
```

- [ ] **Step 4: Rodar para verificar que testes passam**

```bash
go test ./internal/provider/ -run TestGitLabProvider -v
```

Esperado: todos os testes GitLab passam.

- [ ] **Step 5: Rodar toda a suite**

```bash
go test ./...
```

Esperado: `ok` em todos os pacotes (exceto falhas pre-existentes de git bareRepo).

- [ ] **Step 6: Commit**

```bash
git add internal/provider/gitlab.go internal/provider/gitlab_test.go
git commit -m "feat(provider/gitlab): implement ListPullRequests and CreatePullRequest"
```

---

## Task 4: Core Logic

**Files:**
- Modify: `internal/core/artifacts.go`
- Create: `internal/core/pull_requests.go`
- Create: `internal/core/pull_requests_test.go`

- [ ] **Step 1: Escrever os testes**

`internal/core/pull_requests_test.go`:

```go
package core_test

import (
	"context"
	"errors"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/core"
	"github.com/arijunior2020/tfrepo/internal/provider"
)

// fakePRsProvider implementa provider.RepositoryProvider para testes de PRs.
type fakePRsProvider struct {
	prs       map[string][]provider.PullRequest // chave: "namespace/repo"
	createErr error
	created   []provider.PullRequest
}

func (f *fakePRsProvider) Name() string { return "fake" }
func (f *fakePRsProvider) ListNamespaces(_ context.Context) ([]provider.Namespace, error) {
	panic("not implemented")
}
func (f *fakePRsProvider) ListRepositories(_ context.Context, _ string) ([]provider.RepositorySummary, error) {
	panic("not implemented")
}
func (f *fakePRsProvider) GetRepositoryDetails(_ context.Context, _, _ string) (provider.RepositoryDetails, error) {
	panic("not implemented")
}
func (f *fakePRsProvider) CreateRepository(_ context.Context, _ string, _ provider.CreateRepositoryInput) (provider.RepositorySummary, error) {
	panic("not implemented")
}
func (f *fakePRsProvider) GetAuthenticatedCloneURL(_, _ string) string { panic("not implemented") }
func (f *fakePRsProvider) GetRepositoryState(_ context.Context, _, _ string) (provider.RepositoryState, error) {
	panic("not implemented")
}
func (f *fakePRsProvider) ListLabels(_ context.Context, _, _ string) ([]provider.Label, error) {
	panic("not implemented")
}
func (f *fakePRsProvider) CreateLabel(_ context.Context, _, _ string, _ provider.Label) (provider.Label, error) {
	panic("not implemented")
}
func (f *fakePRsProvider) ListMilestones(_ context.Context, _, _ string) ([]provider.Milestone, error) {
	panic("not implemented")
}
func (f *fakePRsProvider) CreateMilestone(_ context.Context, _, _ string, _ provider.Milestone) (provider.Milestone, error) {
	panic("not implemented")
}
func (f *fakePRsProvider) ListIssues(_ context.Context, _, _ string) ([]provider.Issue, error) {
	panic("not implemented")
}
func (f *fakePRsProvider) CreateIssue(_ context.Context, _, _ string, _ provider.Issue) (provider.Issue, error) {
	panic("not implemented")
}
func (f *fakePRsProvider) ListPullRequests(_ context.Context, namespace, repo string) ([]provider.PullRequest, error) {
	return f.prs[namespace+"/"+repo], nil
}
func (f *fakePRsProvider) CreatePullRequest(_ context.Context, _, _ string, pr provider.PullRequest) (provider.PullRequest, error) {
	if f.createErr != nil {
		return provider.PullRequest{}, f.createErr
	}
	created := pr
	created.ExternalID = int64(len(f.created) + 100)
	f.created = append(f.created, created)
	return created, nil
}

func TestMigratePullRequests_HappyPath(t *testing.T) {
	source := &fakePRsProvider{
		prs: map[string][]provider.PullRequest{
			"srcorg/repo1": {
				{ExternalID: 1, Title: "PR A", State: "open", SourceBranch: "feat/a", TargetBranch: "main"},
				{ExternalID: 2, Title: "PR B", State: "open", SourceBranch: "feat/b", TargetBranch: "main"},
			},
		},
	}
	target := &fakePRsProvider{
		prs: map[string][]provider.PullRequest{
			"tgtorg/repo1": {},
		},
	}

	plan := core.MigrationPlan{
		Tasks: []core.MigrationTask{
			{ID: "t1", Source: core.TaskEndpoint{Namespace: "srcorg", Repo: "repo1"}, Target: core.TaskEndpoint{Namespace: "tgtorg", Repo: "repo1"}},
		},
	}
	report, err := core.MigratePullRequests(t.Context(), plan, core.PRsMigrateProviders{Source: source, Target: target})
	if err != nil {
		t.Fatalf("MigratePullRequests: %v", err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(report.Results))
	}
	r := report.Results[0]
	if r.Status != "success" {
		t.Errorf("Status = %q, want success", r.Status)
	}
	if r.PRsCreated != 2 {
		t.Errorf("PRsCreated = %d, want 2", r.PRsCreated)
	}
}

func TestMigratePullRequests_SkipsExistingByTitle(t *testing.T) {
	source := &fakePRsProvider{
		prs: map[string][]provider.PullRequest{
			"srcorg/repo1": {
				{ExternalID: 1, Title: "Existing PR", State: "open", SourceBranch: "feat/x", TargetBranch: "main"},
				{ExternalID: 2, Title: "New PR", State: "open", SourceBranch: "feat/y", TargetBranch: "main"},
			},
		},
	}
	target := &fakePRsProvider{
		prs: map[string][]provider.PullRequest{
			"tgtorg/repo1": {
				{ExternalID: 50, Title: "Existing PR", State: "open", SourceBranch: "feat/x", TargetBranch: "main"},
			},
		},
	}

	plan := core.MigrationPlan{
		Tasks: []core.MigrationTask{
			{ID: "t1", Source: core.TaskEndpoint{Namespace: "srcorg", Repo: "repo1"}, Target: core.TaskEndpoint{Namespace: "tgtorg", Repo: "repo1"}},
		},
	}
	report, err := core.MigratePullRequests(t.Context(), plan, core.PRsMigrateProviders{Source: source, Target: target})
	if err != nil {
		t.Fatalf("MigratePullRequests: %v", err)
	}
	r := report.Results[0]
	if r.PRsCreated != 1 {
		t.Errorf("PRsCreated = %d, want 1 (PR existente deve ser ignorado)", r.PRsCreated)
	}
}

func TestMigratePullRequests_CreateError(t *testing.T) {
	source := &fakePRsProvider{
		prs: map[string][]provider.PullRequest{
			"srcorg/repo1": {
				{ExternalID: 1, Title: "Failing PR", State: "open", SourceBranch: "feat/err", TargetBranch: "main"},
			},
		},
	}
	target := &fakePRsProvider{
		prs:       map[string][]provider.PullRequest{"tgtorg/repo1": {}},
		createErr: errors.New("branch not found"),
	}

	plan := core.MigrationPlan{
		Tasks: []core.MigrationTask{
			{ID: "t1", Source: core.TaskEndpoint{Namespace: "srcorg", Repo: "repo1"}, Target: core.TaskEndpoint{Namespace: "tgtorg", Repo: "repo1"}},
		},
	}
	report, err := core.MigratePullRequests(t.Context(), plan, core.PRsMigrateProviders{Source: source, Target: target})
	if err != nil {
		t.Fatalf("MigratePullRequests returned unexpected error: %v", err)
	}
	r := report.Results[0]
	if r.Status != "failed" {
		t.Errorf("Status = %q, want failed", r.Status)
	}
	if len(r.Errors) == 0 {
		t.Error("Errors deve ter ao menos uma entrada")
	}
}
```

- [ ] **Step 2: Rodar para verificar falha**

```bash
go test ./internal/core/ -run TestMigratePullRequests -v
```

Esperado: `FAIL` — `core.MigratePullRequests` não existe.

- [ ] **Step 3: Adicionar tipos em `internal/core/artifacts.go`**

Adicionar após `IssuesReport`:

```go
type PRsMigrateResult struct {
	ID         string   `json:"id"`
	Status     string   `json:"status"` // "success" | "failed"
	PRsCreated int      `json:"prsCreated"`
	Errors     []string `json:"errors,omitempty"`
}

type PRsReport struct {
	GeneratedAt time.Time          `json:"generatedAt"`
	Results     []PRsMigrateResult `json:"results"`
}
```

- [ ] **Step 4: Criar `internal/core/pull_requests.go`**

```go
package core

import (
	"context"
	"fmt"
	"time"

	"github.com/arijunior2020/tfrepo/internal/provider"
	"github.com/arijunior2020/tfrepo/internal/security"
)

// PRsMigrateProviders holds the source and target providers for PR migration.
type PRsMigrateProviders struct {
	Source provider.RepositoryProvider
	Target provider.RepositoryProvider
}

// MigratePullRequests migrates open pull requests from source to target for
// each task in plan. Closed and merged PRs are skipped — they are historical
// artifacts preserved in git history. Existing PRs in the target with the
// same title are skipped (idempotent).
func MigratePullRequests(ctx context.Context, plan MigrationPlan, providers PRsMigrateProviders) (PRsReport, error) {
	results := make([]PRsMigrateResult, 0, len(plan.Tasks))
	for _, task := range plan.Tasks {
		results = append(results, migrateRepoPullRequests(ctx, task, providers))
	}
	return PRsReport{GeneratedAt: time.Now().UTC(), Results: results}, nil
}

func migrateRepoPullRequests(ctx context.Context, task MigrationTask, providers PRsMigrateProviders) PRsMigrateResult {
	result := PRsMigrateResult{ID: task.ID, Status: "success"}

	sourcePRs, err := providers.Source.ListPullRequests(ctx, task.Source.Namespace, task.Source.Repo)
	if err != nil {
		result.Status = "failed"
		result.Errors = append(result.Errors, security.Redact(fmt.Sprintf("list source pull requests: %v", err)))
		return result
	}

	targetPRs, err := providers.Target.ListPullRequests(ctx, task.Target.Namespace, task.Target.Repo)
	if err != nil {
		result.Status = "failed"
		result.Errors = append(result.Errors, security.Redact(fmt.Sprintf("list target pull requests: %v", err)))
		return result
	}

	existing := make(map[string]bool, len(targetPRs))
	for _, pr := range targetPRs {
		existing[pr.Title] = true
	}

	for _, src := range sourcePRs {
		if existing[src.Title] {
			continue
		}
		if _, err := providers.Target.CreatePullRequest(ctx, task.Target.Namespace, task.Target.Repo, src); err != nil {
			result.Status = "failed"
			result.Errors = append(result.Errors, security.Redact(fmt.Sprintf("create pull request %q: %v", src.Title, err)))
			continue
		}
		result.PRsCreated++
	}

	return result
}
```

- [ ] **Step 5: Rodar para verificar que os testes passam**

```bash
go test ./internal/core/ -run TestMigratePullRequests -v
```

Esperado: todos os 3 testes passam.

- [ ] **Step 6: Rodar toda a suite**

```bash
go test ./...
```

Esperado: `ok` em todos os pacotes (exceto falhas pre-existentes de git bareRepo).

- [ ] **Step 7: Commit**

```bash
git add internal/core/artifacts.go internal/core/pull_requests.go internal/core/pull_requests_test.go
git commit -m "feat(core): implement MigratePullRequests with skip-by-title"
```

---

## Task 5: CLI + Destroy + README

**Files:**
- Create: `internal/cli/migrate_prs.go`
- Create: `internal/cli/migrate_prs_test.go`
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/destroy.go`
- Modify: `internal/cli/destroy_test.go`
- Modify: `README.md`

- [ ] **Step 1: Escrever os testes do CLI**

`internal/cli/migrate_prs_test.go`:

```go
package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/core"
	"github.com/arijunior2020/tfrepo/internal/provider"
)

// fakeMigratePRsProvider implementa apenas os métodos usados pelo migrate-prs.
type fakeMigratePRsProvider struct {
	prs       map[string][]provider.PullRequest
	createErr error
}

func (f *fakeMigratePRsProvider) Name() string { return "fake" }
func (f *fakeMigratePRsProvider) ListNamespaces(_ context.Context) ([]provider.Namespace, error) {
	panic("not implemented")
}
func (f *fakeMigratePRsProvider) ListRepositories(_ context.Context, _ string) ([]provider.RepositorySummary, error) {
	panic("not implemented")
}
func (f *fakeMigratePRsProvider) GetRepositoryDetails(_ context.Context, _, _ string) (provider.RepositoryDetails, error) {
	panic("not implemented")
}
func (f *fakeMigratePRsProvider) CreateRepository(_ context.Context, _ string, _ provider.CreateRepositoryInput) (provider.RepositorySummary, error) {
	panic("not implemented")
}
func (f *fakeMigratePRsProvider) GetAuthenticatedCloneURL(_, _ string) string {
	panic("not implemented")
}
func (f *fakeMigratePRsProvider) GetRepositoryState(_ context.Context, _, _ string) (provider.RepositoryState, error) {
	panic("not implemented")
}
func (f *fakeMigratePRsProvider) ListLabels(_ context.Context, _, _ string) ([]provider.Label, error) {
	panic("not implemented")
}
func (f *fakeMigratePRsProvider) CreateLabel(_ context.Context, _, _ string, _ provider.Label) (provider.Label, error) {
	panic("not implemented")
}
func (f *fakeMigratePRsProvider) ListMilestones(_ context.Context, _, _ string) ([]provider.Milestone, error) {
	panic("not implemented")
}
func (f *fakeMigratePRsProvider) CreateMilestone(_ context.Context, _, _ string, _ provider.Milestone) (provider.Milestone, error) {
	panic("not implemented")
}
func (f *fakeMigratePRsProvider) ListIssues(_ context.Context, _, _ string) ([]provider.Issue, error) {
	panic("not implemented")
}
func (f *fakeMigratePRsProvider) CreateIssue(_ context.Context, _, _ string, _ provider.Issue) (provider.Issue, error) {
	panic("not implemented")
}
func (f *fakeMigratePRsProvider) ListPullRequests(_ context.Context, namespace, repo string) ([]provider.PullRequest, error) {
	return f.prs[namespace+"/"+repo], nil
}
func (f *fakeMigratePRsProvider) CreatePullRequest(_ context.Context, _, _ string, pr provider.PullRequest) (provider.PullRequest, error) {
	if f.createErr != nil {
		return provider.PullRequest{}, f.createErr
	}
	pr.ExternalID = 99
	return pr, nil
}

func TestRunMigratePRsWithProviders_WritesReport(t *testing.T) {
	dir := t.TempDir()

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(origDir); err != nil {
			t.Logf("cleanup: failed to restore directory: %v", err)
		}
	})
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	plan := core.MigrationPlan{
		Tasks: []core.MigrationTask{
			{
				ID:     "t1",
				Source: core.TaskEndpoint{Namespace: "src", Repo: "repo"},
				Target: core.TaskEndpoint{Namespace: "tgt", Repo: "repo"},
			},
		},
	}
	if err := core.WriteJSON(migrationPlanPath, plan); err != nil {
		t.Fatal(err)
	}

	src := &fakeMigratePRsProvider{
		prs: map[string][]provider.PullRequest{
			"src/repo": {
				{ExternalID: 1, Title: "PR 1", State: "open", SourceBranch: "feat/1", TargetBranch: "main"},
			},
		},
	}
	tgt := &fakeMigratePRsProvider{
		prs: map[string][]provider.PullRequest{"tgt/repo": {}},
	}

	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"source":{"provider":"github","namespace":"src"},"target":{"provider":"github","namespace":"tgt"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	exitCode := runMigratePRsWithProviders(t.Context(), configPath, &stdout, &stderr, src, tgt)

	if exitCode != 0 {
		t.Errorf("exitCode = %d, want 0; stderr = %s", exitCode, stderr.String())
	}

	reportPath := filepath.Join(dir, prsReportPath)
	if _, err := os.Stat(reportPath); err != nil {
		t.Errorf("%s não foi criado: %v", prsReportPath, err)
	}

	var report core.PRsReport
	if err := core.ReadJSON(prsReportPath, &report); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if len(report.Results) != 1 || report.Results[0].PRsCreated != 1 {
		t.Errorf("Report inesperado: %+v", report.Results)
	}
}
```

- [ ] **Step 2: Rodar para verificar falha**

```bash
go test ./internal/cli/ -run TestRunMigratePRs -v
```

Esperado: `FAIL` — `runMigratePRsWithProviders` não existe.

- [ ] **Step 3: Criar `internal/cli/migrate_prs.go`**

```go
package cli

import (
	"context"
	"fmt"
	"io"
	"os/signal"
	"syscall"

	"github.com/arijunior2020/tfrepo/internal/config"
	"github.com/arijunior2020/tfrepo/internal/core"
	"github.com/arijunior2020/tfrepo/internal/provider"
	"github.com/spf13/cobra"
)

const prsReportPath = "prs-report.json"

func newMigratePRsCommand(exitCode *int) *cobra.Command {
	return &cobra.Command{
		Use:   "migrate-prs",
		Short: "Migra pull requests abertos dos repositórios de origem para o destino",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := cmd.Flags().GetString(configFlagName)
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			*exitCode = runMigratePRs(ctx, configPath, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	}
}

func runMigratePRs(ctx context.Context, configPath string, stdout, stderr io.Writer) int {
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	source, err := NewProvider(cfg.Source)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	target, err := NewProvider(cfg.Target)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return runMigratePRsWithProviders(ctx, configPath, stdout, stderr, source, target)
}

func runMigratePRsWithProviders(ctx context.Context, _ string, stdout, stderr io.Writer, source, target provider.RepositoryProvider) int {
	var plan core.MigrationPlan
	if err := core.ReadJSON(migrationPlanPath, &plan); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	report, err := core.MigratePullRequests(ctx, plan, core.PRsMigrateProviders{Source: source, Target: target})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if err := core.WriteJSON(prsReportPath, report); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	var failed int
	for _, r := range report.Results {
		if r.Status == "failed" {
			failed++
		}
	}

	label := countLabel(len(report.Results), "repositório", "repositórios")
	suffix := ""
	if failed > 0 {
		suffix = fmt.Sprintf(", %d com falha", failed)
	}
	fmt.Fprintf(stdout, "Wrote %s (%s%s)\n", prsReportPath, label, suffix)

	if failed > 0 {
		return 1
	}
	return 0
}
```

- [ ] **Step 4: Adicionar comando em `internal/cli/root.go`**

Localizar a linha com `newMigrateIssuesCommand` e adicionar após:

```go
root.AddCommand(newMigratePRsCommand(exitCode))
```

- [ ] **Step 5: Rodar para verificar que o teste passa**

```bash
go test ./internal/cli/ -run TestRunMigratePRs -v
```

Esperado: `PASS`.

- [ ] **Step 6: Atualizar `internal/cli/destroy.go`**

Adicionar `prsReportPath` após `issuesReportPath`:

```go
var migrationArtifactPaths = []string{
	inventoryPath,
	migrationPlanPath,
	migrationReportPath,
	labelsReportPath,
	issuesReportPath,
	prsReportPath,         // <- nova linha
	validationReportPath,
}
```

- [ ] **Step 7: Atualizar `internal/cli/destroy_test.go`**

Localizar a asserção `"Removidos 6 arquivos."` e substituir por `"Removidos 7 arquivos."`.

- [ ] **Step 8: Verificar testes de destroy**

```bash
go test ./internal/cli/ -run TestDestroy -v
```

Esperado: `PASS`.

- [ ] **Step 9: Atualizar `README.md`**

**9a.** Na lista ordenada do pipeline (dentro da seção "Execute o pipeline"), adicionar `migrate-prs` após `migrate-issues`:

```bash
tfrepo migrate-prs       # migra PRs abertos → prs-report.json
```

**9b.** Adicionar nova seção `### tfrepo migrate-prs` após a seção de `migrate-issues`:

```markdown
### tfrepo migrate-prs

Migra pull requests abertos dos repositórios de origem para o destino.
Apenas PRs abertos são migrados — PRs fechados e mesclados são artefatos históricos preservados no histórico git.
PRs já existentes no destino (por título) são ignorados — a operação é idempotente.

```bash
tfrepo migrate-prs
```
```

**9c.** Adicionar `prs-report.json` na tabela de artefatos (após `issues-report.json`):

```markdown
| `prs-report.json`    | `migrate-prs`     | —                                     |
```

- [ ] **Step 10: Rodar toda a suite**

```bash
go test ./...
```

Esperado: `ok` em todos os pacotes (exceto falhas pre-existentes de git bareRepo).

- [ ] **Step 11: Commit**

```bash
git add internal/cli/migrate_prs.go internal/cli/migrate_prs_test.go
git add internal/cli/root.go internal/cli/destroy.go internal/cli/destroy_test.go
git add README.md
git commit -m "feat(cli): add migrate-prs command, update destroy and README"
```

---

## Self-Review

### Cobertura de spec

| Requisito | Task |
|---|---|
| Tipo neutro `PullRequest` | Task 1 |
| `ListPullRequests` / `CreatePullRequest` na interface | Task 1 |
| Fakes existentes atualizados | Task 1 |
| GitHub: listar apenas abertos (`state="open"`) | Task 2 |
| GitHub: `ExternalID=GetNumber()`, `SourceBranch=Head.Ref`, `TargetBranch=Base.Ref` | Task 2 |
| GitLab: listar apenas abertos (`state="opened"`) | Task 3 |
| GitLab: normalizar "opened"→"open" | Task 3 |
| GitLab: `ExternalID=IID` (não ID global) | Task 3 |
| Skip-by-title (idempotente) | Task 4 |
| `security.Redact` em todos os erros do report | Task 4 |
| `prs-report.json` gerado | Task 5 |
| `destroy` atualizado (7 artefatos) | Task 5 |
| README atualizado | Task 5 |

### Consistência de tipos

- `PullRequest.SourceBranch string` / `TargetBranch string` — usados em Tasks 1–5 ✓
- `PRsMigrateResult.PRsCreated int` — field name consistente com `IssuesCreated` ✓
- `runMigratePRsWithProviders` — assinatura compatível com o teste CLI ✓
- `prsReportPath = "prs-report.json"` — constante definida em `migrate_prs.go`, usada em `destroy.go` ✓
