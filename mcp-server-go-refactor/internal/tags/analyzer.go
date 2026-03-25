package tags

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"mcp-server-go-refactor/internal/dstutil"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/registry"
	"strings"

	"github.com/dave/dst"
	"github.com/mark3labs/mcp-go/mcp"
)

// Tool implements the tag manager tool.
type Tool struct{}

// Register adds the tag manager tool to the registry.
func Register() {
	registry.Global.Register(&Tool{})
}

func (t *Tool) Metadata() mcp.Tool {
	return mcp.NewTool("go_tag_manager",
		mcp.WithDescription("Provides automated transformation of struct field tags (e.g., json, yaml, mapstructure) across an entire type. It supports bulk case conversion (snake_case, camelCase, etc.) and is essential for standardizing API data models or migrating between different serialization formats."),
		mcp.WithString("pkg", mcp.Description("The package path"), mcp.Required()),
		mcp.WithString("structName", mcp.Description("The name of the struct"), mcp.Required()),
		mcp.WithString("caseFormat", mcp.Description("Target case format (e.g., camel, snake)"), mcp.Required()),
		mcp.WithString("targetTag", mcp.Description("The tag key to transform (e.g., json, yaml)"), mcp.Required()),
		mcp.WithBoolean("rewrite", mcp.Description("If true, automatically updates the source code (comment-safe).")),
	)
}

func (t *Tool) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pkg := request.GetString("pkg", "")
	structName := request.GetString("structName", "")
	caseFormat := request.GetString("caseFormat", "")
	targetTag := request.GetString("targetTag", "")
	rewrite := request.GetBool("rewrite", false)

	if rewrite {
		err := ApplyTags(ctx, pkg, structName, caseFormat, targetTag)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Successfully updated struct %s tags in %s", structName, pkg)), nil
	}

	result, err := AnalyzeTags(ctx, pkg, structName, caseFormat, targetTag)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("%+v", result)), nil
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
