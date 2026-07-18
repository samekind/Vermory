package utilityeval

import (
	"fmt"
	"strings"
	"unicode"

	"vermory/internal/reality"
)

const scorerVersion = "utilityeval-checks-v1"

func scoreTask(task reality.DownstreamTask, output string) Score {
	normalized := normalize(output)
	var required []CheckResult
	var forbidden []CheckResult

	for _, declaration := range task.DeterministicChecks {
		declaration = strings.TrimSpace(declaration)
		switch {
		case strings.HasPrefix(declaration, "contains:"):
			value := strings.TrimSpace(strings.TrimPrefix(declaration, "contains:"))
			passed := value != "" && strings.Contains(normalized, normalize(value))
			required = append(required, CheckResult{Check: declaration, Passed: passed, Reason: matchReason(passed, value)})
		case strings.HasPrefix(declaration, "not_contains:"):
			value := strings.TrimSpace(strings.TrimPrefix(declaration, "not_contains:"))
			passed := value == "" || !strings.Contains(normalized, normalize(value))
			forbidden = append(forbidden, CheckResult{Check: declaration, Passed: passed, Reason: matchReason(!passed, value)})
		case declaration == "language:zh":
			passed := containsHan(output)
			required = append(required, CheckResult{Check: declaration, Passed: passed, Reason: matchReason(passed, "Chinese text")})
		default:
			required = append(required, CheckResult{Check: declaration, Passed: false, Reason: "unsupported deterministic check"})
		}
	}

	forbiddenHits := 0
	for _, check := range forbidden {
		if !check.Passed {
			forbiddenHits++
		}
	}
	success := forbiddenHits == 0
	for _, check := range required {
		if !check.Passed {
			success = false
		}
	}
	return Score{Success: success, RequiredChecks: required, ForbiddenChecks: forbidden, ForbiddenHits: forbiddenHits}
}

func normalize(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(value)), " ")
}

func containsHan(value string) bool {
	for _, r := range value {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func matchReason(passed bool, value string) string {
	if passed {
		return "matched"
	}
	return fmt.Sprintf("did not satisfy %q", value)
}
