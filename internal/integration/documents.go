// Package integration implements the Atlassian (Jira/Confluence) and GitLab
// integration layer for the MagicDev pipeline. It produces enriched documents
// from Blueprint data and pushes them to external systems.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	gmhtml "github.com/yuin/goldmark/renderer/html"
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
// Returns the Jira issue key and the authoritative browse URL for downstream consumers.
func ProcessDocumentGeneration(store *db.Store, title, markdown, targetBranch, sessionID string, bp *db.Blueprint, synthesis *db.SynthesisResolution) (string, string, error) {
	ctx := context.Background()

	// Capture the Jira base URL ONCE from viper to prevent fsnotify
	// hot-reload drift across multiple downstream consumers.
	jiraBaseURL := viper.GetString("jira.url")

	// 1. Jira Ticket Creation or Retrieval
	var jiraToken string
	if store != nil {
		jiraToken, _ = store.GetSecret("jira")
	}
	jc := NewJiraClient(jiraBaseURL, jiraToken)

	jiraID := viper.GetString("jira.issue")
	if jiraID == "" {
		projectKey := viper.GetString("jira.project")
		if projectKey == "" {
			projectKey = "PROJ"
		}

		fields := map[string]interface{}{
			"summary":   title,
			"project":   map[string]string{"key": projectKey},
			"issuetype": map[string]string{"name": "Task"},
		}

		// Populate story points from Blueprint complexity scores if available
		if bp != nil && len(bp.ComplexityScores) > 0 {
			totalPoints := 0
			for _, points := range bp.ComplexityScores {
				totalPoints += points
			}

			storyPointsField := viper.GetString("jira.story_points_field")
			if storyPointsField != "" {
				fields[storyPointsField] = float64(totalPoints)
				slog.Info("generate_documents: setting story points from blueprint",
					"field", storyPointsField,
					"total_points", totalPoints,
				)
			} else {
				slog.Debug("generate_documents: skipping story points (jira.story_points_field not configured)")
			}
		}

		issue, status, jiraErr := jc.CreateIssue(ctx, &JiraIssuePayload{Fields: fields})
		if jiraErr != nil {
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

	// Compute the authoritative browse URL for downstream consumers.
	var browseURL string
	if jiraID != "" && jiraID != "UNKNOWN" && jiraBaseURL != "" {
		browseURL = fmt.Sprintf("%s/browse/%s", jiraBaseURL, jiraID)
		slog.Info("generate_documents: jira ticket resolved",
			"jira_id", jiraID,
			"jira_base_url", jiraBaseURL,
			"browse_url", browseURL,
		)
	}

	// 2. Confluence Page Creation (Markdown -> ADF)
	var confluenceToken string
	if store != nil {
		confluenceToken, _ = store.GetSecret("confluence")
	}
	cc := NewConfluenceClient(viper.GetString("confluence.url"), confluenceToken)

	// Enrich markdown with Technical Implementation Roadmap section from Blueprint
	enrichedMarkdown := markdown
	if bp != nil {
		enrichedMarkdown = appendRoadmapSection(markdown, bp)
	}

	if jiraID != "" && jiraID != "UNKNOWN" {
		enrichedMarkdown = fmt.Sprintf("**Associated Jira Task:** [%s](%s/browse/%s)\n\n%s", jiraID, jiraBaseURL, jiraID, enrichedMarkdown)
	}

	// Normalize line endings for Windows
	enrichedMarkdown = normalizeLineEndings(enrichedMarkdown)

	// 3. Generate Hybrid Markdown & MADR Early
	hybridBytes, hybridErr := generateHybridMarkdown(jiraID, enrichedMarkdown, bp, synthesis)
	if hybridErr != nil {
		return "", "", hybridErr
	}

	var session *db.SessionState
	if store != nil {
		var loadErr error
		session, loadErr = store.LoadSession(sessionID)
		if loadErr != nil {
			slog.Warn("failed to load session for ADR generation", "error", loadErr)
		}
	}

	var adrMarkdown string
	if bp != nil && session != nil {
		adrMarkdown = buildMADRDocument(title, session, bp, jiraBaseURL, browseURL)
	}

	// Format Hybrid Markdown for Confluence (replace frontmatter with code block)
	hybridStr := string(hybridBytes)
	hybridStr = strings.Replace(hybridStr, "---json\n", "```json\n", 1)
	hybridStr = strings.Replace(hybridStr, "\n---\n\n", "\n```\n\n", 1)

	// Convert Hybrid Markdown to XHTML storage format for Confluence Data Center
	xhtml, err := markdownToXHTML(hybridStr)
	if err != nil {
		return "", "", fmt.Errorf("failed to convert markdown to XHTML: %w", err)
	}

	space := viper.GetString("confluence.space")
	if space == "" {
		space = "SPACE"
	}

	confluenceTitle := title
	if jiraID != "" && jiraID != "UNKNOWN" {
		confluenceTitle = fmt.Sprintf("[%s] %s", jiraID, title)
	}

	contentPayload := &ConfluenceContentPayload{
		Type:  "page",
		Title: confluenceTitle,
		Space: ConfluenceSpaceRef{Key: space},
		Body: ConfluenceBodyPayload{
			Storage: ConfluenceStoragePayload{
				Value:          xhtml,
				Representation: "storage",
			},
		},
	}

	parentPageID := viper.GetString("confluence.parent_page_id")
	if parentPageID != "" {
		contentPayload.Ancestors = []ConfluenceAncestorRef{{ID: parentPageID}}
	}

	var parentID string
	createdPage, status, err := cc.CreateContent(ctx, contentPayload)
	if err != nil {
		slog.Warn("generate_documents: confluence page creation failed",
			"error", err,
			"status", status,
		)
	} else if createdPage != nil {
		parentID = createdPage.ID
	}

	// 2.5 Generate Confluence Child Pages for Original Markdown
	if parentID != "" && enrichedMarkdown != "" {
		childXHTML, err := markdownToXHTML(enrichedMarkdown)
		if err != nil {
			slog.Warn("failed to convert Original Markdown to XHTML", "error", err)
		} else {
			childPayload := &ConfluenceContentPayload{
				Type:  "page",
				Title: fmt.Sprintf("%s - Original Specifications", confluenceTitle),
				Space: ConfluenceSpaceRef{Key: space},
				Ancestors: []ConfluenceAncestorRef{
					{ID: parentID},
				},
				Body: ConfluenceBodyPayload{
					Storage: ConfluenceStoragePayload{
						Value:          childXHTML,
						Representation: "storage",
					},
				},
			}

			if _, childStatus, err := cc.CreateContent(ctx, childPayload); err != nil {
				slog.Warn("generate_documents: confluence child page creation failed",
					"error", err,
					"status", childStatus,
				)
			}
		}
	}

	// 2.6 Jira to Confluence Remote Link and Attachments
	if createdPage != nil {
		pageURL := fmt.Sprintf("%s/pages/viewpage.action?pageId=%s", viper.GetString("confluence.url"), createdPage.ID)

		// Create Jira Remote Link
		linkPayload := &JiraRemoteLinkPayload{}
		linkPayload.Object.URL = pageURL
		linkPayload.Object.Title = createdPage.Title
		if jiraID != "" && jiraID != "UNKNOWN" {
			if err := jc.CreateRemoteLink(ctx, jiraID, linkPayload); err != nil {
				slog.Warn("generate_documents: failed to create jira remote link", "error", err)
			}
		} else {
			slog.Debug("generate_documents: skipping jira remote link (no valid issue)")
		}

		// Upload D2 SVG as attachment (renders natively in Confluence)
		if bp != nil && bp.D2SVG != "" {
			fileName := fmt.Sprintf("%s_architecture.svg", title)
			reader := strings.NewReader(bp.D2SVG)
			if err := cc.CreateAttachment(ctx, createdPage.ID, "current", fileName, reader); err != nil {
				slog.Warn("generate_documents: failed to upload D2 SVG attachment to Confluence", "error", err)
			} else {
				slog.Info("generate_documents: D2 SVG attached to Confluence page",
					"file", fileName,
					"page_id", createdPage.ID,
					"svg_bytes", len(bp.D2SVG),
				)
			}
		} else if bp != nil && bp.D2Source != "" {
			slog.Warn("generate_documents: D2 source exists but SVG is empty — skipping Confluence attachment (rendering likely failed upstream)")
		}
	}

	// 2.7 Verify Cross-Links
	confluencePayload := xhtml
	if err := verifyCrossLinks(hybridBytes, confluencePayload, jiraID); err != nil {
		slog.Error("cross-link verification failed", "error", err)
		return "", "", err
	}

	// 4. Git Push via GitLab API
	mainGitLabContent := []byte(adrMarkdown)
	if len(mainGitLabContent) == 0 {
		mainGitLabContent = hybridBytes
		slog.Debug("generate_documents: using hybrid markdown for git commit (no MADR generated)")
	} else {
		slog.Debug("generate_documents: using MADR for git commit",
			"madr_bytes", len(mainGitLabContent),
			"jira_id", jiraID,
		)
	}
	if err := pushToGitLab(store, jiraID, targetBranch, title, mainGitLabContent, nil, bp); err != nil {
		return "", "", fmt.Errorf("gitlab push failed: %w", err)
	}

	return jiraID, browseURL, nil
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
	decisionCount := 0
	if bp != nil {
		for _, points := range bp.ComplexityScores {
			totalSP += points
		}
		fileCount = len(bp.FileStructure)
		adrCount = len(bp.ADRs)
	}
	if synthesis != nil {
		decisionCount = len(synthesis.Decisions)
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
		SchemaVersion:    db.CurrentSchemaVersion,
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339),
		JiraID:           jiraID,
		ProjectStep:      "Finalized Design",
		DecisionCount:    decisionCount,
		TotalStoryPoints: totalSP,
		FileCount:        fileCount,
		ADRCount:         adrCount,
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

func pushToGitLab(store *db.Store, jiraID, targetBranch, title string, fileContent []byte, adrContent []byte, bp *db.Blueprint) error {
	var gitToken string
	if store != nil {
		gitToken, _ = store.GetSecret("gitlab")
		if gitToken == "" {
			gitToken, _ = store.GetSecret("git")
		}
	}
	if gitToken == "" {
		return fmt.Errorf("gitlab token must be configured (run: mcp-server-magicdev token reconfigure)")
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

	if len(adrContent) > 0 {
		adrPath := fmt.Sprintf("%s_ADR.md", title)
		adrAction := gitlab.FileCreate
		if _, _, err := git.RepositoryFiles.GetFile(projectPath, adrPath, &gitlab.GetFileOptions{Ref: gitlab.Ptr(targetBranch)}); err == nil {
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
		if _, _, err := git.RepositoryFiles.GetFile(projectPath, d2Path, &gitlab.GetFileOptions{Ref: gitlab.Ptr(targetBranch)}); err == nil {
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
			if _, _, err := git.RepositoryFiles.GetFile(projectPath, svgPath, &gitlab.GetFileOptions{Ref: gitlab.Ptr(targetBranch)}); err == nil {
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
			slog.Warn("generate_documents: D2 source exists but SVG is empty — skipping SVG commit (rendering likely failed in BlueprintImplementation)")
		}
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


// verifyCrossLinks ensures that the Jira ID is embedded in both the hybrid markdown and confluence payload.
// It implements a 3-attempt exponential backoff retry loop.
func verifyCrossLinks(hybridBytes []byte, confluencePayload string, jiraID string) error {
	var err error
	for i := 0; i < 3; i++ {
		hybridHasJira := strings.Contains(string(hybridBytes), jiraID)
		confluenceHasJira := strings.Contains(confluencePayload, jiraID)

		if (hybridHasJira && confluenceHasJira) || jiraID == "UNKNOWN" {
			return nil
		}

		err = fmt.Errorf("Jira ID %q missing in cross-links (hybrid: %v, confluence: %v)", jiraID, hybridHasJira, confluenceHasJira)
		time.Sleep(time.Duration(1<<i) * time.Second) // exponential backoff
	}
	return err
}

// markdownToXHTML converts markdown content to XHTML storage format
// compatible with Confluence Data Center's /rest/api/content endpoint.
// Uses goldmark with XHTML rendering to ensure void elements (hr, br, img)
// use self-closing syntax (<hr />) required by Confluence's strict XHTML parser.
func markdownToXHTML(md string) (string, error) {
	// Pre-process: escape angle brackets in C#/Java generics outside code blocks
	// e.g. List<string> → List&lt;string&gt; but leave ```code blocks``` alone.
	sanitized := escapeGenericsOutsideCode(md)

	converter := goldmark.New(
		goldmark.WithRendererOptions(
			gmhtml.WithXHTML(),
		),
	)
	var buf bytes.Buffer
	if err := converter.Convert([]byte(sanitized), &buf); err != nil {
		return "", fmt.Errorf("goldmark conversion failed: %w", err)
	}
	return buf.String(), nil
}

// escapeGenericsOutsideCode escapes angle brackets that look like C#/Java generics
// (e.g. List<T>, Dictionary<string,object>) in markdown text that is NOT inside
// fenced code blocks or inline code spans. This prevents Confluence's strict XHTML
// parser from interpreting them as malformed HTML tags.
func escapeGenericsOutsideCode(md string) string {
	var result strings.Builder
	result.Grow(len(md))

	lines := strings.Split(md, "\n")
	inFencedBlock := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Toggle fenced code block state
		if strings.HasPrefix(trimmed, "```") {
			inFencedBlock = !inFencedBlock
			result.WriteString(line)
			if i < len(lines)-1 {
				result.WriteByte('\n')
			}
			continue
		}

		// Inside fenced code blocks, pass through unchanged
		if inFencedBlock {
			result.WriteString(line)
			if i < len(lines)-1 {
				result.WriteByte('\n')
			}
			continue
		}

		// Outside code blocks: escape angle brackets that are NOT part of
		// inline code spans (backtick-delimited). Process segments between backticks.
		escaped := escapeAngleBracketsPreservingInlineCode(line)
		result.WriteString(escaped)
		if i < len(lines)-1 {
			result.WriteByte('\n')
		}
	}

	return result.String()
}

// escapeAngleBracketsPreservingInlineCode escapes < and > in non-code segments
// of a single line, preserving content inside `backtick` inline code spans.
func escapeAngleBracketsPreservingInlineCode(line string) string {
	var result strings.Builder
	result.Grow(len(line) + 16)

	inInlineCode := false
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if ch == '`' {
			inInlineCode = !inInlineCode
			result.WriteByte(ch)
			continue
		}
		if inInlineCode {
			result.WriteByte(ch)
			continue
		}
		switch ch {
		case '<':
			result.WriteString("&lt;")
		case '>':
			result.WriteString("&gt;")
		default:
			result.WriteByte(ch)
		}
	}

	return result.String()
}

