package benchmark

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"unicode"
)

type LongMemEvalTurn struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	HasAnswer bool   `json:"has_answer,omitempty"`
}

type LongMemEvalRecord struct {
	QuestionID         string              `json:"question_id"`
	QuestionType       string              `json:"question_type"`
	Question           string              `json:"question"`
	Answer             string              `json:"answer"`
	QuestionDate       string              `json:"question_date"`
	HaystackDates      []string            `json:"haystack_dates"`
	HaystackSessionIDs []string            `json:"haystack_session_ids"`
	HaystackSessions   [][]LongMemEvalTurn `json:"haystack_sessions"`
	AnswerSessionIDs   []string            `json:"answer_session_ids"`
}

func (record *LongMemEvalRecord) UnmarshalJSON(data []byte) error {
	var raw struct {
		QuestionID         string              `json:"question_id"`
		QuestionType       string              `json:"question_type"`
		Question           string              `json:"question"`
		Answer             json.RawMessage     `json:"answer"`
		QuestionDate       string              `json:"question_date"`
		HaystackDates      []string            `json:"haystack_dates"`
		HaystackSessionIDs []string            `json:"haystack_session_ids"`
		HaystackSessions   [][]LongMemEvalTurn `json:"haystack_sessions"`
		AnswerSessionIDs   []string            `json:"answer_session_ids"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	answer, err := decodeScalarString(raw.Answer)
	if err != nil {
		return fmt.Errorf("answer: %w", err)
	}
	*record = LongMemEvalRecord{
		QuestionID:         raw.QuestionID,
		QuestionType:       raw.QuestionType,
		Question:           raw.Question,
		Answer:             answer,
		QuestionDate:       raw.QuestionDate,
		HaystackDates:      raw.HaystackDates,
		HaystackSessionIDs: raw.HaystackSessionIDs,
		HaystackSessions:   raw.HaystackSessions,
		AnswerSessionIDs:   raw.AnswerSessionIDs,
	}
	return nil
}

type LongMemEvalSession struct {
	ID       string
	Date     string
	Turns    []LongMemEvalTurn
	Position int
	score    int
}

type DeterministicScore struct {
	ExactMatch          bool    `json:"exact_match"`
	TokenF1             float64 `json:"token_f1"`
	AnswerTokenRecall   float64 `json:"answer_token_recall"`
	AbstentionExpected  bool    `json:"abstention_expected"`
	AbstentionDetected  bool    `json:"abstention_detected"`
	ReferenceVariant    string  `json:"reference_variant"`
	NormalizedReference string  `json:"normalized_reference"`
	NormalizedResponse  string  `json:"normalized_response"`
}

func LoadLongMemEval(path string) ([]LongMemEvalRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var records []LongMemEvalRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("LongMemEval dataset contains no records")
	}
	seen := make(map[string]struct{}, len(records))
	for i, record := range records {
		if err := record.validate(); err != nil {
			return nil, fmt.Errorf("LongMemEval record %d: %w", i, err)
		}
		if _, exists := seen[record.QuestionID]; exists {
			return nil, fmt.Errorf("LongMemEval dataset contains duplicate question_id %q", record.QuestionID)
		}
		seen[record.QuestionID] = struct{}{}
	}
	return records, nil
}

func (record LongMemEvalRecord) validate() error {
	if strings.TrimSpace(record.QuestionID) == "" || strings.TrimSpace(record.QuestionType) == "" {
		return fmt.Errorf("question_id and question_type are required")
	}
	if strings.TrimSpace(record.Question) == "" || strings.TrimSpace(record.Answer) == "" {
		return fmt.Errorf("question and answer are required")
	}
	if len(record.HaystackDates) != len(record.HaystackSessionIDs) || len(record.HaystackDates) != len(record.HaystackSessions) {
		return fmt.Errorf("parallel session arrays have lengths dates=%d ids=%d sessions=%d", len(record.HaystackDates), len(record.HaystackSessionIDs), len(record.HaystackSessions))
	}
	if len(record.HaystackSessions) == 0 {
		return fmt.Errorf("at least one haystack session is required")
	}
	for i, id := range record.HaystackSessionIDs {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("haystack session %d has an empty id", i)
		}
		if len(record.HaystackSessions[i]) == 0 {
			return fmt.Errorf("haystack session %q has no turns", id)
		}
		nonEmptyTurns := 0
		for _, turn := range record.HaystackSessions[i] {
			if strings.TrimSpace(turn.Role) == "" {
				return fmt.Errorf("haystack session %q has an empty role", id)
			}
			if strings.TrimSpace(turn.Content) == "" {
				if turn.HasAnswer {
					return fmt.Errorf("haystack session %q has an empty answer-labeled turn", id)
				}
				continue
			}
			nonEmptyTurns++
		}
		if nonEmptyTurns == 0 {
			return fmt.Errorf("haystack session %q has no non-empty turns", id)
		}
	}
	return nil
}

func SelectRecords(records []LongMemEvalRecord, ids []string) ([]LongMemEvalRecord, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("at least one selected record id is required")
	}
	byID := make(map[string]LongMemEvalRecord, len(records))
	for _, record := range records {
		byID[record.QuestionID] = record
	}
	selected := make([]LongMemEvalRecord, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, rawID := range ids {
		id := strings.TrimSpace(rawID)
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("selected record id %q is duplicate", id)
		}
		seen[id] = struct{}{}
		record, exists := byID[id]
		if !exists {
			return nil, fmt.Errorf("selected record id %q is missing from dataset", id)
		}
		selected = append(selected, record)
	}
	return selected, nil
}

func RetrieveSessions(record LongMemEvalRecord, limit int) []LongMemEvalSession {
	if limit <= 0 || limit > len(record.HaystackSessions) {
		limit = len(record.HaystackSessions)
	}
	queryTokens := tokenSet(normalizeText(record.Question))
	sessions := make([]LongMemEvalSession, 0, len(record.HaystackSessions))
	for i, turns := range record.HaystackSessions {
		session := LongMemEvalSession{
			ID:       record.HaystackSessionIDs[i],
			Date:     record.HaystackDates[i],
			Turns:    append([]LongMemEvalTurn(nil), turns...),
			Position: i,
		}
		session.score = overlapCount(queryTokens, tokenSet(normalizeText(session.SemanticText())))
		sessions = append(sessions, session)
	}
	sort.SliceStable(sessions, func(i, j int) bool {
		if sessions[i].score == sessions[j].score {
			return sessions[i].Position < sessions[j].Position
		}
		return sessions[i].score > sessions[j].score
	})
	return sessions[:limit]
}

func (session LongMemEvalSession) SemanticText() string {
	lines := make([]string, 0, len(session.Turns)+1)
	if date := strings.TrimSpace(session.Date); date != "" {
		lines = append(lines, "Date: "+date)
	}
	for _, turn := range session.Turns {
		role := strings.ToLower(strings.TrimSpace(turn.Role))
		content := strings.TrimSpace(turn.Content)
		if role == "" || content == "" {
			continue
		}
		lines = append(lines, role+": "+content)
	}
	return strings.Join(lines, "\n")
}

func ScoreAnswer(record LongMemEvalRecord, response string) DeterministicScore {
	normalizedResponse := normalizeText(response)
	best := DeterministicScore{
		AbstentionExpected: strings.HasSuffix(record.QuestionID, "_abs"),
		AbstentionDetected: detectAbstention(normalizedResponse),
		NormalizedResponse: normalizedResponse,
	}
	for _, variant := range answerVariants(record.Answer) {
		normalizedReference := normalizeText(variant)
		exact := normalizedReference != "" && (normalizedResponse == normalizedReference || containsNormalizedPhrase(normalizedResponse, normalizedReference))
		f1, recall := tokenMetrics(normalizedResponse, normalizedReference)
		if exact {
			f1 = 1
			recall = 1
		}
		if exact || f1 > best.TokenF1 || (f1 == best.TokenF1 && recall > best.AnswerTokenRecall) {
			best.ExactMatch = exact
			best.TokenF1 = roundMetric(f1)
			best.AnswerTokenRecall = roundMetric(recall)
			best.ReferenceVariant = variant
			best.NormalizedReference = normalizedReference
		}
	}
	return best
}

func answerVariants(answer string) []string {
	answer = strings.TrimSpace(answer)
	variants := []string{answer}
	lower := strings.ToLower(answer)
	if index := strings.Index(lower, " (or "); index >= 0 && strings.HasSuffix(answer, ")") {
		primary := strings.TrimSpace(answer[:index])
		alternate := strings.TrimSpace(answer[index+len(" (or ") : len(answer)-1])
		if primary != "" && alternate != "" {
			variants = []string{primary, alternate}
		}
	}
	return variants
}

func normalizeText(text string) string {
	var b strings.Builder
	spacePending := false
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if spacePending && b.Len() > 0 {
				b.WriteByte(' ')
			}
			spacePending = false
			b.WriteRune(r)
		} else {
			spacePending = true
		}
	}
	return strings.TrimSpace(b.String())
}

func containsNormalizedPhrase(response, reference string) bool {
	return strings.Contains(" "+response+" ", " "+reference+" ")
}

func tokenMetrics(prediction, reference string) (f1, recall float64) {
	predictionTokens := strings.Fields(prediction)
	referenceTokens := strings.Fields(reference)
	if len(predictionTokens) == 0 || len(referenceTokens) == 0 {
		return 0, 0
	}
	predictionCounts := tokenCounts(predictionTokens)
	referenceCounts := tokenCounts(referenceTokens)
	common := 0
	for token, count := range referenceCounts {
		if predictionCounts[token] < count {
			count = predictionCounts[token]
		}
		common += count
	}
	if common == 0 {
		return 0, 0
	}
	precision := float64(common) / float64(len(predictionTokens))
	recall = float64(common) / float64(len(referenceTokens))
	return 2 * precision * recall / (precision + recall), recall
}

func tokenCounts(tokens []string) map[string]int {
	counts := make(map[string]int, len(tokens))
	for _, token := range tokens {
		counts[token]++
	}
	return counts
}

func tokenSet(text string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, token := range strings.Fields(text) {
		if _, stopword := lexicalStopwords[token]; stopword {
			continue
		}
		set[token] = struct{}{}
	}
	return set
}

var lexicalStopwords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "are": {}, "did": {}, "do": {}, "i": {},
	"in": {}, "is": {}, "it": {}, "me": {}, "my": {}, "of": {}, "on": {},
	"the": {}, "to": {}, "was": {}, "what": {}, "when": {}, "where": {},
	"which": {}, "who": {}, "with": {},
}

func overlapCount(left, right map[string]struct{}) int {
	count := 0
	for token := range left {
		if _, exists := right[token]; exists {
			count++
		}
	}
	return count
}

func detectAbstention(normalized string) bool {
	phrases := []string{
		"not enough information",
		"insufficient information",
		"not mentioned",
		"cannot determine",
		"cannot be determined",
		"can t determine",
		"no information",
		"not provided",
		"wasn t provided",
		"do not have that information",
		"don t have that information",
		"unknown",
	}
	for _, phrase := range phrases {
		if containsNormalizedPhrase(normalized, phrase) {
			return true
		}
	}
	return false
}

func roundMetric(value float64) float64 {
	return math.Round(value*10000) / 10000
}

func decodeScalarString(raw json.RawMessage) (string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "", fmt.Errorf("must be a string or number")
	}
	if strings.HasPrefix(trimmed, "\"") {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", err
		}
		return value, nil
	}
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") || trimmed == "true" || trimmed == "false" {
		return "", fmt.Errorf("must be a string or number")
	}
	for _, r := range trimmed {
		if !(unicode.IsDigit(r) || r == '-' || r == '+' || r == '.' || r == 'e' || r == 'E') {
			return "", fmt.Errorf("must be a string or number")
		}
	}
	return trimmed, nil
}
