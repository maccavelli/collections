// Package integration implements the Atlassian (Jira/Confluence) and GitLab
// integration layer for the MagicDev pipeline. It produces enriched documents
// from Blueprint data and pushes them to external systems.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"time"

	"github.com/ctreminiom/go-atlassian/v2/confluence"
	"github.com/ctreminiom/go-atlassian/v2/jira/v3"
	"github.com/ctreminiom/go-atlassian/v2/pkg/infra/models"
	"github.com/ericmason/mdadf"
	"github.com/spf13/viper"
	"gitlab.com/gitlab-org/api/client-go"

	"mcp-server-magicdev/internal/db"
)

// HybridMarkdown is the Git-committed artifact combining Jira metadata,
// Blueprint enrichment data, and Zstd-compressed markdown payload.
type HybridMarkdown struct {
	Metadata struct {
		JiraID             string          `json:"jira_id"`
		ProjectStep        string          `json:"step"`
		Compression        string          `json:"compression"`
		DependencyManifest []db.Dependency `json:"dependency_manifest,omitzero"`
		DecisionCount      int             `json:"decision_count,omitzero"`
	} `json:"metadata"`
	Payload string `json:"payload_b64"`
}

// ProcessDocumentGeneration creates Jira task, Confluence page, and Hybrid Markdown Git commits.
// When bp is non-nil, the Blueprint data enriches all three outputs.
func ProcessDocumentGeneration(store *db.Store, title, markdown, targetBranch, sessionID string, bp *db.Blueprint, synthesis *db.SynthesisResolution) (string, error) {
	ctx := context.Background()

	// 1. Jira Ticket Creation or Retrieval
	jiraClient, err := v3.New(nil, viper.GetString("jira.url"))
	if err != nil {
		return "", fmt.Errorf("failed to create jira client: %w", err)
	}
	var jiraToken string
	if store != nil {
		jiraToken, _ = store.GetSecret("jira")
	}
	jiraClient.Auth.SetBasicAuth("", jiraToken)

	jiraID := viper.GetString("jira.issue")
	if jiraID == "" {
		projectKey := viper.GetString("jira.project")
		if projectKey == "" {
			projectKey = "PROJ"
		}

		issuePayload := &models.IssueScheme{
			Fields: &models.IssueFieldsScheme{
				Summary:   title,
				Project:   &models.ProjectScheme{Key: projectKey},
				IssueType: &models.IssueTypeScheme{Name: "Task"},
			},
		}

		// Populate story points from Blueprint complexity scores if available
		var customFields *models.CustomFields
		if bp != nil && len(bp.ComplexityScores) > 0 {
			totalPoints := 0
			for _, points := range bp.ComplexityScores {
				totalPoints += points
			}

			storyPointsField := viper.GetString("jira.story_points_field")
			if storyPointsField == "" {
				storyPointsField = "customfield_10016" // Jira Cloud default
			}

			customFields = &models.CustomFields{}
			if err := customFields.Number(storyPointsField, float64(totalPoints)); err != nil {
				slog.Warn("generate_documents: failed to set story points custom field", "error", err)
			} else {
				slog.Info("generate_documents: setting story points from blueprint",
					"field", storyPointsField,
					"total_points", totalPoints,
				)
			}
		}

		issue, resp, jiraErr := jiraClient.Issue.Create(ctx, issuePayload, customFields)
		if jiraErr != nil {
			status := 0
			if resp != nil {
				status = resp.StatusCode
			}
			slog.Warn("generate_documents: jira issue creation failed",
				"error", jiraErr,
				"status", status,
			)
			jiraID = "UNKNOWN"
		} else if issue != nil {
			jiraID = issue.Key
		} else {
			jiraID = "UNKNOWN"
		}
	}

	// 2. Confluence Page Creation (Markdown -> ADF)
	confluenceClient, err := confluence.New(nil, viper.GetString("confluence.url"))
	if err != nil {
		return "", fmt.Errorf("failed to create confluence client: %w", err)
	}
	var confluenceToken string
	if store != nil {
		confluenceToken, _ = store.GetSecret("confluence")
	}
	confluenceClient.Auth.SetBasicAuth("", confluenceToken)

	// Enrich markdown with Technical Implementation Roadmap section from Blueprint
	enrichedMarkdown := markdown
	if bp != nil {
		enrichedMarkdown = appendRoadmapSection(markdown, bp)
	}

	if jiraID != "" && jiraID != "UNKNOWN" {
		enrichedMarkdown = fmt.Sprintf("**Associated Jira Task:** [%s](%s/browse/%s)\n\n%s", jiraID, viper.GetString("jira.url"), jiraID, enrichedMarkdown)
	}

	// Normalize line endings for Windows
	enrichedMarkdown = normalizeLineEndings(enrichedMarkdown)

	// Convert Markdown to ADF
	adfDoc, err := mdadf.Convert(enrichedMarkdown)
	if err != nil {
		return "", fmt.Errorf("failed to convert markdown to ADF: %w", err)
	}

	adfBytes, err := json.Marshal(adfDoc)
	if err != nil {
		return "", fmt.Errorf("failed to marshal ADF document: %w", err)
	}

	space := viper.GetString("confluence.space")
	if space == "" {
		space = "SPACE"
	}

	confluenceTitle := title
	if jiraID != "" && jiraID != "UNKNOWN" {
		confluenceTitle = fmt.Sprintf("[%s] %s", jiraID, title)
	}

	contentPayload := &models.ContentScheme{
		Type:  "page",
		Title: confluenceTitle,
		Space: &models.SpaceScheme{Key: space},
		Body: &models.BodyScheme{
			Storage: &models.BodyNodeScheme{
				Value:          string(adfBytes),
				Representation: "atlas_doc_format",
			},
		},
	}

	parentPageID := viper.GetString("confluence.parent_page_id")
	if parentPageID != "" {
		contentPayload.Ancestors = []*models.ContentScheme{{ID: parentPageID}}
	}

	var parentID string
	createdPage, resp, err := confluenceClient.Content.Create(ctx, contentPayload)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		slog.Warn("generate_documents: confluence page creation failed",
			"error", err,
			"status", status,
		)
	} else if createdPage != nil {
		parentID = createdPage.ID
	}

	// 2.5 Generate Confluence Child Pages for ADRs
	if parentID != "" && bp != nil && len(bp.ADRs) > 0 {
		for i, adr := range bp.ADRs {
			adrMarkdown := fmt.Sprintf("# ADR %d: %s\n\n**Status:** %s\n\n## Context\n%s\n\n## Decision\n%s\n\n## Consequences\n%s\n",
				i+1, adr.Title, adr.Status, adr.Context, adr.Decision, adr.Consequences)

			adrADF, err := mdadf.Convert(adrMarkdown)
			if err != nil {
				slog.Warn("failed to convert ADR to ADF", "error", err)
				continue
			}
			adrBytes, err := json.Marshal(adrADF)
			if err != nil {
				slog.Warn("failed to marshal ADR ADF", "error", err)
				continue
			}

			adrPayload := &models.ContentScheme{
				Type:  "page",
				Title: fmt.Sprintf("%s - ADR: %s", confluenceTitle, adr.Title),
				Space: &models.SpaceScheme{Key: space},
				Ancestors: []*models.ContentScheme{
					{ID: parentID},
				},
				Body: &models.BodyScheme{
					Storage: &models.BodyNodeScheme{
						Value:          string(adrBytes),
						Representation: "atlas_doc_format",
					},
				},
			}

			if _, resp, err := confluenceClient.Content.Create(ctx, adrPayload); err != nil {
				status := 0
				if resp != nil {
					status = resp.StatusCode
				}
				slog.Warn("generate_documents: confluence adr page creation failed",
					"error", err,
					"status", status,
				)
			}
		}
	}

	// 3. Generate Hybrid Markdown
	hybridBytes, err := generateHybridMarkdown(jiraID, enrichedMarkdown, bp, synthesis)
	if err != nil {
		return "", err
	}

	// 4. Git Push via GitLab API
	if err := pushToGitLab(store, jiraID, targetBranch, title, hybridBytes, bp); err != nil {
		return "", fmt.Errorf("gitlab push failed: %w", err)
	}

	return jiraID, nil
}

// appendRoadmapSection adds a "Technical Implementation Roadmap" section to the markdown.
func appendRoadmapSection(markdown string, bp *db.Blueprint) string {
	var b strings.Builder
	b.WriteString(markdown)
	b.WriteString("\n\n---\n\n## Technical Implementation Roadmap\n\n")

	if len(bp.ImplementationStrategy) > 0 {
		b.WriteString("### Strategy Mapping\n\n")
		b.WriteString("| Requirement | Pattern |\n")
		b.WriteString("|---|---|\n")
		for req, pattern := range bp.ImplementationStrategy {
			b.WriteString(fmt.Sprintf("| %s | %s |\n", req, pattern))
		}
		b.WriteString("\n")
	}

	if len(bp.DependencyManifest) > 0 {
		b.WriteString("### Dependency Manifest\n\n")
		b.WriteString("| Package | Version | Ecosystem |\n")
		b.WriteString("|---|---|---|\n")
		for _, dep := range bp.DependencyManifest {
			b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", dep.Name, dep.Version, dep.Ecosystem))
		}
		b.WriteString("\n")
	}

	if len(bp.ComplexityScores) > 0 {
		b.WriteString("### Complexity Estimation\n\n")
		totalPoints := 0
		for feature, points := range bp.ComplexityScores {
			b.WriteString(fmt.Sprintf("- **%s**: %d SP\n", feature, points))
			totalPoints += points
		}
		b.WriteString(fmt.Sprintf("\n**Total:** %d story points\n\n", totalPoints))
	}

	if len(bp.AporiaTraceability) > 0 {
		b.WriteString("### Aporia Traceability\n\n")
		for contradiction, resolution := range bp.AporiaTraceability {
			b.WriteString(fmt.Sprintf("- **%s** → %s\n", contradiction, resolution))
		}
	}

	return b.String()
}

// normalizeLineEndings converts LF to CRLF on Windows for markdown bodies.
func normalizeLineEndings(body string) string {
	if runtime.GOOS == "windows" {
		// Avoid double-converting existing CRLF
		body = strings.ReplaceAll(body, "\r\n", "\n")
		body = strings.ReplaceAll(body, "\n", "\r\n")
	}
	return body
}

func generateHybridMarkdown(jiraID, markdown string, bp *db.Blueprint, synthesis *db.SynthesisResolution) ([]byte, error) {
	// Compute aggregate metrics for frontmatter
	totalSP := 0
	fileCount := 0
	adrCount := 0
	if bp != nil {
		for _, points := range bp.ComplexityScores {
			totalSP += points
		}
		fileCount = len(bp.FileStructure)
		adrCount = len(bp.ADRs)
	}

	meta := struct {
		SchemaVersion      int             `json:"schema_version"`
		GeneratedAt        string          `json:"generated_at"`
		JiraID             string          `json:"jira_id"`
		ProjectStep        string          `json:"project_step"`
		DependencyManifest []db.Dependency `json:"dependency_manifest,omitempty"`
		DecisionCount      int             `json:"decision_count,omitempty"`
		TotalStoryPoints   int             `json:"total_story_points,omitempty"`
		FileCount          int             `json:"file_count,omitempty"`
		ADRCount           int             `json:"adr_count,omitempty"`
	}{
		SchemaVersion:     db.CurrentSchemaVersion,
		GeneratedAt:       time.Now().UTC().Format(time.RFC3339),
		JiraID:            jiraID,
		ProjectStep:       "Finalized Design",
		TotalStoryPoints:  totalSP,
		FileCount:         fileCount,
		ADRCount:          adrCount,
	}
	if bp != nil {
		meta.DependencyManifest = bp.DependencyManifest
	}

	jsonBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, err
	}

	var buf strings.Builder
	buf.WriteString("---json\n")
	buf.Write(jsonBytes)
	buf.WriteString("\n---\n\n")
	buf.WriteString(markdown)

	return []byte(buf.String()), nil
}

func pushToGitLab(store *db.Store, jiraID, targetBranch, title string, fileContent []byte, bp *db.Blueprint) error {
	var gitToken string
	if store != nil {
		gitToken, _ = store.GetSecret("gitlab")
	}
	if gitToken == "" {
		return fmt.Errorf("git_token must be configured for GitLab API push")
	}

	serverURL := viper.GetString("git.server_url")
	projectPath := viper.GetString("git.project_path")
	if serverURL == "" || projectPath == "" {
		return fmt.Errorf("git.server_url and git.project_path must be configured")
	}

	git, err := gitlab.NewClient(gitToken, gitlab.WithBaseURL(serverURL))
	if err != nil {
		return fmt.Errorf("failed to create gitlab client: %w", err)
	}

	// Auto-create branch if it does not exist
	defaultBranch := viper.GetString("git.default_branch")
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	_, resp, err := git.Branches.GetBranch(projectPath, targetBranch)
	if err != nil && resp != nil && resp.StatusCode == 404 {
		slog.Info("branch not found, creating", "branch", targetBranch, "ref", defaultBranch)
		_, _, createErr := git.Branches.CreateBranch(projectPath, &gitlab.CreateBranchOptions{
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
	if _, _, err := git.RepositoryFiles.GetFile(projectPath, filePath, &gitlab.GetFileOptions{Ref: gitlab.Ptr(targetBranch)}); err == nil {
		fileAction = gitlab.FileUpdate
	}

	actions = append(actions, &gitlab.CommitActionOptions{
		Action:   gitlab.Ptr(fileAction),
		FilePath: gitlab.Ptr(filePath),
		Content:  gitlab.Ptr(string(fileContent)),
	})

	// Commit standalone Mermaid diagram file alongside the spec
	if bp != nil && bp.MermaidDiagram != "" {
		mmdPath := fmt.Sprintf("%s_architecture.mmd", title)
		mmdAction := gitlab.FileCreate
		if _, _, err := git.RepositoryFiles.GetFile(projectPath, mmdPath, &gitlab.GetFileOptions{Ref: gitlab.Ptr(targetBranch)}); err == nil {
			mmdAction = gitlab.FileUpdate
		}
		actions = append(actions, &gitlab.CommitActionOptions{
			Action:   gitlab.Ptr(mmdAction),
			FilePath: gitlab.Ptr(mmdPath),
			Content:  gitlab.Ptr(bp.MermaidDiagram),
		})
	}

	opt := &gitlab.CreateCommitOptions{
		Branch:        gitlab.Ptr(targetBranch),
		CommitMessage: gitlab.Ptr(commitMsg),
		Actions:       actions,
	}

	_, _, err = git.Commits.CreateCommit(projectPath, opt)
	if err != nil {
		return fmt.Errorf("failed to create commit: %w", err)
	}

	return nil
}
