# tfrepo migrate-issues Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `tfrepo migrate-issues` — migra issues (abertas e fechadas) dos repositórios de origem para o destino, traduzindo referências de milestone via `labels-report.json`, e grava `issues-report.json`.

**Architecture:** Segue exatamente o padrão do Plan 8 (`migrate-labels`): tipo neutro `Issue` em `internal/provider/issues.go`, dois métodos novos na interface `RepositoryProvider`, implementações GitHub/GitLab, função core `MigrateIssues`, e comando Cobra `migrate-issues`. O CLI lê `labels-report.json` (opcional) para obter `MilestoneIDMap` por tarefa e traduzir IDs de milestone antes de criar cada issue no destino.

**Tech Stack:** Go 1.24, `github.com/google/go-github/v74`, `gitlab.com/gitlab-org/api/client-go v1.46.0`, Cobra v1.10.2, `github.com/arijunior2020/tfrepo/internal/security`.

---

## File Map

| Arquivo | Ação |
|---|---|
| `internal/provider/issues.go` | Criar — tipo `Issue` |
| `internal/provider/issues_test.go` | Criar — JSON round-trip |
| `internal/provider/types.go` | Modificar — 2 métodos na interface |
| `internal/provider/github.go` | Modificar — `ListIssues`, `CreateIssue` |
| `internal/provider/github_test.go` | Modificar — 2 novos testes |
| `internal/provider/gitlab.go` | Modificar — `ListIssues`, `CreateIssue` |
| `internal/provider/gitlab_test.go` | Modificar — 2 novos testes |
| `internal/core/artifacts.go` | Modificar — `IssuesMigrateResult`, `IssuesReport` |
| `internal/core/issues.go` | Criar — `MigrateIssues` |
| `internal/core/issues_test.go` | Criar — testes da lógica core |
| `internal/cli/migrate_issues.go` | Criar — comando Cobra |
| `internal/cli/migrate_issues_test.go` | Criar — testes CLI |
| `internal/cli/destroy.go` | Modificar — adicionar `issuesReportPath` |
| `internal/cli/destroy_test.go` | Modificar — 5→6 arquivos |
| `internal/cli/root.go` | Modificar — registrar comando |
| `README.md` | Modificar — pipeline + seção + tabela |
| Todos `*_test.go` com fake providers | Modificar — 2 stubs novos cada |

---

## Contexto crítico para o implementador

### Padrões do projeto (NUNCA violar)

- `*int exitCode` em todos os comandos; signal handling em `newXxxCommand`, NÃO em `runXxx`
- `--config/-c` é flag **persistente** no root. Subcomandos lêem via `cmd.Flags().GetString(configFlagName)` dentro de `RunE` — NUNCA registrar flag local própria
- Erros em structs de relatório SEMPRE passam por `security.Redact(err.Error())` antes de entrar em `Errors []string`
- `Label.Color` sem `#`; `Milestone.ExternalID`: GitHub = `GetNumber()`, GitLab = `milestone.ID`
- `MilestoneIDMap = map[string]int64`; chave = `strconv.FormatInt(id, 10)`
- Mensagens de erro em inglês em todos os providers

### APIs — GitHub Issues

```go
// Listar (com paginação):
page, resp, err := p.client.Issues.ListByRepo(ctx, namespace, repo, &github.IssueListByRepoOptions{
    State:       "all",   // "open" | "closed" | "all"
    ListOptions: github.ListOptions{PerPage: 100},
})
// Filtrar PRs:
if issue.IsPullRequest() { continue }  // PullRequestLinks != nil
// ExternalID:
int64(issue.GetNumber())
// Milestone ref (source ExternalID):
int64(issue.GetMilestone().GetNumber())
// Labels:
for _, l := range issue.Labels { l.GetName() }
// State: issue.GetState() → "open" | "closed"

// Criar:
created, _, err := p.client.Issues.Create(ctx, namespace, repo, &github.IssueRequest{
    Title:  github.Ptr(issue.Title),
    Body:   github.Ptr(issue.Body),
    // Labels: *[]string  (set se len > 0)
    // Milestone: *int  (cast de int64, set se MilestoneExternalID != nil)
})
// Fechar (Create não aceita State — fazer Edit separado):
edited, _, err := p.client.Issues.Edit(ctx, namespace, repo, int(created.GetNumber()), &github.IssueRequest{
    State: github.Ptr("closed"),
})
```

### APIs — GitLab Issues

```go
// Listar (sem State = retorna todos abertos+fechados):
page, resp, err := p.client.Issues.ListProjectIssues(pid, &gitlab.ListProjectIssuesOptions{
    ListOptions: gitlab.ListOptions{PerPage: listPerPage},
    // State omitido → API retorna TODOS (opened + closed)
}, gitlab.WithContext(ctx))
// ExternalID: i.IID (int64 — número do projeto, NÃO i.ID global)
// State: i.State → "opened" | "closed"  → normalizar "opened" → "open"
// Milestone ref: i.Milestone.ID (int64, mesmo ID que ExternalID no Plan 8)
// Labels: i.Labels (type Labels = []string)

// Criar:
created, _, err := p.client.Issues.CreateIssue(pid, &gitlab.CreateIssueOptions{
    Title:       gitlab.Ptr(issue.Title),
    Description: gitlab.Ptr(issue.Body),
    MilestoneID: issue.MilestoneExternalID,  // *int64, nil se sem milestone
    Labels:      &labels,                    // *gitlab.LabelOptions (= *[]string)
}, gitlab.WithContext(ctx))
// Fechar (CreateIssueOptions NÃO tem StateEvent — UpdateIssue separado):
updated, _, err := p.client.Issues.UpdateIssue(pid, created.IID, &gitlab.UpdateIssueOptions{
    StateEvent: gitlab.Ptr("close"),
}, gitlab.WithContext(ctx))
// UpdateIssue recebe IID (int64), não ID global
```

### Helpers de conversão (evitar duplicação)

Definir em cada arquivo de provider:

```go
// em github.go:
func githubIssueToIssue(i *github.Issue) provider.Issue { ... }

// em gitlab.go:
func gitlabIssueToIssue(i *gitlab.Issue) provider.Issue { ... }
```

Reutilizar em `ListIssues` e em `CreateIssue` para construir o retorno.

### Fakes em arquivos de teste

Após adicionar os 2 métodos à interface, `go build ./...` falhará com erros de compilação em todos os arquivos de teste que têm fakes. Encontrar todos com:

```bash
go build ./... 2>&1 | grep "does not implement"
```

Adicionar stubs `panic("not implemented")` em cada fake encontrado — ver exemplo na Task 1.

---

## Task 1: Tipo `Issue` + Interface + Stubs

**Files:**
- Create: `internal/provider/issues.go`
- Create: `internal/provider/issues_test.go`
- Modify: `internal/provider/types.go`
- Modify: todos os arquivos `*_test.go` que falham em `go build ./...`

- [ ] **Step 1: Escrever o teste de JSON round-trip**

`internal/provider/issues_test.go`:
```go
package provider_test

import (
	"encoding/json"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/provider"
)

func TestIssueJSONRoundTrip(t *testing.T) {
	milestoneID := int64(42)
	orig := provider.Issue{
		ExternalID:          int64(7),
		Title:               "Fix segfault",
		Body:                "Reproduces on Linux",
		State:               "closed",
		Labels:              []string{"bug", "priority"},
		MilestoneExternalID: &milestoneID,
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got provider.Issue
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.ExternalID != orig.ExternalID {
		t.Errorf("ExternalID = %d, want %d", got.ExternalID, orig.ExternalID)
	}
	if got.Title != orig.Title {
		t.Errorf("Title = %q, want %q", got.Title, orig.Title)
	}
	if got.State != orig.State {
		t.Errorf("State = %q, want %q", got.State, orig.State)
	}
	if len(got.Labels) != 2 || got.Labels[0] != "bug" {
		t.Errorf("Labels = %v, want [bug priority]", got.Labels)
	}
	if got.MilestoneExternalID == nil || *got.MilestoneExternalID != 42 {
		t.Errorf("MilestoneExternalID = %v, want &42", got.MilestoneExternalID)
	}
}

func TestIssueJSONNoMilestone(t *testing.T) {
	orig := provider.Issue{
		ExternalID: int64(1),
		Title:      "No milestone",
		State:      "open",
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got provider.Issue
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.MilestoneExternalID != nil {
		t.Errorf("MilestoneExternalID = %v, want nil", got.MilestoneExternalID)
	}
}
```

- [ ] **Step 2: Rodar para verificar falha**

```bash
cd /media/arimateia-junior/Dados4/projetos/tfrepo
go test ./internal/provider/ -run TestIssueJSON -v
```

Esperado: `FAIL` — `provider.Issue` não existe.

- [ ] **Step 3: Criar `internal/provider/issues.go`**

```go
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
```

- [ ] **Step 4: Rodar para verificar que testes passam**

```bash
go test ./internal/provider/ -run TestIssueJSON -v
```

Esperado: `PASS`.

- [ ] **Step 5: Adicionar métodos à interface em `internal/provider/types.go`**

Localizar o bloco de labels/milestones (adicionado no Plan 8) e adicionar após:

```go
	// Issues (tfrepo migrate-issues)
	ListIssues(ctx context.Context, namespace, repo string) ([]Issue, error)
	CreateIssue(ctx context.Context, namespace, repo string, issue Issue) (Issue, error)
```

- [ ] **Step 6: Encontrar todos os fakes quebrados**

```bash
go build ./... 2>&1 | grep "does not implement\|missing method"
```

Isso lista os arquivos e structs que precisam de stubs. Tipicamente são fakes em:
- `internal/core/migrate_test.go`
- `internal/core/labels_test.go`
- `internal/core/scan_test.go`
- `internal/core/validate_test.go`
- `internal/cli/migrate_test.go`
- `internal/cli/scan_test.go`
- `internal/cli/validate_test.go`

- [ ] **Step 7: Adicionar stubs em cada fake encontrado**

Para cada fake provider que falha, adicionar (substituindo `fakeXxxProvider` pelo nome real do struct):

```go
func (f *fakeXxxProvider) ListIssues(_ context.Context, _, _ string) ([]provider.Issue, error) {
	panic("not implemented")
}

func (f *fakeXxxProvider) CreateIssue(_ context.Context, _, _ string, _ provider.Issue) (provider.Issue, error) {
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
git add internal/provider/issues.go internal/provider/issues_test.go internal/provider/types.go
git add internal/core/*_test.go internal/cli/*_test.go
git commit -m "feat(provider): add Issue type and ListIssues/CreateIssue interface methods"
```

---

## Task 2: Implementação GitHub Provider

**Files:**
- Modify: `internal/provider/github.go`
- Modify: `internal/provider/github_test.go`

- [ ] **Step 1: Escrever o teste de `ListIssues`**

Adicionar em `internal/provider/github_test.go` (seguindo o padrão httptest existente):

```go
func TestGitHubProviderListIssues(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Page 1: issue aberta + PR (deve ser filtrado)
	mux.HandleFunc("/repos/myorg/myrepo/issues", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != "all" {
			t.Errorf("state = %q, want %q", r.URL.Query().Get("state"), "all")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]*github.Issue{
			{
				Number: github.Ptr(1),
				Title:  github.Ptr("Real issue"),
				Body:   github.Ptr("body text"),
				State:  github.Ptr("open"),
				Labels: []*github.Label{
					{Name: github.Ptr("bug")},
				},
			},
			{
				Number: github.Ptr(2),
				Title:  github.Ptr("A pull request"),
				State:  github.Ptr("open"),
				PullRequestLinks: &github.PullRequestLinks{
					URL: github.Ptr("https://example.com/pulls/2"),
				},
			},
		})
	})

	p := newTestGitHubProvider(t, srv.URL)
	issues, err := p.ListIssues(t.Context(), "myorg", "myrepo")
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("len(issues) = %d, want 1 (PR deve ser filtrado)", len(issues))
	}
	got := issues[0]
	if got.ExternalID != 1 {
		t.Errorf("ExternalID = %d, want 1", got.ExternalID)
	}
	if got.Title != "Real issue" {
		t.Errorf("Title = %q, want %q", got.Title, "Real issue")
	}
	if got.State != "open" {
		t.Errorf("State = %q, want open", got.State)
	}
	if len(got.Labels) != 1 || got.Labels[0] != "bug" {
		t.Errorf("Labels = %v, want [bug]", got.Labels)
	}
}

func TestGitHubProviderCreateIssue(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	var editCalled bool
	mux.HandleFunc("/repos/myorg/myrepo/issues", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(&github.Issue{
			Number: github.Ptr(10),
			Title:  github.Ptr("Created issue"),
			State:  github.Ptr("open"),
		})
	})
	mux.HandleFunc("/repos/myorg/myrepo/issues/10", func(w http.ResponseWriter, r *http.Request) {
		editCalled = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(&github.Issue{
			Number: github.Ptr(10),
			Title:  github.Ptr("Created issue"),
			State:  github.Ptr("closed"),
		})
	})

	p := newTestGitHubProvider(t, srv.URL)
	result, err := p.CreateIssue(t.Context(), "myorg", "myrepo", provider.Issue{
		Title:  "Created issue",
		Body:   "body",
		State:  "closed",
		Labels: []string{"bug"},
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if !editCalled {
		t.Error("Edit not called for closed issue")
	}
	if result.State != "closed" {
		t.Errorf("State = %q, want closed", result.State)
	}
	if result.ExternalID != 10 {
		t.Errorf("ExternalID = %d, want 10", result.ExternalID)
	}
}
```

- [ ] **Step 2: Rodar para verificar falha**

```bash
go test ./internal/provider/ -run TestGitHubProviderListIssues -v
go test ./internal/provider/ -run TestGitHubProviderCreateIssue -v
```

Esperado: `FAIL` — métodos não implementados.

- [ ] **Step 3: Implementar em `internal/provider/github.go`**

Adicionar helper privado e os dois métodos:

```go
func githubIssueToIssue(i *github.Issue) Issue {
	iss := Issue{
		ExternalID: int64(i.GetNumber()),
		Title:      i.GetTitle(),
		Body:       i.GetBody(),
		State:      i.GetState(), // "open" | "closed"
	}
	for _, l := range i.Labels {
		iss.Labels = append(iss.Labels, l.GetName())
	}
	if m := i.GetMilestone(); m != nil {
		id := int64(m.GetNumber())
		iss.MilestoneExternalID = &id
	}
	return iss
}

func (p *GitHubProvider) ListIssues(ctx context.Context, namespace, repo string) ([]Issue, error) {
	var issues []Issue
	opts := &github.IssueListByRepoOptions{
		State:       "all",
		ListOptions: github.ListOptions{PerPage: 100},
	}
	for {
		page, resp, err := p.client.Issues.ListByRepo(ctx, namespace, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("list issues for %s/%s: %w", namespace, repo, err)
		}
		for _, i := range page {
			if i.IsPullRequest() {
				continue
			}
			issues = append(issues, githubIssueToIssue(i))
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return issues, nil
}

func (p *GitHubProvider) CreateIssue(ctx context.Context, namespace, repo string, issue Issue) (Issue, error) {
	req := &github.IssueRequest{
		Title: github.Ptr(issue.Title),
		Body:  github.Ptr(issue.Body),
	}
	if len(issue.Labels) > 0 {
		l := issue.Labels
		req.Labels = &l
	}
	if issue.MilestoneExternalID != nil {
		m := int(*issue.MilestoneExternalID)
		req.Milestone = &m
	}
	created, _, err := p.client.Issues.Create(ctx, namespace, repo, req)
	if err != nil {
		return Issue{}, fmt.Errorf("create issue %q in %s/%s: %w", issue.Title, namespace, repo, err)
	}
	if issue.State == "closed" {
		edited, _, err := p.client.Issues.Edit(ctx, namespace, repo, created.GetNumber(), &github.IssueRequest{
			State: github.Ptr("closed"),
		})
		if err != nil {
			return Issue{}, fmt.Errorf("close issue %q in %s/%s: %w", issue.Title, namespace, repo, err)
		}
		return githubIssueToIssue(edited), nil
	}
	return githubIssueToIssue(created), nil
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

Esperado: `ok` em todos os pacotes.

- [ ] **Step 6: Commit**

```bash
git add internal/provider/github.go internal/provider/github_test.go
git commit -m "feat(provider/github): implement ListIssues and CreateIssue"
```

---

## Task 3: Implementação GitLab Provider

**Files:**
- Modify: `internal/provider/gitlab.go`
- Modify: `internal/provider/gitlab_test.go`

- [ ] **Step 1: Escrever o teste de `ListIssues`**

Adicionar em `internal/provider/gitlab_test.go`:

```go
func TestGitLabProviderListIssues(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/api/v4/projects/mygroup%2Fmyrepo/issues", func(w http.ResponseWriter, r *http.Request) {
		// state omitido = retorna todos
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]*gitlab.Issue{
			{
				IID:         1,
				Title:       "Open issue",
				Description: "desc",
				State:       "opened",
				Labels:      gitlab.Labels{"enhancement"},
			},
			{
				IID:   2,
				Title: "Closed issue",
				State: "closed",
			},
		})
	})

	p := newTestGitLabProvider(t, srv.URL)
	issues, err := p.ListIssues(t.Context(), "mygroup", "myrepo")
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("len(issues) = %d, want 2", len(issues))
	}
	if issues[0].State != "open" {
		t.Errorf("issues[0].State = %q, want open (deve normalizar 'opened')", issues[0].State)
	}
	if issues[1].State != "closed" {
		t.Errorf("issues[1].State = %q, want closed", issues[1].State)
	}
	if issues[0].ExternalID != 1 {
		t.Errorf("ExternalID = %d, want 1 (IID, não ID global)", issues[0].ExternalID)
	}
	if len(issues[0].Labels) != 1 || issues[0].Labels[0] != "enhancement" {
		t.Errorf("Labels = %v, want [enhancement]", issues[0].Labels)
	}
}

func TestGitLabProviderCreateIssue(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	var updateCalled bool
	mux.HandleFunc("/api/v4/projects/mygroup%2Fmyrepo/issues", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(&gitlab.Issue{
			IID:   5,
			Title: "New issue",
			State: "opened",
		})
	})
	mux.HandleFunc("/api/v4/projects/mygroup%2Fmyrepo/issues/5", func(w http.ResponseWriter, r *http.Request) {
		updateCalled = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(&gitlab.Issue{
			IID:   5,
			Title: "New issue",
			State: "closed",
		})
	})

	p := newTestGitLabProvider(t, srv.URL)
	result, err := p.CreateIssue(t.Context(), "mygroup", "myrepo", provider.Issue{
		Title: "New issue",
		Body:  "body",
		State: "closed",
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if !updateCalled {
		t.Error("UpdateIssue not called for closed issue")
	}
	if result.State != "closed" {
		t.Errorf("State = %q, want closed", result.State)
	}
	if result.ExternalID != 5 {
		t.Errorf("ExternalID = %d, want 5", result.ExternalID)
	}
}
```

- [ ] **Step 2: Rodar para verificar falha**

```bash
go test ./internal/provider/ -run TestGitLabProviderListIssues -v
go test ./internal/provider/ -run TestGitLabProviderCreateIssue -v
```

Esperado: `FAIL` — métodos não implementados.

- [ ] **Step 3: Implementar em `internal/provider/gitlab.go`**

Adicionar helper privado e os dois métodos:

```go
func gitlabIssueToIssue(i *gitlab.Issue) Issue {
	iss := Issue{
		ExternalID: i.IID,
		Title:      i.Title,
		Body:       i.Description,
		State:      "open",
	}
	if i.State == "closed" {
		iss.State = "closed"
	}
	for _, l := range i.Labels {
		iss.Labels = append(iss.Labels, l)
	}
	if i.Milestone != nil {
		id := i.Milestone.ID
		iss.MilestoneExternalID = &id
	}
	return iss
}

func (p *GitLabProvider) ListIssues(ctx context.Context, namespace, repo string) ([]Issue, error) {
	pid := gitlabPID(namespace, repo)
	var issues []Issue
	opts := &gitlab.ListProjectIssuesOptions{
		ListOptions: gitlab.ListOptions{PerPage: listPerPage},
		// State omitido: GitLab API retorna todos (opened + closed)
	}
	for {
		page, resp, err := p.client.Issues.ListProjectIssues(pid, opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list issues for %s: %w", pid, err)
		}
		for _, i := range page {
			issues = append(issues, gitlabIssueToIssue(i))
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return issues, nil
}

func (p *GitLabProvider) CreateIssue(ctx context.Context, namespace, repo string, issue Issue) (Issue, error) {
	pid := gitlabPID(namespace, repo)
	opts := &gitlab.CreateIssueOptions{
		Title:       gitlab.Ptr(issue.Title),
		Description: gitlab.Ptr(issue.Body),
	}
	if len(issue.Labels) > 0 {
		labels := gitlab.LabelOptions(issue.Labels)
		opts.Labels = &labels
	}
	if issue.MilestoneExternalID != nil {
		opts.MilestoneID = issue.MilestoneExternalID
	}
	created, _, err := p.client.Issues.CreateIssue(pid, opts, gitlab.WithContext(ctx))
	if err != nil {
		return Issue{}, fmt.Errorf("create issue %q in %s: %w", issue.Title, pid, err)
	}
	if issue.State == "closed" {
		updated, _, err := p.client.Issues.UpdateIssue(pid, created.IID, &gitlab.UpdateIssueOptions{
			StateEvent: gitlab.Ptr("close"),
		}, gitlab.WithContext(ctx))
		if err != nil {
			return Issue{}, fmt.Errorf("close issue %q in %s: %w", issue.Title, pid, err)
		}
		return gitlabIssueToIssue(updated), nil
	}
	return gitlabIssueToIssue(created), nil
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

Esperado: `ok` em todos os pacotes.

- [ ] **Step 6: Commit**

```bash
git add internal/provider/gitlab.go internal/provider/gitlab_test.go
git commit -m "feat(provider/gitlab): implement ListIssues and CreateIssue"
```

---

## Task 4: Core Logic

**Files:**
- Modify: `internal/core/artifacts.go`
- Create: `internal/core/issues.go`
- Create: `internal/core/issues_test.go`

### Contexto: `MilestoneIDMap` e tradução

`MilestoneIDMap = map[string]int64` onde chave = `strconv.FormatInt(sourceExternalID, 10)`.

Antes de chamar `CreateIssue`, a função core traduz `issue.MilestoneExternalID` de "ID no source" para "ID no target" usando o mapa. Se não encontrar o mapa (ou se a issue não tiver milestone), mantém nil.

### Contexto: skip-by-title

Para cada tarefa: listar issues existentes no target, construir `map[string]bool` por título. Issues do source cujo título já existe no target são ignoradas.

- [ ] **Step 1: Escrever os testes**

`internal/core/issues_test.go`:

```go
package core_test

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/core"
	"github.com/arijunior2020/tfrepo/internal/provider"
)

// fakeIssuesProvider implementa provider.RepositoryProvider para testes de issues.
// Só os métodos ListIssues e CreateIssue são relevantes; os demais entram em panic.
type fakeIssuesProvider struct {
	issues       map[string][]provider.Issue // chave: "namespace/repo"
	createErr    error
	created      []provider.Issue
}

func (f *fakeIssuesProvider) Name() string { return "fake" }

func (f *fakeIssuesProvider) ListNamespaces(_ context.Context) ([]provider.Namespace, error) {
	panic("not implemented")
}
func (f *fakeIssuesProvider) GetRepository(_ context.Context, _, _ string) (provider.RepositoryDetails, error) {
	panic("not implemented")
}
func (f *fakeIssuesProvider) ListRepositories(_ context.Context, _ string) ([]provider.RepositoryDetails, error) {
	panic("not implemented")
}
func (f *fakeIssuesProvider) CreateRepository(_ context.Context, _ string, _ provider.RepositoryDetails) (provider.RepositoryDetails, error) {
	panic("not implemented")
}
func (f *fakeIssuesProvider) ListBranches(_ context.Context, _, _ string) ([]string, error) {
	panic("not implemented")
}
func (f *fakeIssuesProvider) ListTags(_ context.Context, _, _ string) ([]string, error) {
	panic("not implemented")
}
func (f *fakeIssuesProvider) CloneURL(_ context.Context, _, _ string) (string, error) {
	panic("not implemented")
}
func (f *fakeIssuesProvider) ListLabels(_ context.Context, _, _ string) ([]provider.Label, error) {
	panic("not implemented")
}
func (f *fakeIssuesProvider) CreateLabel(_ context.Context, _, _ string, _ provider.Label) (provider.Label, error) {
	panic("not implemented")
}
func (f *fakeIssuesProvider) ListMilestones(_ context.Context, _, _ string) ([]provider.Milestone, error) {
	panic("not implemented")
}
func (f *fakeIssuesProvider) CreateMilestone(_ context.Context, _, _ string, _ provider.Milestone) (provider.Milestone, error) {
	panic("not implemented")
}
func (f *fakeIssuesProvider) ListIssues(_ context.Context, namespace, repo string) ([]provider.Issue, error) {
	return f.issues[namespace+"/"+repo], nil
}
func (f *fakeIssuesProvider) CreateIssue(_ context.Context, _, _ string, issue provider.Issue) (provider.Issue, error) {
	if f.createErr != nil {
		return provider.Issue{}, f.createErr
	}
	created := issue
	created.ExternalID = int64(len(f.created) + 100)
	f.created = append(f.created, created)
	return created, nil
}

func makePlan(tasks ...core.MigrationTask) core.MigrationPlan {
	return core.MigrationPlan{Tasks: tasks}
}

func makeTask(id, srcNS, srcRepo, tgtNS, tgtRepo string) core.MigrationTask {
	return core.MigrationTask{
		ID:     id,
		Source: core.TaskEndpoint{Namespace: srcNS, Repo: srcRepo},
		Target: core.TaskEndpoint{Namespace: tgtNS, Repo: tgtRepo},
	}
}

func TestMigrateIssues_HappyPath(t *testing.T) {
	milestoneID := int64(10)
	source := &fakeIssuesProvider{
		issues: map[string][]provider.Issue{
			"srcorg/repo1": {
				{ExternalID: 1, Title: "Issue A", State: "open", Labels: []string{"bug"}},
				{ExternalID: 2, Title: "Issue B", State: "closed", MilestoneExternalID: &milestoneID},
			},
		},
	}
	target := &fakeIssuesProvider{
		issues: map[string][]provider.Issue{
			"tgtorg/repo1": {}, // vazio
		},
	}

	milestoneMap := core.MilestoneIDMap{
		strconv.FormatInt(10, 10): int64(99),
	}

	plan := makePlan(makeTask("t1", "srcorg", "repo1", "tgtorg", "repo1"))
	report, err := core.MigrateIssues(t.Context(), plan,
		core.IssuesMigrateProviders{Source: source, Target: target},
		map[string]core.MilestoneIDMap{"t1": milestoneMap})
	if err != nil {
		t.Fatalf("MigrateIssues: %v", err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(report.Results))
	}
	r := report.Results[0]
	if r.Status != "success" {
		t.Errorf("Status = %q, want success", r.Status)
	}
	if r.IssuesCreated != 2 {
		t.Errorf("IssuesCreated = %d, want 2", r.IssuesCreated)
	}
	// Verificar que MilestoneExternalID foi traduzido de 10 → 99
	if target.created[1].MilestoneExternalID == nil || *target.created[1].MilestoneExternalID != 99 {
		t.Errorf("MilestoneExternalID = %v, want &99 (traduzido via MilestoneIDMap)", target.created[1].MilestoneExternalID)
	}
}

func TestMigrateIssues_SkipsExistingByTitle(t *testing.T) {
	source := &fakeIssuesProvider{
		issues: map[string][]provider.Issue{
			"srcorg/repo1": {
				{ExternalID: 1, Title: "Already there", State: "open"},
				{ExternalID: 2, Title: "New issue", State: "open"},
			},
		},
	}
	target := &fakeIssuesProvider{
		issues: map[string][]provider.Issue{
			"tgtorg/repo1": {
				{ExternalID: 50, Title: "Already there", State: "open"},
			},
		},
	}

	plan := makePlan(makeTask("t1", "srcorg", "repo1", "tgtorg", "repo1"))
	report, err := core.MigrateIssues(t.Context(), plan,
		core.IssuesMigrateProviders{Source: source, Target: target},
		nil)
	if err != nil {
		t.Fatalf("MigrateIssues: %v", err)
	}
	r := report.Results[0]
	if r.IssuesCreated != 1 {
		t.Errorf("IssuesCreated = %d, want 1 (issue existente deve ser ignorada)", r.IssuesCreated)
	}
}

func TestMigrateIssues_CreateError(t *testing.T) {
	source := &fakeIssuesProvider{
		issues: map[string][]provider.Issue{
			"srcorg/repo1": {
				{ExternalID: 1, Title: "Failing issue", State: "open"},
			},
		},
	}
	target := &fakeIssuesProvider{
		issues:    map[string][]provider.Issue{"tgtorg/repo1": {}},
		createErr: errors.New("api error"),
	}

	plan := makePlan(makeTask("t1", "srcorg", "repo1", "tgtorg", "repo1"))
	report, err := core.MigrateIssues(t.Context(), plan,
		core.IssuesMigrateProviders{Source: source, Target: target},
		nil)
	if err != nil {
		t.Fatalf("MigrateIssues returned unexpected error: %v", err)
	}
	r := report.Results[0]
	if r.Status != "failed" {
		t.Errorf("Status = %q, want failed", r.Status)
	}
	if len(r.Errors) == 0 {
		t.Error("Errors deve ter ao menos uma entrada")
	}
}

func TestMigrateIssues_NoMilestoneMap(t *testing.T) {
	milestoneID := int64(10)
	source := &fakeIssuesProvider{
		issues: map[string][]provider.Issue{
			"srcorg/repo1": {
				{ExternalID: 1, Title: "Issue", State: "open", MilestoneExternalID: &milestoneID},
			},
		},
	}
	target := &fakeIssuesProvider{
		issues: map[string][]provider.Issue{"tgtorg/repo1": {}},
	}

	plan := makePlan(makeTask("t1", "srcorg", "repo1", "tgtorg", "repo1"))
	report, err := core.MigrateIssues(t.Context(), plan,
		core.IssuesMigrateProviders{Source: source, Target: target},
		nil) // sem mapa → milestone deve ficar nil no target
	if err != nil {
		t.Fatalf("MigrateIssues: %v", err)
	}
	if len(report.Results) != 1 || report.Results[0].Status != "success" {
		t.Errorf("Result inesperado: %+v", report.Results)
	}
	// Milestone não traduzido → não deve ser passado ao Create
	if target.created[0].MilestoneExternalID != nil {
		t.Errorf("MilestoneExternalID = %v, want nil (sem mapa de tradução)", target.created[0].MilestoneExternalID)
	}
}
```

- [ ] **Step 2: Rodar para verificar falha**

```bash
go test ./internal/core/ -run TestMigrateIssues -v
```

Esperado: `FAIL` — `core.MigrateIssues` não existe.

- [ ] **Step 3: Adicionar tipos em `internal/core/artifacts.go`**

Adicionar após `LabelsReport`:

```go
type IssuesMigrateResult struct {
	ID            string   `json:"id"`
	Status        string   `json:"status"` // "success" | "failed"
	IssuesCreated int      `json:"issuesCreated"`
	Errors        []string `json:"errors,omitempty"`
}

type IssuesReport struct {
	GeneratedAt time.Time              `json:"generatedAt"`
	Results     []IssuesMigrateResult  `json:"results"`
}
```

- [ ] **Step 4: Criar `internal/core/issues.go`**

```go
package core

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/arijunior2020/tfrepo/internal/provider"
	"github.com/arijunior2020/tfrepo/internal/security"
)

// IssuesMigrateProviders holds the source and target providers for issue migration.
type IssuesMigrateProviders struct {
	Source provider.RepositoryProvider
	Target provider.RepositoryProvider
}

// MigrateIssues migrates issues from source to target for each task in plan.
// milestoneMapsByTask maps task ID to MilestoneIDMap for milestone ID translation.
// Pass nil to skip milestone translation entirely.
func MigrateIssues(ctx context.Context, plan MigrationPlan, providers IssuesMigrateProviders, milestoneMapsByTask map[string]MilestoneIDMap) (IssuesReport, error) {
	results := make([]IssuesMigrateResult, 0, len(plan.Tasks))
	for _, task := range plan.Tasks {
		results = append(results, migrateRepoIssues(ctx, task, providers, milestoneMapsByTask[task.ID]))
	}
	return IssuesReport{GeneratedAt: time.Now().UTC(), Results: results}, nil
}

func migrateRepoIssues(ctx context.Context, task MigrationTask, providers IssuesMigrateProviders, milestoneMap MilestoneIDMap) IssuesMigrateResult {
	result := IssuesMigrateResult{ID: task.ID, Status: "success"}

	sourceIssues, err := providers.Source.ListIssues(ctx, task.Source.Namespace, task.Source.Repo)
	if err != nil {
		result.Status = "failed"
		result.Errors = append(result.Errors, security.Redact(fmt.Sprintf("list source issues: %v", err)))
		return result
	}

	targetIssues, err := providers.Target.ListIssues(ctx, task.Target.Namespace, task.Target.Repo)
	if err != nil {
		result.Status = "failed"
		result.Errors = append(result.Errors, security.Redact(fmt.Sprintf("list target issues: %v", err)))
		return result
	}

	existing := make(map[string]bool, len(targetIssues))
	for _, i := range targetIssues {
		existing[i.Title] = true
	}

	for _, src := range sourceIssues {
		if existing[src.Title] {
			continue
		}
		toCreate := src
		toCreate.MilestoneExternalID = translateMilestone(src.MilestoneExternalID, milestoneMap)

		if _, err := providers.Target.CreateIssue(ctx, task.Target.Namespace, task.Target.Repo, toCreate); err != nil {
			result.Status = "failed"
			result.Errors = append(result.Errors, security.Redact(fmt.Sprintf("create issue %q: %v", src.Title, err)))
			continue
		}
		result.IssuesCreated++
	}

	return result
}

// translateMilestone looks up sourceID in milestoneMap and returns the target ID.
// Returns nil if sourceID is nil, milestoneMap is nil, or the key is not found.
func translateMilestone(sourceID *int64, milestoneMap MilestoneIDMap) *int64 {
	if sourceID == nil || len(milestoneMap) == 0 {
		return nil
	}
	key := strconv.FormatInt(*sourceID, 10)
	targetID, ok := milestoneMap[key]
	if !ok {
		return nil
	}
	return &targetID
}
```

- [ ] **Step 5: Rodar para verificar que os testes passam**

```bash
go test ./internal/core/ -run TestMigrateIssues -v
```

Esperado: todos os 4 testes passam.

- [ ] **Step 6: Rodar toda a suite**

```bash
go test ./...
```

Esperado: `ok` em todos os pacotes.

- [ ] **Step 7: Commit**

```bash
git add internal/core/artifacts.go internal/core/issues.go internal/core/issues_test.go
git commit -m "feat(core): implement MigrateIssues with milestone translation and skip-by-title"
```

---

## Task 5: CLI + Destroy + README

**Files:**
- Create: `internal/cli/migrate_issues.go`
- Create: `internal/cli/migrate_issues_test.go`
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/destroy.go`
- Modify: `internal/cli/destroy_test.go`
- Modify: `README.md`

### Contexto: leitura de `labels-report.json`

O CLI lê `labels-report.json` com `os.ErrNotExist` tratado silenciosamente. Se o arquivo não existir, `milestoneMapsByTask` fica vazio e milestones não são traduzidos.

- [ ] **Step 1: Escrever os testes do CLI**

`internal/cli/migrate_issues_test.go`:

```go
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/core"
	"github.com/arijunior2020/tfrepo/internal/provider"
)

// fakeMigrateIssuesProvider implementa apenas os métodos usados pelo migrate-issues.
type fakeMigrateIssuesProvider struct {
	issues    map[string][]provider.Issue
	createErr error
}

func (f *fakeMigrateIssuesProvider) Name() string { return "fake" }
func (f *fakeMigrateIssuesProvider) ListNamespaces(_ context.Context) ([]provider.Namespace, error) {
	panic("not implemented")
}
func (f *fakeMigrateIssuesProvider) GetRepository(_ context.Context, _, _ string) (provider.RepositoryDetails, error) {
	panic("not implemented")
}
func (f *fakeMigrateIssuesProvider) ListRepositories(_ context.Context, _ string) ([]provider.RepositoryDetails, error) {
	panic("not implemented")
}
func (f *fakeMigrateIssuesProvider) CreateRepository(_ context.Context, _ string, _ provider.RepositoryDetails) (provider.RepositoryDetails, error) {
	panic("not implemented")
}
func (f *fakeMigrateIssuesProvider) ListBranches(_ context.Context, _, _ string) ([]string, error) {
	panic("not implemented")
}
func (f *fakeMigrateIssuesProvider) ListTags(_ context.Context, _, _ string) ([]string, error) {
	panic("not implemented")
}
func (f *fakeMigrateIssuesProvider) CloneURL(_ context.Context, _, _ string) (string, error) {
	panic("not implemented")
}
func (f *fakeMigrateIssuesProvider) ListLabels(_ context.Context, _, _ string) ([]provider.Label, error) {
	panic("not implemented")
}
func (f *fakeMigrateIssuesProvider) CreateLabel(_ context.Context, _, _ string, _ provider.Label) (provider.Label, error) {
	panic("not implemented")
}
func (f *fakeMigrateIssuesProvider) ListMilestones(_ context.Context, _, _ string) ([]provider.Milestone, error) {
	panic("not implemented")
}
func (f *fakeMigrateIssuesProvider) CreateMilestone(_ context.Context, _, _ string, _ provider.Milestone) (provider.Milestone, error) {
	panic("not implemented")
}
func (f *fakeMigrateIssuesProvider) ListIssues(_ context.Context, namespace, repo string) ([]provider.Issue, error) {
	return f.issues[namespace+"/"+repo], nil
}
func (f *fakeMigrateIssuesProvider) CreateIssue(_ context.Context, _, _ string, issue provider.Issue) (provider.Issue, error) {
	if f.createErr != nil {
		return provider.Issue{}, f.createErr
	}
	issue.ExternalID = 99
	return issue, nil
}

func TestRunMigrateIssues_Success(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	planPath := filepath.Join(dir, migrationPlanPath)
	issuesReportPath := filepath.Join(dir, issuesReportPath) //nolint:govet

	// Mudar para o diretório temporário para que os artefatos sejam escritos lá
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	os.Chdir(dir)

	// Escrever config mínima
	if err := os.WriteFile(configPath, []byte(`{"source":{"provider":"github","namespace":"src"},"target":{"provider":"github","namespace":"tgt"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Escrever migration-plan.json
	plan := core.MigrationPlan{
		Tasks: []core.MigrationTask{
			{ID: "t1", Source: core.TaskEndpoint{Namespace: "src", Repo: "repo"}, Target: core.TaskEndpoint{Namespace: "tgt", Repo: "repo"}},
		},
	}
	if err := core.WriteJSON(migrationPlanPath, plan); err != nil {
		t.Fatal(err)
	}

	src := &fakeMigrateIssuesProvider{
		issues: map[string][]provider.Issue{
			"src/repo": {{ExternalID: 1, Title: "Issue 1", State: "open"}},
		},
	}
	tgt := &fakeMigrateIssuesProvider{
		issues: map[string][]provider.Issue{"tgt/repo": {}},
	}

	var stdout, stderr bytes.Buffer
	exitCode := runMigrateIssuesWithProviders(t.Context(), configPath, &stdout, &stderr, src, tgt)

	if exitCode != 0 {
		t.Errorf("exitCode = %d, want 0; stderr = %s", exitCode, stderr.String())
	}
	if !fileExists(issuesReportPath) {
		t.Errorf("%s não foi criado", issuesReportPath)
	}
	var report core.IssuesReport
	if err := core.ReadJSON(issuesReportPath, &report); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if len(report.Results) != 1 || report.Results[0].IssuesCreated != 1 {
		t.Errorf("Report inesperado: %+v", report.Results)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
```

- [ ] **Step 2: Rodar para verificar falha**

```bash
go test ./internal/cli/ -run TestRunMigrateIssues -v
```

Esperado: `FAIL` — `runMigrateIssuesWithProviders` não existe.

- [ ] **Step 3: Criar `internal/cli/migrate_issues.go`**

```go
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/arijunior2020/tfrepo/internal/config"
	"github.com/arijunior2020/tfrepo/internal/core"
	"github.com/arijunior2020/tfrepo/internal/provider"
	"github.com/spf13/cobra"
)

const issuesReportPath = "issues-report.json"

func newMigrateIssuesCommand(exitCode *int) *cobra.Command {
	return &cobra.Command{
		Use:   "migrate-issues",
		Short: "Migra issues dos repositórios de origem para o destino",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := cmd.Flags().GetString(configFlagName)
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			*exitCode = runMigrateIssues(ctx, configPath, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	}
}

func runMigrateIssues(ctx context.Context, configPath string, stdout, stderr io.Writer) int {
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
	return runMigrateIssuesWithProviders(ctx, configPath, stdout, stderr, source, target)
}

func runMigrateIssuesWithProviders(ctx context.Context, configPath string, stdout, stderr io.Writer, source, target provider.RepositoryProvider) int {
	var plan core.MigrationPlan
	if err := core.ReadJSON(migrationPlanPath, &plan); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	milestoneMapsByTask := buildMilestoneMapsByTask()

	report, err := core.MigrateIssues(ctx, plan, core.IssuesMigrateProviders{Source: source, Target: target}, milestoneMapsByTask)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if err := core.WriteJSON(issuesReportPath, report); err != nil {
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
	fmt.Fprintf(stdout, "Wrote %s (%s%s)\n", issuesReportPath, label, suffix)

	if failed > 0 {
		return 1
	}
	return 0
}

// buildMilestoneMapsByTask reads labels-report.json (if it exists) and returns
// a map from task ID to MilestoneIDMap for milestone ID translation.
// Missing file is silently ignored (no milestone translation).
func buildMilestoneMapsByTask() map[string]core.MilestoneIDMap {
	var labelsReport core.LabelsReport
	if err := core.ReadJSON(labelsReportPath, &labelsReport); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return nil // arquivo corrompido: sem tradução, mas sem falha
	}
	m := make(map[string]core.MilestoneIDMap, len(labelsReport.Results))
	for _, r := range labelsReport.Results {
		m[r.ID] = r.MilestoneIDMap
	}
	return m
}
```

- [ ] **Step 4: Adicionar comando em `internal/cli/root.go`**

Localizar a linha com `newMigrateLabelsCommand` e adicionar após:

```go
root.AddCommand(newMigrateIssuesCommand(exitCode))
```

- [ ] **Step 5: Rodar para verificar que o teste passa**

```bash
go test ./internal/cli/ -run TestRunMigrateIssues -v
```

Esperado: `PASS`.

- [ ] **Step 6: Atualizar `internal/cli/destroy.go`**

Adicionar `issuesReportPath` entre `labelsReportPath` e `validationReportPath`:

```go
var migrationArtifactPaths = []string{
	inventoryPath,
	migrationPlanPath,
	migrationReportPath,
	labelsReportPath,
	issuesReportPath,      // <- nova linha
	validationReportPath,
}
```

- [ ] **Step 7: Atualizar `internal/cli/destroy_test.go`**

Localizar a asserção `"Removidos 5 arquivos."` e substituir por `"Removidos 6 arquivos."`.

- [ ] **Step 8: Verificar testes de destroy**

```bash
go test ./internal/cli/ -run TestDestroy -v
```

Esperado: `PASS`.

- [ ] **Step 9: Atualizar `README.md`**

Localizar a seção de pipeline (que inclui `migrate-labels`) e:

1. Adicionar `migrate-issues` após `migrate-labels` na lista ordenada do pipeline.

2. Adicionar nova seção `### tfrepo migrate-issues` após a seção de `migrate-labels`:

```markdown
### tfrepo migrate-issues

Migra issues (abertas e fechadas) dos repositórios de origem para o destino.
Lê `labels-report.json` para traduzir referências de milestone usando o `milestoneIdMap`.
Issues já existentes no destino (por título) são ignoradas — a operação é idempotente.

```bash
tfrepo migrate-issues
```
```

3. Adicionar `issues-report.json` na tabela de artefatos (após `labels-report.json`):

```markdown
| `issues-report.json` | Resultado da migração de issues, incluindo contagem e erros |
```

4. Na seção de `destroy`, mencionar que `issues-report.json` também é removido.

- [ ] **Step 10: Rodar toda a suite**

```bash
go test ./...
```

Esperado: `ok` em todos os pacotes.

- [ ] **Step 11: Commit**

```bash
git add internal/cli/migrate_issues.go internal/cli/migrate_issues_test.go
git add internal/cli/root.go internal/cli/destroy.go internal/cli/destroy_test.go
git add README.md
git commit -m "feat(cli): add migrate-issues command, update destroy and README"
```

---

## Self-Review

### Cobertura de spec

| Requisito | Task |
|---|---|
| Tipo neutro `Issue` | Task 1 |
| `ListIssues` / `CreateIssue` na interface | Task 1 |
| Fakes existentes atualizados | Task 1 |
| GitHub: filter PRs, `State:"all"`, `ExternalID=GetNumber()` | Task 2 |
| GitHub: fechar via `Issues.Edit` | Task 2 |
| GitLab: sem filtro de state (retorna todos), `ExternalID=IID` | Task 3 |
| GitLab: normalizar "opened"→"open" | Task 3 |
| GitLab: fechar via `UpdateIssue(StateEvent:"close")` | Task 3 |
| `MilestoneIDMap` tradução na camada core | Task 4 |
| Skip-by-title (idempotente) | Task 4 |
| `security.Redact` em todos os erros do report | Task 4 |
| CLI lê `labels-report.json` opcionalmente | Task 5 |
| `issues-report.json` gerado | Task 5 |
| `destroy` atualizado (6 artefatos) | Task 5 |
| README atualizado | Task 5 |

### Consistência de tipos

- `Issue.MilestoneExternalID *int64` — usado em Task 1, 2, 3, 4, 5 ✓
- `MilestoneIDMap = map[string]int64` — chave decimal em `translateMilestone` ✓
- `IssuesMigrateResult.IssuesCreated int` — sem field extra (sem IssueIDMap; Plans futuros não precisam) ✓
- `runMigrateIssuesWithProviders` — assinatura compatível com o teste CLI ✓
