package utilityeval

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"vermory/internal/reality"
)

// BuildComparableContexts creates only the context conditions that do not need
// a live backend. Native and mem0 bodies are injected after their independent
// runtime calls and are still hashed by NewCaseInput before model execution.
func BuildComparableContexts(c reality.Case, nativeContext, mem0Context string) (map[ConditionID]string, error) {
	if strings.TrimSpace(c.Manifest.Task.Prompt) == "" {
		return nil, fmt.Errorf("utilityeval: case %s has no task", c.Manifest.ID)
	}
	return map[ConditionID]string{
		ConditionNoContext:      "",
		ConditionFullHistory:    fullHistory(c.Events),
		ConditionPlainSummary:   plainSummary(c.Events),
		ConditionPlainRetrieval: plainRetrieval(c.Events, c.Manifest.Task.Prompt),
		ConditionMem0OSS:        strings.TrimSpace(mem0Context),
		ConditionVermoryNative:  strings.TrimSpace(nativeContext),
	}, nil
}

func fullHistory(events []reality.Event) string {
	lines := make([]string, 0, len(events))
	for _, event := range events {
		lines = append(lines, fmt.Sprintf("[%s/%s] %s: %s", event.Channel, event.Actor, event.ID, strings.TrimSpace(event.Content)))
	}
	return strings.Join(lines, "\n")
}

func plainSummary(events []reality.Event) string {
	lines := make([]string, 0, len(events))
	for _, event := range events {
		content := strings.Join(strings.Fields(strings.TrimSpace(event.Content)), " ")
		if content != "" {
			lines = append(lines, "- "+content)
		}
	}
	return strings.Join(lines, "\n")
}

func plainRetrieval(events []reality.Event, query string) string {
	type candidate struct {
		index int
		score int
		text  string
	}
	queryTokens := tokens(query)
	candidates := make([]candidate, 0, len(events))
	for index, event := range events {
		text := strings.TrimSpace(event.Content)
		if text == "" {
			continue
		}
		score := 0
		for token := range tokens(text) {
			if queryTokens[token] {
				score++
			}
		}
		candidates = append(candidates, candidate{index: index, score: score, text: text})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].index < candidates[j].index
		}
		return candidates[i].score > candidates[j].score
	})
	if len(candidates) > 4 {
		candidates = candidates[:4]
	}
	lines := make([]string, 0, len(candidates))
	for _, item := range candidates {
		lines = append(lines, item.text)
	}
	return strings.Join(lines, "\n")
}

func tokens(value string) map[string]bool {
	result := make(map[string]bool)
	var current []rune
	flush := func() {
		if len(current) > 0 {
			result[strings.ToLower(string(current))] = true
			current = nil
		}
	}
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Han, r) {
			current = append(current, r)
			continue
		}
		flush()
	}
	flush()
	return result
}
