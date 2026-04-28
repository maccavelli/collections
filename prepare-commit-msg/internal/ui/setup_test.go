package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"prepare-commit-msg/internal/config"
	"testing"

	"github.com/AlecAivazis/survey/v2"
)

func TestRunSetup_Success(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	os.MkdirAll(filepath.Join(tmpDir, ".config", "prepare-commit-msg"), 0755)

	conf := &config.Config{Providers: make(map[string]config.ProviderConfig)}

	oldEnv := osGetenv
	defer func() { osGetenv = oldEnv }()
	osGetenv = func(k string) string {
		if k == "GEMINI_API_KEY" {
			return "test-key"
		}
		return ""
	}

	oldAsk := surveyAskOne
	defer func() { surveyAskOne = oldAsk }()
	
	askCalls := 0
	surveyAskOne = func(p survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
		askCalls++
		switch askCalls {
		case 1:
			*response.(*string) = "gemini"
		case 2:
			*response.(*string) = "Other"
		case 3:
			*response.(*string) = "my-custom-model"
		}
		return nil
	}

	err := RunSetup(context.Background(), conf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conf.ActiveProvider != "gemini" {
		t.Errorf("expected gemini")
	}
	if conf.Providers["gemini"].Model != "my-custom-model" {
		t.Errorf("expected my-custom-model")
	}
	if conf.Providers["gemini"].APIKey != "test-key" {
		t.Errorf("expected test-key")
	}
}

func TestRunSetup_Errors(t *testing.T) {
	conf := &config.Config{Providers: make(map[string]config.ProviderConfig)}

	oldAsk := surveyAskOne
	defer func() { surveyAskOne = oldAsk }()
	
	// Error on provider choice
	surveyAskOne = func(p survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
		return fmt.Errorf("provider select failed")
	}
	if err := RunSetup(context.Background(), conf); err == nil {
		t.Error("expected error")
	}
	
	// Error on API key input
	surveyAskOne = func(p survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
		if _, ok := p.(*survey.Select); ok && p.(*survey.Select).Message == "Choose LLM Provider:" {
			*response.(*string) = "openai"
			return nil
		}
		return fmt.Errorf("api key failed")
	}
	if err := RunSetup(context.Background(), conf); err == nil {
		t.Error("expected error")
	}
}

func TestRunSetup_FallbackAndOtherProviders(t *testing.T) {
	// mock home dir for config save
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	conf := &config.Config{Providers: make(map[string]config.ProviderConfig)}

	oldEnv := osGetenv
	defer func() { osGetenv = oldEnv }()
	osGetenv = func(k string) string { return "" }

	oldAsk := surveyAskOne
	defer func() { surveyAskOne = oldAsk }()
	
	askCalls := 0
	surveyAskOne = func(p survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
		askCalls++
		switch askCalls {
		case 1:
			*response.(*string) = "anthropic" // no env key
		case 2:
			*response.(*string) = "manual-key" // password prompt
		case 3:
			*response.(*string) = "claude-3-5-sonnet-latest" // model select
		}
		return nil
	}

	err := RunSetup(context.Background(), conf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conf.ActiveProvider != "anthropic" {
		t.Errorf("expected anthropic")
	}
	if conf.Providers["anthropic"].APIKey != "manual-key" {
		t.Errorf("expected manual-key")
	}
	
	// Verify fallbacks were populated // 111.4
	fb := conf.Providers["anthropic"].FallbackModels
	if len(fb) == 0 {
		t.Error("expected fallbacks to be populated")
	}
}

func TestRunSetup_CoverageBranches(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)
	conf := &config.Config{Providers: make(map[string]config.ProviderConfig)}

	oldAsk := surveyAskOne
	defer func() { surveyAskOne = oldAsk }()
	oldEnv := osGetenv
	defer func() { osGetenv = oldEnv }()
	osGetenv = func(k string) string { return "" }

	// Test openai path (covers line 82)
	surveyAskOne = func(p survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
		switch pt := p.(type) {
		case *survey.Select:
			if pt.Message == "Choose LLM Provider:" {
				*response.(*string) = "openai"
			} else {
				*response.(*string) = "gpt-4o"
			}
		case *survey.Password:
			*response.(*string) = "test-key"
		}
		return nil
	}
	_ = RunSetup(context.Background(), conf)

	// Test gemini with standard model (covers line 88 and fallback exclusion)
	surveyAskOne = func(p survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
		switch pt := p.(type) {
		case *survey.Select:
			if pt.Message == "Choose LLM Provider:" {
				*response.(*string) = "gemini"
			} else {
				*response.(*string) = "gemini-2.5-flash-lite"
			}
		case *survey.Password:
			*response.(*string) = "test-key"
		}
		return nil
	}
	_ = RunSetup(context.Background(), conf)

	// Test 'Other' model input error
	surveyAskOne = func(p survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
		switch pt := p.(type) {
		case *survey.Select:
			if pt.Message == "Choose LLM Provider:" {
				*response.(*string) = "gemini"
			} else {
				*response.(*string) = "Other"
			}
		case *survey.Password:
			*response.(*string) = "test-key"
		case *survey.Input:
			return fmt.Errorf("mock input error")
		}
		return nil
	}
	if err := RunSetup(context.Background(), conf); err == nil {
		t.Error("expected error from failed 'Other' input")
	}
}
