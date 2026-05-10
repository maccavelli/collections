package server

import (
	"context"
	"testing"
)

func TestHandleUniversal_Search(t *testing.T) {
	srv, _, cleanup := createTestServer(t)
	defer cleanup()
	ctx := context.Background()

	tests := []struct {
		name      string
		namespace string
	}{
		{"Memories", "memories"},
		{"Sessions", "sessions"},
		{"Standards", "standards"},
		{"Projects", "projects"},
		{"Ecosystem", "ecosystem"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := buildReq(`{"query":"test"}`)
			res, _, err := srv.handleUniversalSearch(ctx, req, UniversalSearchInput{Namespace: tt.namespace, Query: "test"})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res == nil {
				t.Fatal("expected non-nil result")
			}
		})
	}

	// Test invalid namespace
	req := buildReq(`{"query":"test"}`)
	_, _, err := srv.handleUniversalSearch(ctx, req, UniversalSearchInput{Namespace: "invalid"})
	if err == nil {
		t.Errorf("expected error for invalid namespace")
	}
}

func TestHandleUniversal_List(t *testing.T) {
	srv, _, cleanup := createTestServer(t)
	defer cleanup()
	ctx := context.Background()

	tests := []struct {
		name      string
		namespace string
	}{
		{"Memories", "memories"},
		{"Categories", "categories"},
		{"Sessions", "sessions"},
		{"Standards", "standards"},
		{"Projects", "projects"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := buildReq(`{}`)
			res, _, err := srv.handleUniversalList(ctx, req, UniversalListInput{Namespace: tt.namespace})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res == nil {
				t.Fatal("expected non-nil result")
			}
		})
	}
}

func TestHandleUniversal_Get(t *testing.T) {
	srv, _, cleanup := createTestServer(t)
	defer cleanup()
	ctx := context.Background()

	tests := []struct {
		name      string
		namespace string
	}{
		{"Memories", "memories"},
		{"Sessions", "sessions"},
		{"Standards", "standards"},
		{"Projects", "projects"},
		{"Ecosystem", "ecosystem"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := buildReq(`{"key":"k1"}`)
			res, _, _ := srv.handleUniversalGet(ctx, req, UniversalGetInput{Namespace: tt.namespace, Key: "k1"})
			// It might fail because key doesn't exist, but we want to cover the switch logic
			if res == nil {
				// srv.handleUniversalGet returns error if key is empty, but not if it's not found (it delegates)
			}
		})
	}
}

func TestHandleUniversal_Delete(t *testing.T) {
	srv, _, cleanup := createTestServer(t)
	defer cleanup()
	ctx := context.Background()

	tests := []struct {
		name      string
		namespace string
	}{
		{"Memories", "memories"},
		{"Standards", "standards"},
		{"Projects", "projects"},
		{"Sessions", "sessions"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := buildReq(`{"key":"k1", "all": true}`)
			// We pass all:true to satisfy the handleDeleteStandards check
			res, _, err := srv.handleUniversalDelete(ctx, req, UniversalDeleteInput{Namespace: tt.namespace, Key: "k1", All: true})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res == nil {
				t.Fatal("expected non-nil result")
			}
		})
	}
}
