package server

import (
	"context"
	"testing"
)

func TestHandleSessions_LifeCycle(t *testing.T) {
	srv, _, cleanup := createTestServer(t)
	defer cleanup()
	ctx := context.Background()

	// 1. Save Session
	saveReq := buildReq(`{"session_id":"s1", "state_data":"data"}`)
	saveArgs := SaveSessionsInput{
		SessionID: "s1",
		StateData: "data",
		ProjectID: "p1",
		ServerID:  "srv1",
		Outcome:   "success",
	}
	res, _, err := srv.handleSaveSessions(ctx, saveReq, saveArgs)
	if err != nil || res.IsError {
		t.Fatalf("SaveSessions failed: %v", err)
	}

	// 2. Get Session
	getReq := buildReq(`{"session_id":"s1"}`)
	getArgs := GetSessionsInput{SessionID: "s1"}
	res, _, err = srv.handleGetSessions(ctx, getReq, getArgs)
	if err != nil || res.IsError {
		t.Fatalf("GetSessions failed: %v", err)
	}

	// 3. List Sessions
	listReq := buildReq(`{"project_id":"p1"}`)
	listArgs := ListSessionsInput{ProjectID: "p1"}
	res, _, err = srv.handleListSessions(ctx, listReq, listArgs)
	if err != nil || res.IsError {
		t.Fatalf("ListSessions failed: %v", err)
	}

	// 4. Delete Sessions
	delReq := buildReq(`{"session_id":"s1"}`)
	delArgs := DeleteSessionsInput{SessionID: "s1"}
	res, _, err = srv.handleDeleteSessions(ctx, delReq, delArgs)
	if err != nil || res.IsError {
		t.Fatalf("DeleteSessions failed: %v", err)
	}
}

func TestHandleSearchSessions(t *testing.T) {
	srv, _, cleanup := createTestServer(t)
	defer cleanup()
	ctx := context.Background()

	// Seed session
	srv.handleSaveSessions(ctx, buildReq(`{}`), SaveSessionsInput{SessionID: "s2", StateData: "searchable state", ProjectID: "p2"})

	searchReq := buildReq(`{"query":"searchable"}`)
	searchArgs := SearchSessionsInput{Query: "searchable"}
	res, _, err := srv.handleSearchSessions(ctx, searchReq, searchArgs)
	if err != nil || res.IsError {
		t.Fatalf("SearchSessions failed: %v", err)
	}
}
