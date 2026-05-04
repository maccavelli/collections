// Package integration implements the Atlassian (Jira/Confluence) and GitLab
// integration layer for the MagicDev pipeline. It produces enriched documents
// from Blueprint data and pushes them to external systems.
package integration

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/ctreminiom/go-atlassian/v2/confluence"
	"github.com/ctreminiom/go-atlassian/v2/jira/v3"
	"github.com/ctreminiom/go-atlassian/v2/pkg/infra/models"
	"github.com/ericmason/mdadf"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/klauspost/compress/zstd"
	"github.com/spf13/viper"

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
		AporiaResolutions  []string        `json:"aporia_resolutions,omitzero"`
	} `json:"metadata"`
	Payload string `json:"payload_b64"`
}

// ProcessDocumentGeneration creates Jira task, Confluence page, and Hybrid Markdown Git commits.
// When bp is non-nil, the Blueprint data enriches all three outputs.
func ProcessDocumentGeneration(title, markdown, repoPath, sessionID string, bp *db.Blueprint, aporias []string) error {
	ctx := context.Background()

	// 1. Jira Ticket Creation
	jiraClient, err := v3.New(nil, viper.GetString("atlassian_url"))
	if err != nil {
		return fmt.Errorf("failed to create jira client: %w", err)
	}
	jiraClient.Auth.SetBasicAuth("", viper.GetString("atlassian_token"))

	issuePayload := &models.IssueScheme{
		Fields: &models.IssueFieldsScheme{
			Summary:   title,
			Project:   &models.ProjectScheme{Key: "PROJ"},
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

		storyPointsField := viper.GetString("jira_story_points_field")
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
	jiraID := "UNKNOWN"
	if jiraErr != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		slog.Warn("generate_documents: jira issue creation failed",
			"error", jiraErr,
			"status", status,
		)
	} else if issue != nil {
		jiraID = issue.Key
	}

	// 2. Confluence Page Creation (Markdown -> ADF)
	confluenceClient, err := confluence.New(nil, viper.GetString("atlassian_url"))
	if err != nil {
		return fmt.Errorf("failed to create confluence client: %w", err)
	}
	confluenceClient.Auth.SetBasicAuth("", viper.GetString("atlassian_token"))

	// Enrich markdown with Technical Implementation Roadmap section from Blueprint
	enrichedMarkdown := markdown
	if bp != nil {
		enrichedMarkdown = appendRoadmapSection(markdown, bp)
	}

	// Normalize line endings for Windows
	enrichedMarkdown = normalizeLineEndings(enrichedMarkdown)

	// Convert Markdown to ADF
	adfDoc, err := mdadf.Convert(enrichedMarkdown)
	if err != nil {
		return fmt.Errorf("failed to convert markdown to ADF: %w", err)
	}

	adfBytes, err := json.Marshal(adfDoc)
	if err != nil {
		return fmt.Errorf("failed to marshal ADF document: %w", err)
	}

	contentPayload := &models.ContentScheme{
		Type:  "page",
		Title: title,
		Space: &models.SpaceScheme{Key: "SPACE"},
		Body: &models.BodyScheme{
			Storage: &models.BodyNodeScheme{
				Value:          string(adfBytes),
				Representation: "atlas_doc_format",
			},
		},
	}

	if _, resp, err := confluenceClient.Content.Create(ctx, contentPayload); err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		slog.Warn("generate_documents: confluence page creation failed",
			"error", err,
			"status", status,
		)
	}

	// 3. Generate Hybrid Markdown with enriched metadata
	hybrid, err := generateHybridMarkdown(jiraID, enrichedMarkdown, bp, aporias)
	if err != nil {
		return err
	}

	hybridJSON, err := json.MarshalIndent(hybrid, "", "  ")
	if err != nil {
		return err
	}

	// 4. Git Push
	if err := pushToGitLab(repoPath, title, hybridJSON); err != nil {
		return fmt.Errorf("git push failed: %w", err)
	}

	return nil
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

func generateHybridMarkdown(jiraID, markdown string, bp *db.Blueprint, aporias []string) (*HybridMarkdown, error) {
	var buf bytes.Buffer
	enc, err := zstd.NewWriter(&buf, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		return nil, err
	}

	if _, err := enc.Write([]byte(markdown)); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}

	b64Payload := base64.StdEncoding.EncodeToString(buf.Bytes())

	hybrid := &HybridMarkdown{}
	hybrid.Metadata.JiraID = jiraID
	hybrid.Metadata.ProjectStep = "Finalized Design"
	hybrid.Metadata.Compression = "zstd"

	// Enrich metadata from Blueprint
	if bp != nil {
		hybrid.Metadata.DependencyManifest = bp.DependencyManifest
	}
	if len(aporias) > 0 {
		hybrid.Metadata.AporiaResolutions = aporias
	}

	hybrid.Payload = b64Payload

	return hybrid, nil
}

func pushToGitLab(repoPath, title string, fileContent []byte) error {
	r, err := git.PlainOpen(repoPath)
	if err != nil {
		return err
	}

	w, err := r.Worktree()
	if err != nil {
		return err
	}

	filePath := fmt.Sprintf("%s.json", title)
	fullPath := repoPath + "/" + filePath
	if err := os.WriteFile(fullPath, fileContent, 0644); err != nil {
		return err
	}

	if _, err := w.Add(filePath); err != nil {
		return err
	}

	_, err = w.Commit(fmt.Sprintf("Add hybrid markdown for %s", title), &git.CommitOptions{
		Author: &object.Signature{
			Name:  "MagicDev Agent",
			Email: "agent@magicdev.local",
			When:  time.Now(),
		},
	})
	if err != nil {
		return err
	}

	sshKeyPath := viper.GetString("ssh_private_key")
	if sshKeyPath == "" {
		return fmt.Errorf("ssh private key path not configured")
	}

	publicKeys, err := ssh.NewPublicKeysFromFile("git", sshKeyPath, "")
	if err != nil {
		return err
	}

	return r.Push(&git.PushOptions{
		Auth: publicKeys,
	})
}
