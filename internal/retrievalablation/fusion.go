package retrievalablation

import (
	"sort"
	"strings"
)

func FuseRRF(query string, limit int, lexical, vector []RankedResult) []RankedResult {
	if limit <= 0 {
		return nil
	}
	query = strings.ToLower(strings.TrimSpace(query))
	byMemory := make(map[string]*RankedResult, len(lexical)+len(vector))
	for index, input := range lexical {
		key := resultKey(input)
		if key == "" {
			continue
		}
		result, exists := byMemory[key]
		if !exists {
			copy := input
			copy.Score = 0
			copy.LexicalRank = 0
			copy.VectorRank = 0
			copy.Exact = false
			result = &copy
			byMemory[key] = result
		}
		result.Eligible = result.Eligible || input.Eligible
		if result.LexicalRank == 0 {
			result.LexicalRank = index + 1
			result.Score += reciprocalRank(index + 1)
		}
	}
	for index, input := range vector {
		key := resultKey(input)
		if key == "" {
			continue
		}
		result, exists := byMemory[key]
		if !exists {
			copy := input
			copy.Score = 0
			copy.LexicalRank = 0
			copy.VectorRank = 0
			copy.Exact = false
			result = &copy
			byMemory[key] = result
		}
		result.Eligible = result.Eligible || input.Eligible
		if result.RecordID == "" {
			result.RecordID = input.RecordID
		}
		if result.Content == "" {
			result.Content = input.Content
		}
		if result.VectorRank == 0 {
			result.VectorRank = index + 1
			result.Score += reciprocalRank(index + 1)
		}
	}
	results := make([]RankedResult, 0, len(byMemory))
	for _, result := range byMemory {
		result.Exact = query != "" && strings.Contains(strings.ToLower(result.Content), query)
		results = append(results, *result)
	}
	sort.Slice(results, func(i, j int) bool {
		left, right := results[i], results[j]
		if left.Exact != right.Exact {
			return left.Exact
		}
		if left.Score != right.Score {
			return left.Score > right.Score
		}
		if rankLess(left.LexicalRank, right.LexicalRank) {
			return true
		}
		if rankLess(right.LexicalRank, left.LexicalRank) {
			return false
		}
		if rankLess(left.VectorRank, right.VectorRank) {
			return true
		}
		if rankLess(right.VectorRank, left.VectorRank) {
			return false
		}
		return resultKey(left) < resultKey(right)
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

func FallbackToLexical(lexical []RankedResult, limit int) []RankedResult {
	if limit <= 0 || len(lexical) == 0 {
		return nil
	}
	if limit > len(lexical) {
		limit = len(lexical)
	}
	results := append([]RankedResult(nil), lexical[:limit]...)
	for index := range results {
		results[index].LexicalRank = index + 1
		results[index].VectorRank = 0
	}
	return results
}

func reciprocalRank(rank int) float64 {
	if rank <= 0 {
		return 0
	}
	return 1 / float64(RRFK+rank)
}

func rankLess(left, right int) bool {
	if left == 0 {
		return false
	}
	if right == 0 {
		return true
	}
	return left < right
}

func resultKey(result RankedResult) string {
	if result.MemoryID != "" {
		return result.MemoryID
	}
	return result.RecordID
}
