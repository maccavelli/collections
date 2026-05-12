package git

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/spf13/viper"
	"gitlab.com/gitlab-org/api/client-go"

	"mcp-server-magicdev/internal/db"
)

type GitLabProvider struct{}

func NewGitLabProvider() *GitLabProvider {
	return &GitLabProvider{}
}

func (p *GitLabProvider) PushDocuments(ctx context.Context, store *db.Store, jiraID, targetBranch, title string, fileContent, adrContent []byte, bp *db.Blueprint) error {
	var gitToken string
	if store != nil {
		gitToken, _ = store.GetSecret("gitlab")
	}
	if gitToken == "" {
		return fmt.Errorf("gitlab token must be configured")
	}

	serverURL := viper.GetString("gitlab.server_url")
	projectPath := viper.GetString("gitlab.project_path")
	if serverURL == "" || projectPath == "" {
		return fmt.Errorf("gitlab.server_url and gitlab.project_path must be configured")
	}

	gitClient, err := gitlab.NewClient(gitToken, gitlab.WithBaseURL(serverURL))
	if err != nil {
		return fmt.Errorf("failed to create gitlab client: %w", err)
	}

	// Auto-create branch if it does not exist
	defaultBranch := viper.GetString("gitlab.default_branch")
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	_, resp, err := gitClient.Branches.GetBranch(projectPath, targetBranch)
	if err != nil && resp != nil && resp.StatusCode == 404 {
		slog.Info("branch not found, creating", "branch", targetBranch, "ref", defaultBranch)
		_, _, createErr := gitClient.Branches.CreateBranch(projectPath, &gitlab.CreateBranchOptions{
			Branch: gitlab.Ptr(targetBranch),
			Ref:    gitlab.Ptr(defaultBranch),
		})
		if createErr != nil {
			return fmt.Errorf("failed to create branch %q from %q: %w", targetBranch, defaultBranch, createErr)
		}
	}

	filePath := fmt.Sprintf("%s.md", title)
	commitMsg := fmt.Sprintf("Add Hybrid Markdown for %s", title)
	if jiraID != "" && jiraID != "UNKNOWN" {
		commitMsg = fmt.Sprintf("[%s] Add Hybrid Markdown for %s", jiraID, title)
	}

	actions := []*gitlab.CommitActionOptions{}

	fileAction := gitlab.FileCreate
	if _, _, err := gitClient.RepositoryFiles.GetFile(projectPath, filePath, &gitlab.GetFileOptions{Ref: gitlab.Ptr(targetBranch)}); err == nil {
		fileAction = gitlab.FileUpdate
	}

	actions = append(actions, &gitlab.CommitActionOptions{
		Action:   gitlab.Ptr(fileAction),
		FilePath: gitlab.Ptr(filePath),
		Content:  gitlab.Ptr(string(fileContent)),
	})

	if len(adrContent) > 0 {
		adrPath := fmt.Sprintf("%s_ADR.md", title)
		adrAction := gitlab.FileCreate
		if _, _, err := gitClient.RepositoryFiles.GetFile(projectPath, adrPath, &gitlab.GetFileOptions{Ref: gitlab.Ptr(targetBranch)}); err == nil {
			adrAction = gitlab.FileUpdate
		}
		actions = append(actions, &gitlab.CommitActionOptions{
			Action:   gitlab.Ptr(adrAction),
			FilePath: gitlab.Ptr(adrPath),
			Content:  gitlab.Ptr(string(adrContent)),
		})
	}

	// Commit D2 source and rendered SVG alongside the spec
	if bp != nil && bp.D2Source != "" {
		d2Path := fmt.Sprintf("%s_architecture.d2", title)
		d2Action := gitlab.FileCreate
		if _, _, err := gitClient.RepositoryFiles.GetFile(projectPath, d2Path, &gitlab.GetFileOptions{Ref: gitlab.Ptr(targetBranch)}); err == nil {
			d2Action = gitlab.FileUpdate
		}
		actions = append(actions, &gitlab.CommitActionOptions{
			Action:   gitlab.Ptr(d2Action),
			FilePath: gitlab.Ptr(d2Path),
			Content:  gitlab.Ptr(bp.D2Source),
		})
		slog.Info("generate_documents: committing D2 source to GitLab",
			"path", d2Path,
			"action", d2Action,
			"d2_bytes", len(bp.D2Source),
		)

		if bp.D2SVG != "" {
			svgPath := fmt.Sprintf("%s_architecture.svg", title)
			svgAction := gitlab.FileCreate
			if _, _, err := gitClient.RepositoryFiles.GetFile(projectPath, svgPath, &gitlab.GetFileOptions{Ref: gitlab.Ptr(targetBranch)}); err == nil {
				svgAction = gitlab.FileUpdate
			}
			actions = append(actions, &gitlab.CommitActionOptions{
				Action:   gitlab.Ptr(svgAction),
				FilePath: gitlab.Ptr(svgPath),
				Content:  gitlab.Ptr(bp.D2SVG),
			})
			slog.Info("generate_documents: committing D2 SVG to GitLab",
				"path", svgPath,
				"action", svgAction,
				"svg_bytes", len(bp.D2SVG),
			)
		} else {
			slog.Warn("generate_documents: D2 source exists but SVG is empty — skipping SVG commit")
		}
	}

	opt := &gitlab.CreateCommitOptions{
		Branch:        gitlab.Ptr(targetBranch),
		CommitMessage: gitlab.Ptr(commitMsg),
		Actions:       actions,
	}

	_, _, err = gitClient.Commits.CreateCommit(projectPath, opt)
	if err != nil {
		return fmt.Errorf("failed to create commit: %w", err)
	}

	return nil
}
