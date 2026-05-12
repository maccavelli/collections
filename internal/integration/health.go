// Package integration provides functionality for the integration subsystem.
package integration

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/spf13/viper"
	"gitlab.com/gitlab-org/api/client-go"
	"mcp-server-magicdev/internal/db"
)

// VerifyConnectivity performs fail-fast verification of environment connectivity
// against GitLab, Jira, and Confluence. If confluence.mock is enabled, it bypasses Confluence.
func VerifyConnectivity(store *db.Store) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. GitLab Check
	if viper.GetBool("git.mock") {
		slog.Info("integration.health: gitlab connectivity mocked (bypassed)")
	} else {
		var gitToken string
		if store != nil {
			gitToken, _ = store.GetSecret("gitlab")
			if gitToken == "" {
				gitToken, _ = store.GetSecret("git")
			}
		}
		if gitToken == "" {
			return fmt.Errorf("gitlab token is required but not configured (run: mcp-server-magicdev token reconfigure)")
		}

		gitBaseURL := viper.GetString("git.server_url")

		var glOpts []gitlab.ClientOptionFunc
		if gitBaseURL != "" {
			glOpts = append(glOpts, gitlab.WithBaseURL(gitBaseURL))
		}

		gl, err := gitlab.NewClient(gitToken, glOpts...)
		if err != nil {
			return fmt.Errorf("failed to initialize gitlab client: %w", err)
		}

		_, _, err = gl.Users.CurrentUser(gitlab.WithContext(ctx))
		if err != nil {
			return fmt.Errorf("gitlab connectivity check failed: %w", err)
		}
		slog.Info("integration.health: gitlab connectivity verified")
	}

	// 2. Jira Check
	if viper.GetBool("jira.mock") {
		slog.Info("integration.health: jira connectivity mocked (bypassed)")
	} else {
		jiraURL := viper.GetString("jira.url")
		if jiraURL == "" {
			return fmt.Errorf("jira.url is required but not configured")
		}

		var jiraToken string
		if store != nil {
			jiraToken, _ = store.GetSecret("jira")
		}
		if jiraToken == "" {
			return fmt.Errorf("jira token is required but not configured (run: mcp-server-magicdev token reconfigure)")
		}

		jc := NewJiraClient(jiraURL, jiraToken)

		jProjectKey := viper.GetString("jira.project")
		if jProjectKey == "" {
			jProjectKey = "PROJ"
		}
		if err := jc.GetProject(ctx, jProjectKey); err != nil {
			return fmt.Errorf("jira connectivity check failed (project %s): %w", jProjectKey, err)
		}
		slog.Info("integration.health: jira connectivity verified")
	}

	// 3. Confluence Check
	if viper.GetBool("confluence.mock") {
		slog.Info("integration.health: confluence connectivity mocked (bypassed)")
		return nil
	}

	confluenceURL := viper.GetString("confluence.url")
	if confluenceURL == "" {
		return fmt.Errorf("confluence.url is required but not configured")
	}

	var confluenceToken string
	if store != nil {
		confluenceToken, _ = store.GetSecret("confluence")
	}
	if confluenceToken == "" {
		return fmt.Errorf("confluence token is required but not configured (run: mcp-server-magicdev token reconfigure)")
	}

	cc := NewConfluenceClient(confluenceURL, confluenceToken)

	cSpaceKey := viper.GetString("confluence.space")
	if cSpaceKey == "" {
		cSpaceKey = "SPACE"
	}
	if err := cc.GetSpace(ctx, cSpaceKey); err != nil {
		return fmt.Errorf("confluence connectivity check failed (space %s): %w", cSpaceKey, err)
	}
	slog.Info("integration.health: confluence connectivity verified")

	return nil
}
