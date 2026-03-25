package models

import (
	"fmt"
	"strings"
)

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

// SearchMetadata represents machine-readable metadata about the search.
type SearchMetadata struct {
	Query      string `json:"query"`
	TotalCount int    `json:"total_count"`
	SearchType string `json:"search_type"`
}

// SearchResponse represents the formatted response for the MCP tool.
// It supports a hybrid format with both structured JSON and Markdown content.
type SearchResponse struct {
	Type      string          `json:"type"`
	Metadata  *SearchMetadata `json:"metadata,omitempty"`
	ResultsMD string          `json:"results_md,omitempty"`
	Results   []SearchResult  `json:"results,omitempty"`
}

// SearchResponse20 represents the Structured JSON 2.0 format for media results.
type SearchResponse20 struct {
	Version  string          `json:"version"`
	Metadata *SearchMetadata `json:"metadata"`
	Results  []MediaResult20 `json:"results"`
}

// MediaResult20 represents a single result in the Structured JSON 2.0 format.
type MediaResult20 struct {
	Title        string `json:"title"`
	PageURL      string `json:"page_url"`
	MediaURL     string `json:"media_url,omitempty"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	Duration     string `json:"duration,omitempty"`
	Publisher    string    `json:"publisher,omitempty"`
	Source       string `json:"source,omitempty"`
}

// ToMarkdown converts search results to a beautifully formatted Markdown string.
func (r *SearchResponse) ToMarkdown() string {
	if len(r.Results) == 0 {
		return "_No results found for the requested query._"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Search Results: %s (%s)\n\n", r.Metadata.Query, strings.Title(r.Type)))

	for _, res := range r.Results {
		switch r.Type {
		case "images":
			sb.WriteString(fmt.Sprintf("#### ![%s](%s)\n", res.Title, res.Thumbnail))
			sb.WriteString(fmt.Sprintf("**[%s](%s)**\n", res.Title, res.URL))
			if res.Publisher != "" {
				sb.WriteString(fmt.Sprintf("*Publisher: %s*\n", res.Publisher))
			}
		case "videos":
			sb.WriteString(fmt.Sprintf("#### [%s](%s)\n", res.Title, res.URL))
			if res.Duration != "" {
				sb.WriteString(fmt.Sprintf("*Duration: %s*\n", res.Duration))
			}
			if res.Thumbnail != "" {
				sb.WriteString(fmt.Sprintf("![Thumbnail](%s)\n", res.Thumbnail))
			}
			if res.Publisher != "" {
				sb.WriteString(fmt.Sprintf("*Publisher: %s*\n", res.Publisher))
			}
		case "books":
			sb.WriteString(fmt.Sprintf("#### [%s](%s)\n", res.Title, res.URL))
			if res.Author != "" {
				sb.WriteString(fmt.Sprintf("**Author:** %s\n", res.Author))
			}
			if res.Info != "" {
				sb.WriteString(fmt.Sprintf("> %s\n", res.Info))
			}
		default: // web and news
			sb.WriteString(fmt.Sprintf("#### [%s](%s)\n", res.Title, res.URL))
			if res.Description != "" {
				sb.WriteString(fmt.Sprintf("> %s\n", res.Description))
			}
			metaParts := []string{}
			if res.Source != "" {
				metaParts = append(metaParts, fmt.Sprintf("Source: %s", res.Source))
			}
			if res.Date != "" {
				metaParts = append(metaParts, fmt.Sprintf("Date: %s", res.Date))
			}
			if len(metaParts) > 0 {
				sb.WriteString(fmt.Sprintf("*%s*\n", strings.Join(metaParts, " | ")))
			}
		}
		sb.WriteString("\n---\n\n")
	}

	return sb.String()
}
