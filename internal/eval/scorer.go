package eval

import "strings"

// ScoreOutput preserves the existing string-check scoring and derives
// acceptance-shaped proxy metrics from the same deterministic checks. These
// proxy metrics are intentionally conservative placeholders for reporting, not
// substitutes for full continuation/groundedness/fitness evaluation.
func ScoreOutput(task Task, output string) Score {
	lowerOutput := strings.ToLower(output)

	var missing []string
	for _, item := range task.MustInclude {
		if !strings.Contains(lowerOutput, strings.ToLower(item)) {
			missing = append(missing, item)
		}
	}
	for _, alternatives := range task.MustIncludeAny {
		matched := false
		for _, alternative := range alternatives {
			if strings.Contains(lowerOutput, strings.ToLower(alternative)) {
				matched = true
				break
			}
		}
		if !matched {
			missing = append(missing, "any of: "+strings.Join(alternatives, " | "))
		}
	}

	var violations []string
	for _, item := range task.MustNotInclude {
		if strings.Contains(lowerOutput, strings.ToLower(item)) {
			violations = append(violations, item)
		}
	}

	hitRate := 1.0
	requiredFacts := len(task.MustInclude) + len(task.MustIncludeAny)
	if requiredFacts > 0 {
		hitRate = float64(requiredFacts-len(missing)) / float64(requiredFacts)
	}

	groundedness := 1.0
	if len(task.MustNotInclude) > 0 {
		groundedness = float64(len(task.MustNotInclude)-len(violations)) / float64(len(task.MustNotInclude))
	}

	targetFitness := hitRate
	if groundedness < targetFitness {
		targetFitness = groundedness
	}

	return Score{
		MustIncludeHitRate:       hitRate,
		MissingIncludes:          missing,
		MustNotIncludeViolations: violations,
		Continuation:             hitRate,
		Groundedness:             groundedness,
		TargetFitness:            targetFitness,
	}
}
