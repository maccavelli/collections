package integration

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
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
)

type HybridMarkdown struct {
	Metadata struct {
		JiraID      string `json:"jira_id"`
		ProjectStep string `json:"step"`
		Compression string `json:"compression"`
	} `json:"metadata"`
	Payload string `json:"payload_b64"`
}

func ProcessDocumentGeneration(title, markdown, repoPath, sessionID string) error {
	ctx := context.Background()

	// 1. Jira Ticket Creation
	jiraClient, err := v3.New(nil, viper.GetString("atlassian_url"))
	if err != nil {
		return fmt.Errorf("failed to create jira client: %w", err)
	}
	jiraClient.Auth.SetBasicAuth("", viper.GetString("atlassian_token"))

	issuePayload := &models.IssueScheme{
		Fields: &models.IssueFieldsScheme{
			Summary: title,
			Project: &models.ProjectScheme{Key: "PROJ"},
			IssueType: &models.IssueTypeScheme{Name: "Task"},
		},
	}
	issue, _, _ := jiraClient.Issue.Create(ctx, issuePayload, nil)
	jiraID := "UNKNOWN"
	if issue != nil {
		jiraID = issue.Key
	}

	// 2. Confluence Page Creation (Markdown -> ADF)
	confluenceClient, err := confluence.New(nil, viper.GetString("atlassian_url"))
	if err != nil {
		return fmt.Errorf("failed to create confluence client: %w", err)
	}
	confluenceClient.Auth.SetBasicAuth("", viper.GetString("atlassian_token"))

	// Convert Markdown to ADF
	adfDoc, err := mdadf.Convert(markdown)
	if err != nil {
		return fmt.Errorf("failed to convert markdown to ADF: %w", err)
	}

	adfBytes, _ := json.Marshal(adfDoc)

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
	
	_, _, _ = confluenceClient.Content.Create(ctx, contentPayload)

	// 3. Generate Hybrid Markdown
	hybrid, err := generateHybridMarkdown(jiraID, markdown)
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

func generateHybridMarkdown(jiraID, markdown string) (*HybridMarkdown, error) {
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
