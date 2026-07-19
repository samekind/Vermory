package utilityeval

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"vermory/internal/reality"
)

const ScorerVersion = "utilityeval-checks-v3"

func scoreTask(task reality.DownstreamTask, aliases map[string][]string, output string) Score {
	normalized := normalize(output)
	var required []CheckResult
	var forbidden []CheckResult

	for _, declaration := range task.DeterministicChecks {
		declaration = strings.TrimSpace(declaration)
		switch {
		case strings.HasPrefix(declaration, "contains:"):
			value := strings.TrimSpace(strings.TrimPrefix(declaration, "contains:"))
			passed, matched := containsDeclaredEquivalent(normalized, value, aliases[value])
			required = append(required, CheckResult{Check: declaration, Passed: passed, Reason: equivalentMatchReason(passed, value, matched, aliases[value])})
		case strings.HasPrefix(declaration, "not_contains:"):
			value := strings.TrimSpace(strings.TrimPrefix(declaration, "not_contains:"))
			matchedForbidden, matched := containsDeclaredEquivalent(normalized, value, aliases[value])
			passed := value == "" || !matchedForbidden
			forbidden = append(forbidden, CheckResult{Check: declaration, Passed: passed, Reason: equivalentMatchReason(!passed, value, matched, aliases[value])})
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
	return Score{Success: success, NormalizedOutput: normalized, RequiredChecks: required, ForbiddenChecks: forbidden, ForbiddenHits: forbiddenHits}
}

func normalize(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(norm.NFKC.String(value))), " ")
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

func containsDeclaredEquivalent(normalizedOutput, canonical string, aliases []string) (bool, string) {
	for _, candidate := range append([]string{canonical}, aliases...) {
		candidate = strings.TrimSpace(candidate)
		if strings.HasPrefix(candidate, "regex:") {
			pattern := strings.TrimPrefix(candidate, "regex:")
			if compiled, err := regexp.Compile(pattern); err == nil && compiled.MatchString(normalizedOutput) {
				return true, candidate
			}
			continue
		}
		if candidate != "" && strings.Contains(normalizedOutput, normalize(candidate)) {
			return true, candidate
		}
	}
	return false, ""
}

func equivalentMatchReason(passed bool, canonical, matched string, aliases []string) string {
	if passed {
		if matched == canonical {
			return "matched canonical phrase"
		}
		return fmt.Sprintf("matched declared equivalent %q", matched)
	}
	if len(aliases) == 0 {
		return fmt.Sprintf("did not satisfy %q", canonical)
	}
	return fmt.Sprintf("did not satisfy %q or its declared equivalents", canonical)
}

func validateScoringAliases(aliases map[string][]string, declarations []string) error {
	declaredValues := make(map[string]struct{}, len(declarations))
	for _, declaration := range declarations {
		declaration = strings.TrimSpace(declaration)
		for _, prefix := range []string{"contains:", "not_contains:"} {
			if strings.HasPrefix(declaration, prefix) {
				declaredValues[strings.TrimSpace(strings.TrimPrefix(declaration, prefix))] = struct{}{}
			}
		}
	}
	keys := make([]string, 0, len(aliases))
	for canonical := range aliases {
		keys = append(keys, canonical)
	}
	sort.Strings(keys)
	for _, canonical := range keys {
		if strings.TrimSpace(canonical) == "" {
			return fmt.Errorf("canonical phrase is empty")
		}
		if declarations != nil {
			if _, ok := declaredValues[canonical]; !ok {
				return fmt.Errorf("canonical phrase %q has no deterministic check", canonical)
			}
		}
		values := aliases[canonical]
		if len(values) == 0 {
			return fmt.Errorf("canonical phrase %q has no equivalents", canonical)
		}
		seen := map[string]struct{}{normalize(canonical): {}}
		for _, value := range values {
			value = strings.TrimSpace(value)
			if strings.HasPrefix(value, "regex:") {
				pattern := strings.TrimPrefix(value, "regex:")
				if pattern == "" {
					return fmt.Errorf("canonical phrase %q has an empty regex equivalent", canonical)
				}
				if _, err := regexp.Compile(pattern); err != nil {
					return fmt.Errorf("canonical phrase %q has invalid regex equivalent: %w", canonical, err)
				}
			}
			normalized := normalize(value)
			if normalized == "" {
				return fmt.Errorf("canonical phrase %q has an empty equivalent", canonical)
			}
			if _, exists := seen[normalized]; exists {
				return fmt.Errorf("canonical phrase %q repeats equivalent %q", canonical, value)
			}
			seen[normalized] = struct{}{}
		}
	}
	return nil
}
