package loader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/tools/go/packages"
)

// DefaultMode provides the standard set of flags needed for most AST analysis.
const DefaultMode = packages.NeedName | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedImports

// LoadPackages provides a robust way to load Go packages, handling absolute paths
// by correctly setting the working directory for the build system query tool.
func LoadPackages(ctx context.Context, pkgPath string, mode packages.LoadMode) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode:    mode,
		Tests:   true,
		Context: ctx,
	}

	pattern := pkgPath
	// If the path is absolute, set the Dir so packages.Load can resolve the module root
	if filepath.IsAbs(pkgPath) {
		info, err := os.Stat(pkgPath)
		if err == nil {
			if info.IsDir() {
				cfg.Dir = pkgPath
				pattern = "."
			} else {
				cfg.Dir = filepath.Dir(pkgPath)
				pattern = "."
			}
		}
	}

	pkgs, err := packages.Load(cfg, pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to load package: %v", err)
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no package found for %s", pkgPath)
	}

	// Check for errors in loaded packages
	for _, p := range pkgs {
		if len(p.Errors) > 0 {
			return nil, fmt.Errorf("package load error: %v", p.Errors[0])
		}
	}

	return pkgs, nil
}
