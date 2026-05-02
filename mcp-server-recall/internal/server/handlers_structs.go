package server

import "mcp-server-recall/internal/memory"

type SearchEcosystemInput struct {
	Query      string `json:"query,omitempty"`
	Package    string `json:"package,omitempty"`
	SymbolType string `json:"symbol_type,omitempty"`
	Interface  string `json:"interface,omitempty"`
	Receiver   string `json:"receiver,omitempty"`
	Domain     string `json:"domain,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

type GetEcosystemInput struct {
	Key string `json:"key"`
}

type DeleteMemoriesInput struct {
	Key      string `json:"key"`
	Category string `json:"category,omitempty"`
	All      bool   `json:"all,omitempty"`
}

type HarvestStandardsInput struct {
	TargetPath string `json:"target_path"`
}

type HarvestProjectsInput struct {
	TargetPath string `json:"target_path"`
}

type ListProjectCategoriesInput struct {
	Package    string `json:"package,omitempty"`
	SymbolType string `json:"symbol_type,omitempty"`
}

type SearchProjectsInput struct {
	Query      string `json:"query,omitempty"`
	Package    string `json:"package,omitempty"`
	SymbolType string `json:"symbol_type,omitempty"`
	Interface  string `json:"interface,omitempty"`
	Receiver   string `json:"receiver,omitempty"`
	Domain     string `json:"domain,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

type GetProjectInput struct {
	Key string `json:"key"`
}

type DeleteProjectsInput struct {
	Category       string `json:"category,omitempty"`
	Package        string `json:"package,omitempty"`
	CategoryNumber int    `json:"category_number,omitempty"`
	All            bool   `json:"all,omitempty"`
}

type ListStandardsCategoriesInput struct {
	Package    string `json:"package,omitempty"`
	SymbolType string `json:"symbol_type,omitempty"`
}

type SearchStandardsInput struct {
	Query      string `json:"query,omitempty"`
	Package    string `json:"package,omitempty"`
	SymbolType string `json:"symbol_type,omitempty"`
	Interface  string `json:"interface,omitempty"`
	Receiver   string `json:"receiver,omitempty"`
	Domain     string `json:"domain,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

type GetStandardInput struct {
	Key string `json:"key"`
}

type DeleteStandardsInput struct {
	Category       string `json:"category,omitempty"`
	Package        string `json:"package,omitempty"`
	CategoryNumber int    `json:"category_number,omitempty"`
	All            bool   `json:"all,omitempty"`
}

type ListCategoriesInput struct {
	Filename string `json:"filename,omitempty"`
}

type IngestFilesInput struct {
	Path string `json:"path"`
}

type HarvestInput struct {
	TargetDomain string `json:"target_domain"`
}

type ContextVacuumInput struct {
	Namespace        string  `json:"namespace"`
	TargetOutcome    string  `json:"target_outcome"`
	FlattenThreshold int     `json:"flatten_threshold"`
	DaysOld          int     `json:"days_old"`
	DedupThreshold   float64 `json:"dedup_threshold"`
	Category         string  `json:"category"`
	ReportOnly       bool    `json:"report_only"`
}

type GetLogsInput struct {
	MaxLines int `json:"max_lines"`
}

type RememberInput struct {
	Title          string   `json:"title"`
	Key            string   `json:"key"`
	Value          string   `json:"value"`
	Category       string   `json:"category"`
	Tags           []string `json:"tags"`
	DedupThreshold float64  `json:"dedup_threshold,omitempty"`
}

type SaveSessionsInput struct {
	ServerID     string `json:"server_id"`
	ProjectID    string `json:"project_id"`
	Outcome      string `json:"outcome"`
	SessionID    string `json:"session_id"`
	Model        string `json:"model,omitempty"`
	TokenSpend   int    `json:"token_spend,omitempty"`
	TraceContext string `json:"trace_context,omitempty"`
	StateData    string `json:"state_data"`
}

type RecallInput struct {
	Key string `json:"key"`
}

type SearchMemoriesInput struct {
	Query string `json:"query"`
	Tag   string `json:"tag"`
	Limit int    `json:"limit"`
}

type SearchSessionsInput struct {
	Query        string `json:"query,omitempty"`
	ProjectID    string `json:"project_id,omitempty"`
	ServerID     string `json:"server_id,omitempty"`
	Outcome      string `json:"outcome,omitempty"`
	TraceContext string `json:"trace_context,omitempty"`
	Limit        int    `json:"limit,omitempty"`
}

type RecallRecentInput struct {
	Count int `json:"count"`
}

type ListMemoriesInput struct {}

type ListSessionsInput struct {
	ProjectID       string `json:"project_id,omitempty"`
	ServerID        string `json:"server_id,omitempty"`
	Outcome         string `json:"outcome,omitempty"`
	TraceContext    string `json:"trace_context,omitempty"`
	Limit           int    `json:"limit,omitempty"`
	TruncateContent bool   `json:"truncate_content,omitempty"`
}

type GetSessionsInput struct {
	Key       string `json:"key,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

type DeleteSessionsInput struct {
	Key       string `json:"key,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	All       bool   `json:"all,omitempty"`
}

type GetMetricsInput struct {}
type ReloadCacheInput struct {}
type ForgetInput struct { Key string `json:"key"` }

type BatchRememberInput struct {
	Entries []memory.BatchEntry `json:"entries"`
}

type BatchRecallInput struct {
	Keys []string `json:"keys"`
}

type ExportMemoriesInput struct {
	Filename string `json:"filename"`
}

type ImportMemoriesInput struct {
	Filename string `json:"filename"`
}
