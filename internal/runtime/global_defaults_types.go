package runtime

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	maxGlobalDefaultKeyBytes     = 64
	maxGlobalDefaultContentBytes = 128 * 1024
)

var globalDefaultKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$`)

type SetGlobalDefaultRequest struct {
	OperationID string `json:"operation_id"`
	Key         string `json:"key"`
	Content     string `json:"content"`
}

func (r *SetGlobalDefaultRequest) Validate() error {
	if err := normalizeGlobalDefaultMutation(&r.OperationID, &r.Content); err != nil {
		return err
	}
	r.Key = strings.TrimSpace(r.Key)
	if r.Key == "" {
		return fmt.Errorf("key is required")
	}
	if len(r.Key) > maxGlobalDefaultKeyBytes {
		return fmt.Errorf("key is too long")
	}
	if !globalDefaultKeyPattern.MatchString(r.Key) {
		return fmt.Errorf("key must use lowercase letters, numbers, and underscores")
	}
	return nil
}

type CorrectGlobalDefaultRequest struct {
	OperationID string `json:"operation_id"`
	MemoryID    string `json:"memory_id"`
	Content     string `json:"content"`
}

func (r *CorrectGlobalDefaultRequest) Validate() error {
	if err := normalizeGlobalDefaultMutation(&r.OperationID, &r.Content); err != nil {
		return err
	}
	r.MemoryID = strings.TrimSpace(r.MemoryID)
	if r.MemoryID == "" {
		return fmt.Errorf("memory_id is required")
	}
	return nil
}

type ForgetGlobalDefaultRequest struct {
	OperationID string `json:"operation_id"`
	MemoryID    string `json:"memory_id"`
}

func (r *ForgetGlobalDefaultRequest) Validate() error {
	r.OperationID = strings.TrimSpace(r.OperationID)
	r.MemoryID = strings.TrimSpace(r.MemoryID)
	if r.OperationID == "" {
		return fmt.Errorf("operation_id is required")
	}
	if len(r.OperationID) > 512 {
		return fmt.Errorf("operation_id is too long")
	}
	if r.MemoryID == "" {
		return fmt.Errorf("memory_id is required")
	}
	return nil
}

type GlobalDefaultMutationReceipt struct {
	ContinuityID  string `json:"continuity_id"`
	ObservationID string `json:"observation_id"`
	MemoryID      string `json:"memory_id"`
	MemoryStatus  string `json:"memory_status"`
	Replayed      bool   `json:"replayed"`
}

type GlobalDefaultsInspection struct {
	ContinuityID string           `json:"continuity_id"`
	Defaults     []GovernedMemory `json:"defaults"`
}

func normalizeGlobalDefaultMutation(operationID, content *string) error {
	*operationID = strings.TrimSpace(*operationID)
	*content = strings.TrimSpace(*content)
	if *operationID == "" {
		return fmt.Errorf("operation_id is required")
	}
	if len(*operationID) > 512 {
		return fmt.Errorf("operation_id is too long")
	}
	if *content == "" {
		return fmt.Errorf("content is required")
	}
	if len(*content) > maxGlobalDefaultContentBytes {
		return fmt.Errorf("content is too long")
	}
	return nil
}
