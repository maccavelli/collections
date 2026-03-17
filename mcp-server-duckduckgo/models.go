package main

// SearchResult represents a structured search result with rich metadata.
type SearchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Source      string `json:"source,omitempty"`
	Date        string `json:"date,omitempty"`
	Author      string `json:"author,omitempty"`
	ImageURL    string `json:"image_url,omitempty"`
	Thumbnail   string `json:"thumbnail,omitempty"`
	Duration    string `json:"duration,omitempty"`
	Publisher   string `json:"publisher,omitempty"`
	Type        string            `json:"type,omitempty"` // web, news, image, video, book
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// SearchResponse represents the structured payload returned by search tools.
type SearchResponse struct {
	Query   string         `json:"query"`
	Results []SearchResult `json:"results"`
	Count   int            `json:"count"`
}
