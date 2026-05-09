// Package integration implements the Atlassian (Jira/Confluence) and GitLab
// integration layer for the MagicDev pipeline.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

// --- Jira REST Types ---

// JiraIssuePayload is the JSON body for POST /rest/api/2/issue.
type JiraIssuePayload struct {
	Fields map[string]interface{} `json:"fields"`
}

// JiraIssueResponse is the subset of fields returned by Jira after issue creation.
type JiraIssueResponse struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Self string `json:"self"`
}

// JiraRemoteLinkPayload is the JSON body for POST /rest/api/2/issue/{key}/remotelink.
type JiraRemoteLinkPayload struct {
	Object struct {
		URL   string `json:"url"`
		Title string `json:"title"`
	} `json:"object"`
}

// --- Confluence REST Types ---

// ConfluenceContentPayload is the JSON body for POST /rest/api/content.
type ConfluenceContentPayload struct {
	Type      string                    `json:"type"`
	Title     string                    `json:"title"`
	Space     ConfluenceSpaceRef        `json:"space"`
	Ancestors []ConfluenceAncestorRef   `json:"ancestors,omitempty"`
	Body      ConfluenceBodyPayload     `json:"body"`
}

// ConfluenceSpaceRef identifies a space by key.
type ConfluenceSpaceRef struct {
	Key string `json:"key"`
}

// ConfluenceAncestorRef identifies a parent page by ID.
type ConfluenceAncestorRef struct {
	ID string `json:"id"`
}

// ConfluenceBodyPayload wraps the storage representation.
type ConfluenceBodyPayload struct {
	Storage ConfluenceStoragePayload `json:"storage"`
}

// ConfluenceStoragePayload holds the page body value and representation type.
type ConfluenceStoragePayload struct {
	Value          string `json:"value"`
	Representation string `json:"representation"`
}

// ConfluenceContentResponse is the subset of fields returned after page creation.
type ConfluenceContentResponse struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// --- Jira Client ---

// JiraClient is a thin, DC-native REST client for Jira Data Center.
// It uses /rest/api/2/ endpoints with Bearer token authentication.
type JiraClient struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// NewJiraClient creates a JiraClient for the given base URL and PAT.
func NewJiraClient(baseURL, token string) *JiraClient {
	return &JiraClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    http.DefaultClient,
	}
}

// GetProject performs a lightweight connectivity check against Jira.
//
// GET /rest/api/2/project/{projectKey}
func (jc *JiraClient) GetProject(ctx context.Context, projectKey string) error {
	endpoint := fmt.Sprintf("%s/rest/api/2/project/%s", jc.BaseURL, projectKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+jc.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := jc.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("jira request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("jira returned HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// CreateIssue creates a new Jira issue.
//
// POST /rest/api/2/issue
func (jc *JiraClient) CreateIssue(ctx context.Context, payload *JiraIssuePayload) (*JiraIssueResponse, int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal issue payload: %w", err)
	}

	endpoint := fmt.Sprintf("%s/rest/api/2/issue", jc.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+jc.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := jc.HTTP.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("jira request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, resp.StatusCode, fmt.Errorf("jira issue creation returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result JiraIssueResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to decode jira response: %w", err)
	}
	return &result, resp.StatusCode, nil
}

// CreateRemoteLink creates a remote link on a Jira issue.
//
// POST /rest/api/2/issue/{issueKey}/remotelink
func (jc *JiraClient) CreateRemoteLink(ctx context.Context, issueKey string, link *JiraRemoteLinkPayload) error {
	body, err := json.Marshal(link)
	if err != nil {
		return fmt.Errorf("failed to marshal remote link payload: %w", err)
	}

	endpoint := fmt.Sprintf("%s/rest/api/2/issue/%s/remotelink", jc.BaseURL, issueKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+jc.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := jc.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("jira request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("jira remote link creation returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// --- Confluence Client ---

// ConfluenceClient is a thin, DC-native REST client for Confluence Data Center.
// It uses /rest/api/ endpoints with Bearer token authentication.
type ConfluenceClient struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// NewConfluenceClient creates a ConfluenceClient for the given base URL and PAT.
func NewConfluenceClient(baseURL, token string) *ConfluenceClient {
	return &ConfluenceClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    http.DefaultClient,
	}
}

// GetSpace performs a lightweight connectivity check against Confluence.
//
// GET /rest/api/space/{spaceKey}
func (cc *ConfluenceClient) GetSpace(ctx context.Context, spaceKey string) error {
	endpoint := fmt.Sprintf("%s/rest/api/space/%s", cc.BaseURL, spaceKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cc.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := cc.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("confluence request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("confluence returned HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// CreateContent creates a new Confluence page.
//
// POST /rest/api/content
func (cc *ConfluenceClient) CreateContent(ctx context.Context, payload *ConfluenceContentPayload) (*ConfluenceContentResponse, int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal content payload: %w", err)
	}

	endpoint := fmt.Sprintf("%s/rest/api/content", cc.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+cc.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := cc.HTTP.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("confluence request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, resp.StatusCode, fmt.Errorf("confluence content creation returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result ConfluenceContentResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to decode confluence response: %w", err)
	}
	return &result, resp.StatusCode, nil
}

// CreateAttachment uploads a file attachment to an existing Confluence page.
//
// POST /rest/api/content/{contentID}/child/attachment
func (cc *ConfluenceClient) CreateAttachment(ctx context.Context, contentID, status, fileName string, reader io.Reader) error {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return fmt.Errorf("failed to create multipart form: %w", err)
	}
	if _, err := io.Copy(part, reader); err != nil {
		return fmt.Errorf("failed to write attachment data: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	endpoint := fmt.Sprintf("%s/rest/api/content/%s/child/attachment", cc.BaseURL, contentID)
	if status != "" {
		endpoint += "?status=" + status
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cc.Token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Atlassian-Token", "nocheck")

	resp, err := cc.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("confluence request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("confluence attachment upload returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
