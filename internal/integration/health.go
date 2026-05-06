package integration

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ctreminiom/go-atlassian/v2/confluence"
	"github.com/ctreminiom/go-atlassian/v2/jira/v3"
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
	gitToken := viper.GetString("git.token")
	if gitToken == "" && store != nil {
		gitToken, _ = store.GetSecret("git")
	}
	if gitToken == "" {
		return fmt.Errorf("git.token is required but not configured")
	}

	gitBaseURL := viper.GetString("git.base_url")
	if gitBaseURL == "" {
		gitBaseURL = "https://gitlab.com/api/v4"
	}
	
	gl, err := gitlab.NewClient(gitToken, gitlab.WithBaseURL(gitBaseURL))
	if err != nil {
		return fmt.Errorf("failed to initialize gitlab client: %w", err)
	}

	_, _, err = gl.Users.CurrentUser(gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("gitlab connectivity check failed: %w", err)
	}
	slog.Info("integration.health: gitlab connectivity verified")

	// 2. Jira Check
	jiraURL := viper.GetString("jira.url")
	if jiraURL == "" {
		return fmt.Errorf("jira.url is required but not configured")
	}
	
	jiraToken := viper.GetString("jira.api_key")
	if jiraToken == "" && store != nil {
		jiraToken, _ = store.GetSecret("jira")
	}
	if jiraToken == "" {
		return fmt.Errorf("jira.api_key is required but not configured")
	}
	
	jiraEmail := viper.GetString("jira.email")
	if jiraEmail == "" {
		return fmt.Errorf("jira.email is required but not configured")
	}

	jClient, err := v3.New(nil, jiraURL)
	if err != nil {
		return fmt.Errorf("failed to initialize jira client: %w", err)
	}
	jClient.Auth.SetBasicAuth(jiraEmail, jiraToken)

	jProjectKey := viper.GetString("jira.project")
	if jProjectKey == "" {
		jProjectKey = "PROJ"
	}
	_, _, err = jClient.Project.Get(ctx, jProjectKey, nil)
	if err != nil {
		return fmt.Errorf("jira connectivity check failed (project %s): %w", jProjectKey, err)
	}
	slog.Info("integration.health: jira connectivity verified")

	// 3. Confluence Check
	if viper.GetBool("confluence.mock") {
		slog.Info("integration.health: confluence connectivity mocked (bypassed)")
		return nil
	}

	confluenceURL := viper.GetString("confluence.url")
	if confluenceURL == "" {
		return fmt.Errorf("confluence.url is required but not configured")
	}
	
	confluenceToken := viper.GetString("confluence.api_key")
	if confluenceToken == "" && store != nil {
		confluenceToken, _ = store.GetSecret("confluence")
	}
	if confluenceToken == "" {
		return fmt.Errorf("confluence.api_key is required but not configured")
	}

	cClient, err := confluence.New(nil, confluenceURL)
	if err != nil {
		return fmt.Errorf("failed to initialize confluence client: %w", err)
	}
	cClient.Auth.SetBasicAuth(jiraEmail, confluenceToken) // Atlassian usually uses the same email

	cSpaceKey := viper.GetString("confluence.space")
	if cSpaceKey == "" {
		cSpaceKey = "SPACE"
	}
	_, _, err = cClient.Space.Get(ctx, cSpaceKey, nil)
	if err != nil {
		return fmt.Errorf("confluence connectivity check failed (space %s): %w", cSpaceKey, err)
	}
	slog.Info("integration.health: confluence connectivity verified")

	return nil
}
