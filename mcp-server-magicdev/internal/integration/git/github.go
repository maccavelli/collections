package git

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/go-github/v62/github"
	"github.com/spf13/viper"

	"mcp-server-magicdev/internal/db"
)

type GitHubProvider struct{}

func NewGitHubProvider() *GitHubProvider {
	return &GitHubProvider{}
}

func (p *GitHubProvider) PushDocuments(ctx context.Context, store *db.Store, jiraID, targetBranch, title string, fileContent, adrContent []byte, bp *db.Blueprint) error {
	var gitToken string
	if store != nil {
		gitToken, _ = store.GetSecret("github")
	}
	if gitToken == "" {
		return fmt.Errorf("github token must be configured")
	}

	serverURL := viper.GetString("github.server_url")
	projectPath := viper.GetString("github.project_path")
	if projectPath == "" {
		return fmt.Errorf("github.project_path must be configured")
	}

	parts := strings.Split(projectPath, "/")
	if len(parts) != 2 {
		return fmt.Errorf("github.project_path must be in the format owner/repo")
	}
	owner, repo := parts[0], parts[1]

	var client *github.Client
	if serverURL != "" && serverURL != "https://github.com" {
		if !strings.HasSuffix(serverURL, "/") {
			serverURL += "/"
		}
		baseURL, err := url.Parse(serverURL + "api/v3/")
		if err != nil {
			return fmt.Errorf("invalid github server url: %w", err)
		}
		uploadURL, err := url.Parse(serverURL + "api/uploads/")
		if err != nil {
			return fmt.Errorf("invalid github upload url: %w", err)
		}
		client, err = github.NewEnterpriseClient(baseURL.String(), uploadURL.String(), nil)
		if err != nil {
			return fmt.Errorf("failed to create github enterprise client: %w", err)
		}
	} else {
		client = github.NewClient(nil)
	}
	client.WithAuthToken(gitToken)

	defaultBranch := viper.GetString("github.default_branch")
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	refName := "refs/heads/" + targetBranch
	ref, resp, err := client.Git.GetRef(ctx, owner, repo, refName)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			slog.Info("branch not found, creating", "branch", targetBranch, "ref", defaultBranch)
			baseRefName := "refs/heads/" + defaultBranch
			baseRef, _, err := client.Git.GetRef(ctx, owner, repo, baseRefName)
			if err != nil {
				return fmt.Errorf("failed to get base branch %q: %w", defaultBranch, err)
			}
			newRef := &github.Reference{
				Ref: github.String(refName),
				Object: &github.GitObject{
					SHA: baseRef.Object.SHA,
				},
			}
			ref, _, err = client.Git.CreateRef(ctx, owner, repo, newRef)
			if err != nil {
				return fmt.Errorf("failed to create branch %q: %w", targetBranch, err)
			}
		} else {
			return fmt.Errorf("failed to get ref for %q: %w", targetBranch, err)
		}
	}

	commit, _, err := client.Git.GetCommit(ctx, owner, repo, *ref.Object.SHA)
	if err != nil {
		return fmt.Errorf("failed to get commit: %w", err)
	}

	entries := []*github.TreeEntry{}

	filePath := fmt.Sprintf("%s.md", title)
	entries = append(entries, &github.TreeEntry{
		Path:    github.String(filePath),
		Mode:    github.String("100644"),
		Type:    github.String("blob"),
		Content: github.String(string(fileContent)),
	})

	if len(adrContent) > 0 {
		adrPath := fmt.Sprintf("%s_ADR.md", title)
		entries = append(entries, &github.TreeEntry{
			Path:    github.String(adrPath),
			Mode:    github.String("100644"),
			Type:    github.String("blob"),
			Content: github.String(string(adrContent)),
		})
	}

	if bp != nil && bp.D2Source != "" {
		d2Path := fmt.Sprintf("%s_architecture.d2", title)
		entries = append(entries, &github.TreeEntry{
			Path:    github.String(d2Path),
			Mode:    github.String("100644"),
			Type:    github.String("blob"),
			Content: github.String(bp.D2Source),
		})
		if bp.D2SVG != "" {
			svgPath := fmt.Sprintf("%s_architecture.svg", title)
			entries = append(entries, &github.TreeEntry{
				Path:    github.String(svgPath),
				Mode:    github.String("100644"),
				Type:    github.String("blob"),
				Content: github.String(bp.D2SVG),
			})
		}
	}

	tree, _, err := client.Git.CreateTree(ctx, owner, repo, *commit.Tree.SHA, entries)
	if err != nil {
		return fmt.Errorf("failed to create tree: %w", err)
	}

	commitMsg := fmt.Sprintf("Add Hybrid Markdown for %s", title)
	if jiraID != "" && jiraID != "UNKNOWN" {
		commitMsg = fmt.Sprintf("[%s] Add Hybrid Markdown for %s", jiraID, title)
	}

	newCommit := &github.Commit{
		Message: github.String(commitMsg),
		Tree:    tree,
		Parents: []*github.Commit{commit},
	}
	createdCommit, _, err := client.Git.CreateCommit(ctx, owner, repo, newCommit, nil)
	if err != nil {
		return fmt.Errorf("failed to create commit: %w", err)
	}

	ref.Object.SHA = createdCommit.SHA
	_, _, err = client.Git.UpdateRef(ctx, owner, repo, ref, false)
	if err != nil {
		return fmt.Errorf("failed to update ref: %w", err)
	}

	return nil
}
