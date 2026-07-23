package reality

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type Experiment0Options struct {
	RunID             string
	CaseRoot          string
	SealedAttestation *Attestation
}

type Experiment0Report struct {
	RunID              string              `json:"run_id"`
	Pass               bool                `json:"pass"`
	PublicValidation   ValidationReport    `json:"public_validation"`
	ContinuityCoverage map[string][]string `json:"continuity_coverage"`
	PressureCoverage   map[string][]string `json:"pressure_coverage"`
	EvidenceLevels     map[string]int      `json:"evidence_levels"`
	SealedStatus       string              `json:"sealed_status"`
	Baselines          map[string]string   `json:"baselines"`
	HypothesisSignals  map[string][]string `json:"hypothesis_signals"`
	Limitations        []string            `json:"limitations"`
}

type Experiment0Artifacts struct {
	JSONPath     string `json:"json_path"`
	MarkdownPath string `json:"markdown_path"`
}

var constitutionalHardGateDefinitions = []string{
	"zero_cross_tenant_leakage",
	"zero_cross_continuity_leakage",
	"zero_wrong_strong_anchor_merge",
	"ambiguous_anchor_abstention",
	"zero_deleted_target_leakage",
	"zero_stale_as_current",
	"explicit_intent_and_live_source_authority",
	"projection_rebuild_equivalence",
	"duplicate_event_idempotency",
	"provider_failure_preserves_governance",
}

func BuildExperiment0(options Experiment0Options) Experiment0Report {
	validation := ValidateRoot(options.CaseRoot)
	validation.RunID = options.RunID + "-public-validation"
	report := Experiment0Report{
		RunID:              options.RunID,
		PublicValidation:   validation,
		ContinuityCoverage: make(map[string][]string),
		PressureCoverage:   make(map[string][]string),
		EvidenceLevels: map[string]int{
			string(EvidencePublic):        0,
			string(EvidenceWithheldLocal): 0,
			"sealed":                      0,
		},
		SealedStatus: "unavailable",
		Baselines: map[string]string{
			"no_context":                     "legacy_harness_available",
			"stale_context":                  "legacy_harness_available",
			"plain_summary":                  "legacy_harness_available",
			"legacy_contextmesh_packet":      "legacy_harness_available",
			"full_history":                   "planned_comparison",
			"ordinary_vector_rag":            "planned_comparison",
			"mem0":                           "adapter_evidence_only",
			"native_postgresql_pgvector":     "supporting_backend_evidence",
			"legacy_contextmesh_self_case":   "inspired_case",
			"memos_three_client_round_packs": "translated_proxy",
		},
		HypothesisSignals: map[string][]string{
			"constitutional_hard_gate_definitions": append([]string(nil), constitutionalHardGateDefinitions...),
		},
	}

	caseIDs := make(map[string]struct{}, len(validation.Results))
	for _, result := range validation.Results {
		caseIDs[result.CaseID] = struct{}{}
		if result.EvidenceLevel != "" {
			report.EvidenceLevels[string(result.EvidenceLevel)]++
		}
		for _, line := range result.Lines {
			report.ContinuityCoverage[string(line)] = append(report.ContinuityCoverage[string(line)], result.CaseID)
		}
		for _, pressure := range result.Pressures {
			report.PressureCoverage[pressure] = append(report.PressureCoverage[pressure], result.CaseID)
		}
	}
	sortCoverage(report.ContinuityCoverage)
	sortCoverage(report.PressureCoverage)
	report.HypothesisSignals["H-003"] = existingCases(caseIDs, "W01-synapseloom-continuity", "S01-deletion-and-source-injection")
	report.HypothesisSignals["H-005"] = existingCases(caseIDs, "W01-synapseloom-continuity", "C01-device-maintenance-continuity", "F03-verified-tool-outcome-formation", "S01-deletion-and-source-injection")
	report.HypothesisSignals["H-007"] = existingCases(
		caseIDs,
		"G01-language-default-local-override",
		"S01-deletion-and-source-injection",
		"C02-housing-viewing-validity",
		"W03-workspace-workaround-validity",
	)
	report.HypothesisSignals["H-008"] = existingCases(caseIDs, "C01-device-maintenance-continuity", "F03-verified-tool-outcome-formation", "S01-deletion-and-source-injection")
	report.HypothesisSignals["H-013"] = existingCases(caseIDs, "I01-authenticated-multitenant-rls")
	report.HypothesisSignals["H-014"] = existingCases(caseIDs, "I02-postgresql-operations-recovery", "I03-postgresql-ha-pitr")
	report.HypothesisSignals["bridge_seed"] = existingCases(
		caseIDs,
		"B01-conversation-workspace-promotion",
		"B02-linked-conversations-workspace-rebind",
		"B03-three-client-conversation-bridge",
	)
	report.HypothesisSignals["hermes_client_seed"] = existingCases(caseIDs, "H01-hermes-linked-sessions")

	if options.SealedAttestation != nil {
		report.SealedStatus = "attested"
		report.EvidenceLevels["sealed"] = options.SealedAttestation.Counts["cases"]
	}

	report.Limitations = []string{
		"The public cases are an expanding seed batch; target discovery coverage remains incomplete and is not an automatic pass or fail threshold.",
		"Experiment 0 freezes evidence only; it does not execute a production memory implementation or a real client integration.",
		"The new reality cases have not yet run against no-context, full-history, summary, vector-RAG, mem0, and Vermory conditions.",
		"Legacy ContextMesh scenarios and the MemOS three-client round packs remain inspired cases or translated proxies rather than real executed trajectories.",
	}
	if report.EvidenceLevels[string(EvidenceWithheldLocal)] == 0 {
		report.Limitations = append(report.Limitations, "No withheld_local holdout is included in this readout.")
	}
	if options.SealedAttestation == nil {
		report.Limitations = append(report.Limitations, "A genuine external sealed evaluator is unavailable; no sealed result is claimed.")
	}
	if !validation.Pass {
		report.Limitations = append(report.Limitations, "One or more frozen case validations failed; all violations are preserved in public_validation.")
	}
	sort.Strings(report.Limitations)

	report.Pass = validation.Pass && len(constitutionalHardGateDefinitions) != 0
	if options.SealedAttestation != nil && !options.SealedAttestation.HardGatesPass {
		report.Pass = false
	}
	return report
}

func WriteExperiment0Artifacts(artifactRoot string, report Experiment0Report) (Experiment0Artifacts, error) {
	if report.RunID == "" || filepath.Base(report.RunID) != report.RunID || report.RunID == "." || report.RunID == ".." {
		return Experiment0Artifacts{}, fmt.Errorf("invalid run id %q", report.RunID)
	}
	dir := filepath.Join(artifactRoot, "experiment-0", report.RunID)
	jsonPath := filepath.Join(dir, "report.json")
	markdownPath := filepath.Join(dir, "report.md")
	data, err := marshalIndented(report)
	if err != nil {
		return Experiment0Artifacts{}, err
	}
	if err := writeAtomic(jsonPath, data); err != nil {
		return Experiment0Artifacts{}, err
	}
	if err := writeAtomic(markdownPath, []byte(renderExperiment0Markdown(report))); err != nil {
		return Experiment0Artifacts{}, err
	}
	return Experiment0Artifacts{JSONPath: jsonPath, MarkdownPath: markdownPath}, nil
}

func renderExperiment0Markdown(report Experiment0Report) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# Vermory Experiment 0: %s\n\n", report.RunID)
	fmt.Fprintf(&builder, "- Pass: `%t`\n", report.Pass)
	fmt.Fprintf(&builder, "- Frozen cases: `%d`\n", report.PublicValidation.Cases)
	fmt.Fprintf(&builder, "- Sealed status: `%s`\n\n", report.SealedStatus)

	builder.WriteString("## Cases\n\n")
	for _, result := range report.PublicValidation.Results {
		fmt.Fprintf(&builder, "- `%s`: pass=`%t`, lock=`%s`\n", result.CaseID, result.Pass, result.LockSHA256)
		for _, violation := range result.Violations {
			fmt.Fprintf(&builder, "  - violation `%s`: %s\n", violation.Code, violation.Message)
		}
	}

	builder.WriteString("\n## Continuity Coverage\n\n")
	for _, key := range sortedStringSliceMapKeys(report.ContinuityCoverage) {
		fmt.Fprintf(&builder, "- `%s`: %s\n", key, strings.Join(report.ContinuityCoverage[key], ", "))
	}
	builder.WriteString("\n## Pressure Coverage\n\n")
	for _, key := range sortedStringSliceMapKeys(report.PressureCoverage) {
		fmt.Fprintf(&builder, "- `%s`: %s\n", key, strings.Join(report.PressureCoverage[key], ", "))
	}

	builder.WriteString("\n## Evidence Levels\n\n")
	for _, key := range sortedMapKeys(report.EvidenceLevels) {
		fmt.Fprintf(&builder, "- `%s`: %d\n", key, report.EvidenceLevels[key])
	}
	builder.WriteString("\n## Baselines\n\n")
	for _, key := range sortedStringMapKeys(report.Baselines) {
		fmt.Fprintf(&builder, "- `%s`: `%s`\n", key, report.Baselines[key])
	}
	builder.WriteString("\n## Hypothesis Signals\n\n")
	for _, key := range sortedStringSliceMapKeys(report.HypothesisSignals) {
		fmt.Fprintf(&builder, "- `%s`: %s\n", key, strings.Join(report.HypothesisSignals[key], ", "))
	}
	builder.WriteString("\n## Limitations\n\n")
	for _, limitation := range report.Limitations {
		fmt.Fprintf(&builder, "- %s\n", limitation)
	}
	return builder.String()
}

func existingCases(available map[string]struct{}, ids ...string) []string {
	var result []string
	for _, id := range ids {
		if _, exists := available[id]; exists {
			result = append(result, id)
		}
	}
	sort.Strings(result)
	return result
}

func sortCoverage(coverage map[string][]string) {
	for key := range coverage {
		sort.Strings(coverage[key])
	}
}

func sortedStringSliceMapKeys(values map[string][]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
