package main

// SearchResult represents a structured search result with rich metadata.
type SearchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source,omitempty"`
	Date        string `json:"date,omitempty"`
	Author      string `json:"author,omitempty"`
	ImageURL    string `json:"image_url,omitempty"`
	Thumbnail   string `json:"thumbnail,omitempty"`
	Duration    string `json:"duration,omitempty"`
	Publisher   string `json:"publisher,omitempty"`
	Info        string `json:"info,omitempty"`
}

// SearchResponse represents the structured payload returned by search tools.
// Type is set once at the response level since all results share the same type.
// Query and count are omitted — the agent already has both from its own call context.
type SearchResponse struct {
	Type    string         `json:"type"`
	Results []SearchResult `json:"results"`
}
