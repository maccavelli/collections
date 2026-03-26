package engine

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"mcp-server-brainstorm/internal/models"
	"golang.org/x/sync/errgroup"
)

// AnalyzeDiscovery scans the project for context and
// identifies initial gaps. It uses a depth-limited walk
// and skips common non-source directories.
func (e *Engine) AnalyzeDiscovery(
	ctx context.Context, path string,
) ([]models.Gap, error) {
	target := e.ResolvePath(path)
	g, gCtx := errgroup.WithContext(ctx)

	var (
		gaps        []models.Gap
		readmeFound bool
		testFound   bool
		stackGaps   []models.Gap
	)

	// 1. Scan for key files (README, tests) in parallel
	g.Go(func() error {
		rf, tf, err := e.scanForContextFiles(gCtx, target)
		if err != nil {
			return err
		}
		readmeFound, testFound = rf, tf
		return nil
	})

	// 2. Detect language/stack and analyze Go source in parallel
	g.Go(func() error {
		sg, err := e.analyzeStackAndSource(gCtx, target)
		if err != nil {
			return err
		}
		stackGaps = sg
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// 3. Aggregate Gaps
	if !readmeFound {
		gaps = append(gaps, models.Gap{
			Area: "CONTEXT",
			Description: "No README found." +
				" Project context is missing.",
			Severity: "CRITICAL",
		})
	}
	if !testFound {
		gaps = append(gaps, models.Gap{
			Area: "TESTING",
			Description: "No unit tests (_test.go) or test directory found. " +
				"Reliability is unverified.",
			Severity: "HIGH",
		})
	}
	gaps = append(gaps, stackGaps...)

	return gaps, nil
}

func (e *Engine) scanForContextFiles(ctx context.Context, target string) (bool, bool, error) {
	readmeFound := false
	testFound := false
	rootDepth := strings.Count(target, string(os.PathSeparator))

	err := filepath.WalkDir(target, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if d.IsDir() {
			depth := strings.Count(p, string(os.PathSeparator)) - rootDepth
			if depth > maxWalkDepth || skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}

		name := strings.ToUpper(d.Name())
		if strings.HasPrefix(name, "README") {
			readmeFound = true
		}
		if strings.HasSuffix(name, "_TEST.GO") || strings.Contains(strings.ToLower(p), "/test") {
			testFound = true
		}
		return nil
	})
	return readmeFound, testFound, err
}

func (e *Engine) analyzeStackAndSource(ctx context.Context, target string) ([]models.Gap, error) {
	var gaps []models.Gap
	goMod := filepath.Join(target, "go.mod")
	pkgJSON := filepath.Join(target, "package.json")
	reqTxt := filepath.Join(target, "requirements.txt")

	hasGo := fileExists(goMod)
	hasNode := fileExists(pkgJSON)
	hasPy := fileExists(reqTxt)

	if !hasGo && !hasNode && !hasPy {
		gaps = append(gaps, models.Gap{
			Area:        "TECH_STACK",
			Description: "Cannot detect language or stack.",
			Severity:    "RECOMMENDED",
		})
	}

	if hasGo {
		astGaps, err := e.inspector.AnalyzeDirectory(ctx, target)
		if err != nil {
			gaps = append(gaps, models.Gap{
				Area: "STABILITY",
				Description: fmt.Sprintf(
					"failed to analyze Go source: %v", err,
				),
				Severity: "RECOMMENDED",
			})
		} else {
			gaps = append(gaps, astGaps...)
		}
	}
	return gaps, nil
}

// DiscoverProject performs a unified discovery scan, identifying
// gaps and suggesting the next logical step. It returns a
// consolidated DiscoveryResponse with a narrative summary
// and formatted markdown.
func (e *Engine) DiscoverProject(
	ctx context.Context, path string, session *models.Session,
) (models.DiscoveryResponse, error) {
	gaps, err := e.AnalyzeDiscovery(ctx, path)
	if err != nil {
		return models.DiscoveryResponse{}, err
	}

	// Update session gaps for SuggestNextStep.
	session.Gaps = gaps
	nextStep, err := e.SuggestNextStep(ctx, session, "")
	if err != nil {
		return models.DiscoveryResponse{}, err
	}

	narrative := "Project discovery complete."
	if len(gaps) > 0 {
		narrative = fmt.Sprintf(
			"Discovery complete. Identified %d potential gaps.",
			len(gaps),
		)
	}

	// Generate Markdown Summary.
	var sb strings.Builder
	sb.WriteString("### Project Discovery Summary\n\n")
	if len(gaps) > 0 {
		sb.WriteString("| Area | Severity | Description |\n")
		sb.WriteString("| :--- | :---: | :--- |\n")
		for _, g := range gaps {
			sb.WriteString(fmt.Sprintf(
				"| %s | %s | %s |\n",
				g.Area, g.Severity, g.Description,
			))
		}
	} else {
		sb.WriteString("*No critical gaps identified.*\n")
	}
	sb.WriteString(fmt.Sprintf("\n**Next Step**: %s", nextStep))

	return models.DiscoveryResponse{
		Narrative: narrative,
		Reasoning: fmt.Sprintf(
			"The engine performed a depth-limited filesystem scan (max depth: %d) "+
				"and executed targeted Go AST analysis. We identified %d gaps across "+
				"Context, Tech Stack, Testing, and Code Quality domains.",
			maxWalkDepth, len(gaps),
		),
		SummaryMD: sb.String(),
		Gaps:      gaps,
		NextStep:  nextStep,
	}, nil
}

// SuggestNextStep returns the most critical next action
// based on the current session state.
func (e *Engine) SuggestNextStep(
	ctx context.Context,
	session *models.Session,
	_ string,
) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	if len(session.Gaps) > 0 {
		g := session.Gaps[0]
		return fmt.Sprintf(
			"Critical Gap: %s. %s",
			g.Area, g.Description,
		), nil
	}

	switch session.Status {
	case "DISCOVERY":
		return "Discovery complete. Ask for the" +
			" project's primary Purpose.", nil
	case "CLARIFICATION":
		return "Purpose defined. Ask for Constraints" +
			" or Success Criteria.", nil
	default:
		return "Iterative design phase: use" +
			" 'critique_design' to stress-test" +
			" components.", nil
	}
}
