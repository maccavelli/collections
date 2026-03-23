package tags

import (
	"context"
	"fmt"
	"go/ast"
	"mcp-server-go-refactor/internal/registry"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/tools/go/packages"
)

// Tool implements the tag manager tool.
type Tool struct{}

// Register adds the tag manager tool to the registry.
func Register() {
	registry.Global.Register(&Tool{})
}

func (t *Tool) Metadata() mcp.Tool {
	return mcp.NewTool("go_tag_manager",
		mcp.WithDescription("Standardizes or transforms field tags across a struct."),
		mcp.WithString("pkg", mcp.Description("The package path"), mcp.Required()),
		mcp.WithString("structName", mcp.Description("The name of the struct"), mcp.Required()),
		mcp.WithString("caseFormat", mcp.Description("Target case format (e.g., camel, snake)"), mcp.Required()),
		mcp.WithString("targetTag", mcp.Description("The tag key to transform (e.g., json, yaml)"), mcp.Required()),
	)
}

func (t *Tool) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pkg := request.GetString("pkg", "")
	structName := request.GetString("structName", "")
	caseFormat := request.GetString("caseFormat", "")
	targetTag := request.GetString("targetTag", "")
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
	cfg := &packages.Config{
		Mode:    packages.NeedName | packages.NeedSyntax | packages.NeedTypes,
		Tests:   true,
		Context: ctx,
	}
	pkgs, err := packages.Load(cfg, pkgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load package: %v", err)
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no package found at %s", pkgPath)
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
