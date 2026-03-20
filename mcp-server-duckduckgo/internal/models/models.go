package models

// SearchResult represents a single search result from DuckDuckGo or mirrors.
type SearchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source,omitempty"`
	Date        string `json:"date,omitempty"`
	ImageURL    string `json:"image_url,omitempty"`
	Thumbnail   string `json:"thumbnail,omitempty"`
	Duration    string `json:"duration,omitempty"`
	Publisher   string `json:"publisher,omitempty"`
	Author      string `json:"author,omitempty"`
	Info        string `json:"info,omitempty"`
}

// SearchResponse represents the formatted response for the MCP tool.
type SearchResponse struct {
	Type    string         `json:"type"`
	Results []SearchResult `json:"results"`
}
