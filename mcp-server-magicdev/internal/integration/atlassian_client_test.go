package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJiraClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Authorization"), "Bearer test-token") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		
		switch r.URL.Path {
		case "/rest/api/2/project/PROJ":
			w.WriteHeader(http.StatusOK)
		case "/rest/api/2/issue":
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(JiraIssueResponse{ID: "1", Key: "TEST-1"})
		case "/rest/api/2/issue/TEST-1/remotelink":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewJiraClient(server.URL, "test-token")
	ctx := context.Background()

	err := client.GetProject(ctx, "PROJ")
	if err != nil {
		t.Errorf("Unexpected error in GetProject: %v", err)
	}

	payload := &JiraIssuePayload{Fields: map[string]interface{}{"summary": "test"}}
	resp, status, err := client.CreateIssue(ctx, payload)
	if err != nil {
		t.Errorf("Unexpected error in CreateIssue: %v", err)
	}
	if status != http.StatusCreated || resp.Key != "TEST-1" {
		t.Errorf("Unexpected response: %v, %v", status, resp)
	}

	err = client.CreateRemoteLink(ctx, "TEST-1", &JiraRemoteLinkPayload{})
	if err != nil {
		t.Errorf("Unexpected error in CreateRemoteLink: %v", err)
	}
}

func TestConfluenceClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Authorization"), "Bearer test-token") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		
		switch r.URL.Path {
		case "/rest/api/space/SPACE":
			w.WriteHeader(http.StatusOK)
		case "/rest/api/content":
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(ConfluenceContentResponse{ID: "123", Title: "test"})
		case "/rest/api/content/123/child/attachment":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewConfluenceClient(server.URL, "test-token")
	ctx := context.Background()

	err := client.GetSpace(ctx, "SPACE")
	if err != nil {
		t.Errorf("Unexpected error in GetSpace: %v", err)
	}

	payload := &ConfluenceContentPayload{Title: "test"}
	resp, status, err := client.CreateContent(ctx, payload)
	if err != nil {
		t.Errorf("Unexpected error in CreateContent: %v", err)
	}
	if status != http.StatusCreated || resp.ID != "123" {
		t.Errorf("Unexpected response: %v, %v", status, resp)
	}

	err = client.CreateAttachment(ctx, "123", "", "test.svg", strings.NewReader("<svg></svg>"))
	if err != nil {
		t.Errorf("Unexpected error in CreateAttachment: %v", err)
	}
}
