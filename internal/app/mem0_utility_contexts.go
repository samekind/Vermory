package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"vermory/internal/memorybackend"
	"vermory/internal/reality"
	"vermory/internal/utilityeval"
)

type Mem0UtilityContextOptions struct {
	ProfilePath   string
	CaseRoot      string
	BaseURL       string
	APIKey        string
	ContextDir    string
	RunID         string
	IncludeSource bool
}

func PrepareMem0UtilityContexts(ctx context.Context, opts Mem0UtilityContextOptions) error {
	profile, err := utilityeval.LoadProfile(opts.ProfilePath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(opts.BaseURL) == "" || strings.TrimSpace(opts.ContextDir) == "" {
		return fmt.Errorf("mem0 utility preparation requires base URL and context directory")
	}
	if err := os.MkdirAll(opts.ContextDir, 0o700); err != nil {
		return err
	}
	backend, cleanup, err := memorybackend.OpenBackend(ctx, memorybackend.OpenConfig{Name: "mem0", BaseURL: opts.BaseURL, APIKey: opts.APIKey})
	if err != nil {
		return err
	}
	defer cleanup()
	if err := backend.Health(ctx); err != nil {
		return fmt.Errorf("mem0 health: %w", err)
	}
	runID := chooseRunID(opts.RunID, "mem0-contexts")
	for _, caseID := range profile.RealityCaseIDs {
		frozenCase, err := reality.LoadCase(filepath.Join(opts.CaseRoot, caseID))
		if err != nil {
			return err
		}
		scope := memorybackend.Scope{
			TenantID:       "w27-mem0-" + compactIdentifier(runID),
			ContinuityID:   compactIdentifier(caseID),
			ContinuityLine: string(frozenCase.Manifest.ContinuityLines[0]),
		}
		if err := backend.ResetScope(ctx, scope); err != nil {
			return fmt.Errorf("reset mem0 scope %s: %w", caseID, err)
		}
		for index, event := range frozenCase.Events {
			if err := backend.Put(ctx, memorybackend.Record{
				ID:       event.ID,
				Scope:    scope,
				SourceID: event.SourceID,
				Status:   "active",
				Content:  event.Content,
				Metadata: map[string]string{
					"run_id":   runID,
					"sequence": fmt.Sprintf("%d", index+1),
				},
			}); err != nil {
				return fmt.Errorf("put mem0 event %s/%s: %w", caseID, event.ID, err)
			}
		}
		if opts.IncludeSource {
			for _, source := range frozenCase.Manifest.Sources {
				data, readErr := os.ReadFile(filepath.Join(frozenCase.Directory, source.FixturePath))
				if readErr != nil {
					return readErr
				}
				if err := backend.Put(ctx, memorybackend.Record{
					ID:       source.ID,
					Scope:    scope,
					SourceID: source.ID,
					Status:   "active",
					Content:  string(data),
					Metadata: map[string]string{
						"run_id": runID,
						"source": "fixture",
					},
				}); err != nil {
					return fmt.Errorf("put mem0 source %s/%s: %w", caseID, source.ID, err)
				}
			}
		}
		results, err := backend.Search(ctx, memorybackend.Query{Scope: scope, Text: frozenCase.Manifest.Task.Prompt, Limit: 8})
		if err != nil {
			return fmt.Errorf("search mem0 case %s: %w", caseID, err)
		}
		lines := make([]string, 0, len(results))
		for _, result := range results {
			if content := strings.TrimSpace(result.Record.Content); content != "" {
				lines = append(lines, content)
			}
		}
		if len(lines) == 0 {
			return fmt.Errorf("mem0 returned empty context for %s", caseID)
		}
		if err := os.WriteFile(filepath.Join(opts.ContextDir, caseID+".md"), []byte(strings.Join(lines, "\n")), 0o600); err != nil {
			return err
		}
	}
	return nil
}
