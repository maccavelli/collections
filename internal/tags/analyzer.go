package tags

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"log/slog"
	"mcp-server-go-refactor/internal/dstutil"
	"mcp-server-go-refactor/internal/engine"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/models"
	"mcp-server-go-refactor/internal/registry"
	"mcp-server-go-refactor/internal/util"
	"os"
	"strings"

	"github.com/dave/dst"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool implements the tag manager tool.
type Tool struct {
	Engine *engine.Engine
}

func (t *Tool) Name() string {
	return "go_tag_manager"
}

func (t *Tool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: MUTATOR] STRUCT TAG MANAGER: Fixes structural tags (json, yaml) guaranteeing formatted properties to ensure standardized casing across all struct definitions. Validates syntax and enforces naming conventions. Requires struct name from prior discovery. Produces tag modification plan or applies rewritten tags. [Routing Tags: struct-tags, json-tags, yaml-tags, format-struct, fix-casing]",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pkg": map[string]any{
					"type":        "string",
					"description": "The package path",
				},
				"structName": map[string]any{
					"type":        "string",
					"description": "The name of the struct",
				},
				"caseFormat": map[string]any{
					"type":        "string",
					"description": "Target case format (e.g., camel, snake). Defaults to snake.",
				},
				"targetTag": map[string]any{
					"type":        "string",
					"description": "The tag key to transform (e.g., json, yaml). Defaults to json.",
				},
				"rewrite": map[string]any{
					"type":        "boolean",
					"description": "If true, automatically updates the source code (comment-safe).",
				},
			},
			"required": []string{"pkg", "structName"},
		},
	}, t.Handle)
}

// Register adds the tag manager tool to the registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&Tool{Engine: eng})
}

type TagInput struct {
	models.UniversalPipelineInput
}

func (t *Tool) Handle(ctx context.Context, req *mcp.CallToolRequest, input TagInput) (*mcp.CallToolResult, any, error) {
	var session *engine.Session

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine != nil && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	if t.Engine != nil {
		session = t.Engine.LoadSession(ctx, input.Target)
	}

	caseFormat := "snake"
	if cf, ok := input.Flags["caseFormat"].(string); ok && cf != "" {
		caseFormat = cf
	}
	targetTag := "json"
	if tt, ok := input.Flags["targetTag"].(string); ok && tt != "" {
		targetTag = tt
	}
	rewrite := false
	if rw, ok := input.Flags["rewrite"].(bool); ok {
		rewrite = rw
	}

	if rewrite {
		err := ApplyTags(ctx, input.Target, input.Context, caseFormat, targetTag)
		if err != nil {
			res := &mcp.CallToolResult{}
			res.SetError(err)
			return res, nil, nil
		}
		summary := fmt.Sprintf("Successfully updated struct %s tags in %s", input.Context, input.Target)

		if session != nil {
			if session.Metadata == nil {
				session.Metadata = make(map[string]any)
			}
			var diags []string
			if d, ok := session.Metadata["diagnostics"].([]string); ok {
				diags = d
			}
			session.Metadata["diagnostics"] = append(diags, summary)
			t.Engine.SaveSession(session)

			// Publish tag management trace to recall sessions matrix.
			if recallAvailable {
				t.Engine.PublishSessionToRecall(ctx, input.SessionID, input.Target, "tags_managed", "native", "go_tag_manager", "", session.Metadata)
			}
		}

		return &mcp.CallToolResult{}, struct {
			Summary string `json:"summary"`
		}{
			Summary: summary,
		}, nil
	}

	result, err := AnalyzeTags(ctx, input.Target, input.Context, caseFormat, targetTag)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	summary := fmt.Sprintf("Tag analysis for struct %s in %s (%d fields)", input.Context, input.Target, len(result.Modifications))

	if session != nil {
		if session.Metadata == nil {
			session.Metadata = make(map[string]any)
		}

		if recallAvailable {
			tagStds := t.Engine.EnsureRecallCache(ctx, session, "tag_conventions", "search", map[string]interface{}{"namespace": "ecosystem",
				"query": "Go struct tag conventions, JSON serialization standards, YAML tag patterns, and validation tag standards for " + input.Target,
				"limit": 10,
			})
			session.Metadata["recall_cache_tags"] = tagStds

			if tagStds != "" {
				summary += fmt.Sprintf("\n\n[Struct Tag Convention Standards]: %s", tagStds)
			}
		}

		var diags []string
		if d, ok := session.Metadata["diagnostics"].([]string); ok {
			diags = d
		}
		session.Metadata["diagnostics"] = append(diags, summary)
		t.Engine.SaveSession(session)

		if recallAvailable {
			t.Engine.PublishSessionToRecall(ctx, input.SessionID, input.Target, "tags_managed", "native", "go_tag_manager", "", session.Metadata)
		}
	}

	return &mcp.CallToolResult{}, struct {
		Summary string     `json:"summary"`
		Data    *TagResult `json:"data"`
	}{
		Summary: summary,
		Data:    result,
	}, nil
}

// TagModification describes a single field's tag transformation.
type TagModification struct {
	Field        string `json:"Field"`
	OriginalTag  string `json:"OriginalTag"`
	SuggestedTag string `json:"SuggestedTag"`
}

// TagResult contains all tag transformations for a struct.
type TagResult struct {
	StructName    string            `json:"StructName"`
	Modifications []TagModification `json:"Modifications"`
}

// AnalyzeTags calculates standard formatting tags (e.g., json, yaml) for standard cases.
func AnalyzeTags(ctx context.Context, pkgPath string, structName string, caseFormat string, targetTag string) (*TagResult, error) {
	pkgs, err := loader.LoadPackages(ctx, pkgPath, loader.DefaultMode)
	if err != nil {
		return nil, err
	}

	var targetStruct *ast.TypeSpec
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				ts, ok := n.(*ast.TypeSpec)
				if !ok || ts.Name.Name != structName {
					return true
				}
				if _, ok := ts.Type.(*ast.StructType); ok {
					targetStruct = ts
					return false
				}
				return true
			})
			if targetStruct != nil {
				break
			}
		}
		if targetStruct != nil {
			break
		}
	}

	if targetStruct == nil {
		return nil, fmt.Errorf("struct %s not found in package %s", structName, pkgPath)
	}

	st := targetStruct.Type.(*ast.StructType)
	mods := []TagModification{}

	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			continue // Embedded field
		}
		fieldName := field.Names[0].Name
		if !ast.IsExported(fieldName) {
			continue
		}

		original := ""
		if field.Tag != nil {
			original = field.Tag.Value
		}

		formatted := formatCase(fieldName, caseFormat)
		suggested := fmt.Sprintf("`%s:\"%s\"`", targetTag, formatted)

		mods = append(mods, TagModification{
			Field:        fieldName,
			OriginalTag:  original,
			SuggestedTag: suggested,
		})
	}

	return &TagResult{
		StructName:    structName,
		Modifications: mods,
	}, nil
}

// ApplyTags performs automated tag rewriting using DST to preserve comments.
func ApplyTags(ctx context.Context, pkgPath string, structName string, caseFormat string, targetTag string) error {
	res, err := loader.Discover(ctx, pkgPath)
	if err != nil {
		return err
	}

	pkgs, err := loader.LoadPackages(ctx, pkgPath, loader.DefaultMode)
	if err != nil {
		return err
	}

	for _, pkg := range pkgs {
		for _, astFile := range pkg.Syntax {
			// Convert to DST
			dstFile, err := dstutil.ToDST(pkg.Fset, astFile)
			if err != nil {
				continue
			}

			modified := false
			dst.Inspect(dstFile, func(n dst.Node) bool {
				ts, ok := n.(*dst.TypeSpec)
				if !ok || ts.Name.Name != structName {
					return true
				}
				st, ok := ts.Type.(*dst.StructType)
				if !ok {
					return true
				}

				for _, field := range st.Fields.List {
					if len(field.Names) == 0 || !ast.IsExported(field.Names[0].Name) {
						continue
					}
					fieldName := field.Names[0].Name
					formatted := formatCase(fieldName, caseFormat)
					newTag := fmt.Sprintf("`%s:\"%s\"`", targetTag, formatted)

					if field.Tag == nil || field.Tag.Value != newTag {
						field.Tag = &dst.BasicLit{Kind: token.STRING, Value: newTag}
						modified = true
					}
				}
				return false
			})

			if modified {
				data, err := dstutil.WriteFile(dstFile)
				if err != nil {
					return fmt.Errorf("write file: %w", err)
				}
				filename := pkg.Fset.Position(astFile.Pos()).Filename
				if err := res.Runner.WriteFileAtomic(filename, data); err != nil {
					return fmt.Errorf("atomic write %s: %w", filename, err)
				}
			}
		}
	}

	return nil
}

func formatCase(s string, format string) string {
	// Simple case transformation logic
	words := splitWords(s)
	switch strings.ToLower(format) {
	case "snake":
		return strings.Join(words, "_")
	case "camel":
		if len(words) == 0 {
			return ""
		}
		res := strings.ToLower(words[0])
		for i := 1; i < len(words); i++ {
			res += strings.Title(words[i])
		}
		return res
	case "pascal":
		res := ""
		for _, w := range words {
			res += strings.Title(w)
		}
		return res
	case "kebab":
		return strings.Join(words, "-")
	default:
		return strings.ToLower(s)
	}
}

func splitWords(s string) []string {
	var words []string
	var current []rune
	for _, r := range s {
		if r >= 'A' && r <= 'Z' && len(current) > 0 {
			words = append(words, strings.ToLower(string(current)))
			current = []rune{r}
		} else {
			current = append(current, r)
		}
	}
	if len(current) > 0 {
		words = append(words, strings.ToLower(string(current)))
	}
	return words
}
