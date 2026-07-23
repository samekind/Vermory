package retrievalablation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Corpus struct {
	Version string   `json:"version"`
	Name    string   `json:"name"`
	Scopes  []Scope  `json:"scopes"`
	Records []Record `json:"records"`
	Queries []Query  `json:"queries"`
}

type Scope struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`
	Line     string `json:"line"`
	Anchor   string `json:"anchor"`
}

type Record struct {
	ID             string `json:"id"`
	ScopeID        string `json:"scope_id"`
	MemoryKey      string `json:"memory_key,omitempty"`
	Content        string `json:"content"`
	Lifecycle      string `json:"lifecycle"`
	ReplacementID  string `json:"replacement_id,omitempty"`
	ProvenanceCase string `json:"provenance_case"`
}

type Query struct {
	ID                    string   `json:"id"`
	ScopeID               string   `json:"scope_id"`
	Text                  string   `json:"text"`
	Limit                 int      `json:"limit"`
	RelevantRecordIDs     []string `json:"relevant_record_ids"`
	ForbiddenRecordIDs    []string `json:"forbidden_record_ids"`
	TaskExcludedRecordIDs []string `json:"task_excluded_record_ids,omitempty"`
	Cohorts               []string `json:"cohorts"`
}

func LoadCorpus(path string) (Corpus, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return Corpus{}, fmt.Errorf("read retrieval corpus: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var corpus Corpus
	if err := decoder.Decode(&corpus); err != nil {
		return Corpus{}, fmt.Errorf("decode retrieval corpus: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Corpus{}, fmt.Errorf("decode retrieval corpus: trailing JSON")
		}
		return Corpus{}, fmt.Errorf("decode retrieval corpus trailer: %w", err)
	}
	return corpus, nil
}

func ValidateCorpus(root string, corpus Corpus) error {
	if strings.TrimSpace(corpus.Version) == "" || strings.TrimSpace(corpus.Name) == "" {
		return fmt.Errorf("retrieval corpus requires version and name")
	}
	if len(corpus.Scopes) == 0 || len(corpus.Records) == 0 || len(corpus.Queries) == 0 {
		return fmt.Errorf("retrieval corpus requires scopes, records, and queries")
	}
	scopes := make(map[string]Scope, len(corpus.Scopes))
	for index, scope := range corpus.Scopes {
		scope.ID = strings.TrimSpace(scope.ID)
		scope.TenantID = strings.TrimSpace(scope.TenantID)
		scope.Line = strings.TrimSpace(scope.Line)
		scope.Anchor = strings.TrimSpace(scope.Anchor)
		if scope.ID == "" || scope.TenantID == "" || scope.Anchor == "" {
			return fmt.Errorf("scope %d requires id, tenant_id, and anchor", index)
		}
		if scope.Line != "workspace" && scope.Line != "conversation" {
			return fmt.Errorf("scope %q has unsupported line %q", scope.ID, scope.Line)
		}
		if _, exists := scopes[scope.ID]; exists {
			return fmt.Errorf("duplicate scope id %q", scope.ID)
		}
		scopes[scope.ID] = scope
	}

	records := make(map[string]Record, len(corpus.Records))
	for index, record := range corpus.Records {
		record.ID = strings.TrimSpace(record.ID)
		record.ScopeID = strings.TrimSpace(record.ScopeID)
		record.Content = strings.TrimSpace(record.Content)
		record.Lifecycle = strings.TrimSpace(record.Lifecycle)
		record.ReplacementID = strings.TrimSpace(record.ReplacementID)
		record.ProvenanceCase = strings.TrimSpace(record.ProvenanceCase)
		if record.ID == "" || record.ScopeID == "" || record.Content == "" || record.ProvenanceCase == "" {
			return fmt.Errorf("record %d requires id, scope_id, content, and provenance_case", index)
		}
		if _, exists := records[record.ID]; exists {
			return fmt.Errorf("duplicate record id %q", record.ID)
		}
		if _, exists := scopes[record.ScopeID]; !exists {
			return fmt.Errorf("record %q references unknown scope %q", record.ID, record.ScopeID)
		}
		switch record.Lifecycle {
		case "active", "proposed", "superseded", "deleted":
		default:
			return fmt.Errorf("record %q has unsupported lifecycle %q", record.ID, record.Lifecycle)
		}
		if record.Lifecycle == "superseded" && record.ReplacementID == "" {
			return fmt.Errorf("superseded record %q requires replacement_id", record.ID)
		}
		if record.Lifecycle != "superseded" && record.ReplacementID != "" {
			return fmt.Errorf("record %q has replacement_id outside superseded lifecycle", record.ID)
		}
		if err := validateProvenanceCase(root, record.ProvenanceCase); err != nil {
			return fmt.Errorf("record %q provenance: %w", record.ID, err)
		}
		records[record.ID] = record
	}
	for _, record := range records {
		if record.Lifecycle != "superseded" {
			continue
		}
		replacement, exists := records[record.ReplacementID]
		if !exists {
			return fmt.Errorf("superseded record %q references unknown replacement %q", record.ID, record.ReplacementID)
		}
		if replacement.Lifecycle != "active" || replacement.ScopeID != record.ScopeID {
			return fmt.Errorf("superseded record %q replacement must be active in the same scope", record.ID)
		}
	}

	queryIDs := make(map[string]struct{}, len(corpus.Queries))
	for index, query := range corpus.Queries {
		query.ID = strings.TrimSpace(query.ID)
		query.ScopeID = strings.TrimSpace(query.ScopeID)
		query.Text = strings.TrimSpace(query.Text)
		if query.ID == "" || query.ScopeID == "" || query.Text == "" {
			return fmt.Errorf("query %d requires id, scope_id, and text", index)
		}
		if _, exists := queryIDs[query.ID]; exists {
			return fmt.Errorf("duplicate query id %q", query.ID)
		}
		queryIDs[query.ID] = struct{}{}
		if _, exists := scopes[query.ScopeID]; !exists {
			return fmt.Errorf("query %q references unknown scope %q", query.ID, query.ScopeID)
		}
		if query.Limit < 1 || query.Limit > 12 {
			return fmt.Errorf("query %q limit must be between 1 and 12", query.ID)
		}
		if len(query.RelevantRecordIDs) == 0 || len(query.ForbiddenRecordIDs) == 0 || len(query.Cohorts) == 0 {
			return fmt.Errorf("query %q requires relevant, forbidden, and cohort values", query.ID)
		}
		relevant := make(map[string]struct{}, len(query.RelevantRecordIDs))
		for _, recordID := range query.RelevantRecordIDs {
			record, exists := records[recordID]
			if !exists {
				return fmt.Errorf("query %q references unknown relevant record %q", query.ID, recordID)
			}
			if record.Lifecycle != "active" {
				return fmt.Errorf("query %q relevant record %q must be active", query.ID, recordID)
			}
			if record.ScopeID != query.ScopeID {
				return fmt.Errorf("query %q relevant record %q must share its scope", query.ID, recordID)
			}
			if _, duplicate := relevant[recordID]; duplicate {
				return fmt.Errorf("query %q repeats relevant record %q", query.ID, recordID)
			}
			relevant[recordID] = struct{}{}
		}
		taskExcluded := make(map[string]struct{}, len(query.TaskExcludedRecordIDs))
		for _, recordID := range query.TaskExcludedRecordIDs {
			record, exists := records[recordID]
			if !exists {
				return fmt.Errorf("query %q references unknown task-excluded record %q", query.ID, recordID)
			}
			if record.ScopeID != query.ScopeID || record.Lifecycle != "active" {
				return fmt.Errorf("query %q task-excluded record %q must be active in the same scope", query.ID, recordID)
			}
			taskExcluded[recordID] = struct{}{}
		}
		forbidden := make(map[string]struct{}, len(query.ForbiddenRecordIDs))
		for _, recordID := range query.ForbiddenRecordIDs {
			record, exists := records[recordID]
			if !exists {
				return fmt.Errorf("query %q references unknown forbidden record %q", query.ID, recordID)
			}
			if _, overlap := relevant[recordID]; overlap {
				return fmt.Errorf("query %q record %q is both relevant and forbidden", query.ID, recordID)
			}
			if record.ScopeID == query.ScopeID && record.Lifecycle == "active" {
				if _, explicitlyExcluded := taskExcluded[recordID]; explicitlyExcluded {
					forbidden[recordID] = struct{}{}
					continue
				}
				return fmt.Errorf("query %q marks same-scope active distractor %q as forbidden", query.ID, recordID)
			}
			forbidden[recordID] = struct{}{}
		}
		for recordID := range taskExcluded {
			if _, exists := forbidden[recordID]; !exists {
				return fmt.Errorf("query %q task-excluded record %q must also be forbidden", query.ID, recordID)
			}
		}
	}
	return nil
}

func CorpusSHA256(corpus Corpus) (string, error) {
	canonical := corpus
	canonical.Scopes = append([]Scope(nil), corpus.Scopes...)
	canonical.Records = append([]Record(nil), corpus.Records...)
	canonical.Queries = append([]Query(nil), corpus.Queries...)
	sort.Slice(canonical.Scopes, func(i, j int) bool { return canonical.Scopes[i].ID < canonical.Scopes[j].ID })
	sort.Slice(canonical.Records, func(i, j int) bool { return canonical.Records[i].ID < canonical.Records[j].ID })
	for index := range canonical.Queries {
		canonical.Queries[index].RelevantRecordIDs = sortedStrings(canonical.Queries[index].RelevantRecordIDs)
		canonical.Queries[index].ForbiddenRecordIDs = sortedStrings(canonical.Queries[index].ForbiddenRecordIDs)
		canonical.Queries[index].TaskExcludedRecordIDs = sortedStrings(canonical.Queries[index].TaskExcludedRecordIDs)
		canonical.Queries[index].Cohorts = sortedStrings(canonical.Queries[index].Cohorts)
	}
	sort.Slice(canonical.Queries, func(i, j int) bool { return canonical.Queries[i].ID < canonical.Queries[j].ID })
	payload, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("encode canonical retrieval corpus: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func validateProvenanceCase(root, caseName string) error {
	if caseName == "." || caseName == ".." || filepath.Base(caseName) != caseName {
		return fmt.Errorf("case %q must be one directory name", caseName)
	}
	caseRoot := filepath.Join(root, "casebook", "cases")
	sourcePath := filepath.Join(caseRoot, caseName, "source.md")
	relative, err := filepath.Rel(caseRoot, sourcePath)
	if err != nil || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || relative == ".." {
		return fmt.Errorf("case %q escapes casebook root", caseName)
	}
	if info, err := os.Stat(sourcePath); err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("case %q source.md does not exist", caseName)
	}
	return nil
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}
