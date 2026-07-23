package eval

type Task struct {
	ID             string     `json:"id"`
	Prompt         string     `json:"prompt"`
	MustInclude    []string   `json:"must_include"`
	MustIncludeAny [][]string `json:"must_include_any,omitempty"`
	MustNotInclude []string   `json:"must_not_include"`
}

// Score keeps the legacy string-check outputs and adds heuristic proxy metrics
// needed by the internal-ready reporting shape. The derived fields are
// deterministic signals from MustInclude/MustNotInclude matching only; they are
// not full semantic evaluation scores.
type Score struct {
	MustIncludeHitRate       float64  `json:"must_include_hit_rate"`
	MissingIncludes          []string `json:"missing_includes"`
	MustNotIncludeViolations []string `json:"must_not_include_violations"`
	Continuation             float64  `json:"continuation"`
	Groundedness             float64  `json:"groundedness"`
	TargetFitness            float64  `json:"target_fitness"`
}

// AcceptanceScore carries the unified acceptance shape from the plan. In this
// Task 3 slice, not every dimension is produced or hard-gated yet.
type AcceptanceScore struct {
	Continuation  float64 `json:"continuation"`
	Isolation     float64 `json:"isolation"`
	Groundedness  float64 `json:"groundedness"`
	Governance    float64 `json:"governance"`
	TargetFitness float64 `json:"target_fitness"`
	CostFriction  float64 `json:"cost_friction"`
}

// AcceptanceResult reports which hard gates failed for the minimal acceptance
// pass/fail decision.
type AcceptanceResult struct {
	Pass   bool     `json:"pass"`
	Failed []string `json:"failed"`
}
