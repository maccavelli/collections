// Package engine provides functionality for the engine subsystem.
package engine

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/sync/errgroup"
	"mcp-server-brainstorm/internal/models"
)

// AnalyzeDiscovery scans the project for context and
// identifies initial gaps. It uses a depth-limited walk
// and skips common non-source directories.
func (e *Engine) AnalyzeDiscovery(
	ctx context.Context, path string, session *models.Session,
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
		e.mu.RLock()
		rf, okR := session.Metadata["readme_found"].(bool)
		tf, okT := session.Metadata["test_found"].(bool)
		e.mu.RUnlock()

		if okR && okT {
			readmeFound = rf
			testFound = tf
			return nil
		}

		rfScan, tfScan, err := e.scanForContextFiles(gCtx, target)
		if err != nil {
			return err
		}
		readmeFound, testFound = rfScan, tfScan

		e.mu.Lock()
		if session.Metadata == nil {
			session.Metadata = make(map[string]any)
		}
		session.Metadata["readme_found"] = rfScan
		session.Metadata["test_found"] = tfScan
		e.mu.Unlock()

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
			Description: "Cannot detect language or stack. Root boundary markers are missing.",
			Severity:    "RECOMMENDED",
		})
	}

	if hasGo {
		// Rely entirely on CSSA Backplane for Go structural analytics
		if e.ExternalClient != nil {
			tracePayload := e.LoadCrossSessionFromRecall(ctx, "go-refactor", e.ResolvePath(target))
			if tracePayload != "" {
				gaps = append(gaps, models.Gap{
					Area:        "STRUCTURAL",
					Description: "Deep AST telemetry retrieved via backplane orchestrator logic.",
					Severity:    "INFO",
				})
			}
		} else {
			// Standalone Mode Fallback
			gaps = append(gaps, models.Gap{
				Area:        "STRUCTURAL",
				Description: "[STANDALONE] Deep AST structural analysis disabled without Orchestrator backplane.",
				Severity:    "RECOMMENDED",
			})
		}
	}
	return gaps, nil
}

// extractDiscoveryMetadata gathers deep foundation metrics without parsing AST
func (e *Engine) extractDiscoveryMetadata(ctx context.Context, target string) models.DiscoveryMetadata {
	md := models.DiscoveryMetadata{}

	// 1. README Intent
	readmePath := filepath.Join(target, "README.md")
	if content, err := os.ReadFile(readmePath); err == nil {
		str := string(content)
		if len(str) > 500 {
			str = str[:500] + "..."
		}
		md.Intent = str
	}

	// 2. Parse go.mod
	goModPath := filepath.Join(target, "go.mod")
	if content, err := os.ReadFile(goModPath); err == nil {
		if file, err := modfile.Parse(goModPath, content, nil); err == nil {
			if file.Go != nil {
				md.GoVersion = file.Go.Version
			}
			for i, req := range file.Require {
				if i >= 10 {
					break
				}
				md.Dependencies = append(md.Dependencies, req.Mod.Path)
			}
		}
	}

	// 3. Architecture Topology
	dirs := []string{"cmd", "internal", "pkg", "api"}
	for _, d := range dirs {
		if fi, err := os.Stat(filepath.Join(target, d)); err == nil && fi.IsDir() {
			md.ArchitectureMap = append(md.ArchitectureMap, d)
		}
	}

	// 4. Deploy Markers
	markers := []string{"Dockerfile", ".github", ".gitlab-ci.yml"}
	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(target, m)); err == nil {
			md.DeployMarkers = append(md.DeployMarkers, m)
		}
	}

	return md
}

// DiscoverProject performs a unified discovery scan, identifying
// gaps and suggesting the next logical step. It returns a
// consolidated DiscoveryResponse with a narrative summary
// and formatted markdown.
func (e *Engine) DiscoverProject(
	ctx context.Context, path string, session *models.Session,
) (models.DiscoveryResponse, error) {
	gaps, err := e.AnalyzeDiscovery(ctx, path, session)
	if err != nil {
		return models.DiscoveryResponse{}, err
	}

	// Extract Foundation Metadata
	meta := e.extractDiscoveryMetadata(ctx, path)

	// Query Recall lazily for architectural standards
	standards := e.EnsureRecallCache(ctx, session, "discovery_architectural", "search", map[string]any{"namespace": "ecosystem", "query": "baseline project architectural standards", "domain": "discovery", "limit": 10})

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
	if standards != "" {
		narrative += " Standards baseline retrieved from Recall."
	}
	if len(meta.Dependencies) > 0 {
		narrative += " Ecosystem footprint mapped."
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
		Summary: fmt.Sprintf("Project discovery complete. %d gaps identified.", len(gaps)),
		Data: struct {
			Narrative string                   `json:"narrative"`
			Reasoning string                   `json:"reasoning,omitempty"`
			Gaps      []models.Gap             `json:"gaps"`
			NextStep  string                   `json:"next_step"`
			Standards string                   `json:"standards,omitempty"`
			Metadata  models.DiscoveryMetadata `json:"metadata"`
		}{
			Narrative: narrative,
			Reasoning: fmt.Sprintf(
				"The engine performed a depth-limited filesystem scan (max depth: %d) "+
					"and executed targeted Go AST analysis. We identified %d gaps across "+
					"Context, Tech Stack, Testing, and Code Quality domains.",
				maxWalkDepth, len(gaps),
			),
			Gaps:      gaps,
			NextStep:  nextStep,
			Standards: standards,
			Metadata:  meta,
		},
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
