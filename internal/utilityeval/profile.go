package utilityeval

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ProviderLane struct {
	Provider                 string `json:"provider"`
	Model                    string `json:"model"`
	RequiredForCompatibility bool   `json:"required_for_compatibility"`
}

type Profile struct {
	Version                  string         `json:"version"`
	ID                       string         `json:"id"`
	ProfileName              string         `json:"profile_name"`
	RealityCaseIDs           []string       `json:"reality_case_ids"`
	Conditions               []ConditionID  `json:"conditions"`
	ProviderLanes            []ProviderLane `json:"provider_lanes"`
	MinimumCompleteRealLanes int            `json:"minimum_complete_real_lanes"`
	CallsPerCompleteLane     int            `json:"calls_per_complete_lane"`
	MaxContextBytes          int            `json:"max_context_bytes"`
	MaxOutputTokens          int            `json:"max_output_tokens"`
	NativeWritebackStatus    string         `json:"native_writeback_status"`
	HardGateCount            int            `json:"hard_gate_count"`
	HardGates                []string       `json:"hard_gates"`
}

func LoadProfile(path string) (Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, err
	}
	var profile Profile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&profile); err != nil {
		return Profile{}, fmt.Errorf("utilityeval: decode profile: %w", err)
	}
	if err := profile.Validate(); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func (p Profile) Validate() error {
	if p.Version != "1" || strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.ProfileName) == "" {
		return errors.New("utilityeval: profile identity is invalid")
	}
	if len(p.RealityCaseIDs) == 0 || len(p.Conditions) != len(FrozenConditions) {
		return errors.New("utilityeval: profile case or condition set is invalid")
	}
	for index, expected := range FrozenConditions {
		if p.Conditions[index] != expected {
			return fmt.Errorf("utilityeval: profile condition %d is %q, expected %q", index, p.Conditions[index], expected)
		}
	}
	if len(p.ProviderLanes) == 0 || p.MinimumCompleteRealLanes < 1 || p.MinimumCompleteRealLanes > len(p.ProviderLanes) {
		return errors.New("utilityeval: profile provider lane requirements are invalid")
	}
	if p.CallsPerCompleteLane != len(p.RealityCaseIDs)*len(FrozenConditions) {
		return fmt.Errorf("utilityeval: calls_per_complete_lane must be %d", len(p.RealityCaseIDs)*len(FrozenConditions))
	}
	if p.MaxContextBytes <= 0 || p.MaxOutputTokens <= 0 || p.NativeWritebackStatus != "proposed" {
		return errors.New("utilityeval: profile execution limits are invalid")
	}
	if p.HardGateCount != len(p.HardGates) || p.HardGateCount == 0 {
		return errors.New("utilityeval: profile hard gate count is invalid")
	}
	seenCases := make(map[string]struct{}, len(p.RealityCaseIDs))
	for _, caseID := range p.RealityCaseIDs {
		if strings.TrimSpace(caseID) == "" {
			return errors.New("utilityeval: profile contains an empty case id")
		}
		if _, exists := seenCases[caseID]; exists {
			return fmt.Errorf("utilityeval: profile repeats case %q", caseID)
		}
		seenCases[caseID] = struct{}{}
	}
	for _, lane := range p.ProviderLanes {
		if strings.TrimSpace(lane.Provider) == "" || strings.TrimSpace(lane.Model) == "" {
			return errors.New("utilityeval: provider lane is incomplete")
		}
		if lane.Provider == "siliconflow" && strings.Contains(strings.ToLower(lane.Model), "/pro") {
			return fmt.Errorf("utilityeval: Pro SiliconFlow model is not allowed: %s", lane.Model)
		}
	}
	return nil
}

type ContextBundle struct {
	Version   string      `json:"version"`
	ProfileID string      `json:"profile_id"`
	Inputs    []CaseInput `json:"inputs"`
}

func (b ContextBundle) Validate() error {
	if b.Version != "1" || strings.TrimSpace(b.ProfileID) == "" || len(b.Inputs) == 0 {
		return errors.New("utilityeval: context bundle identity is invalid")
	}
	seen := make(map[string]struct{}, len(b.Inputs))
	for _, input := range b.Inputs {
		if err := input.Validate(); err != nil {
			return err
		}
		if _, exists := seen[input.ID]; exists {
			return fmt.Errorf("utilityeval: context bundle repeats case %q", input.ID)
		}
		seen[input.ID] = struct{}{}
	}
	return nil
}

func LoadContextBundle(path string) (ContextBundle, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ContextBundle{}, err
	}
	var bundle ContextBundle
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&bundle); err != nil {
		return ContextBundle{}, fmt.Errorf("utilityeval: decode context bundle: %w", err)
	}
	if err := bundle.Validate(); err != nil {
		return ContextBundle{}, err
	}
	return bundle, nil
}

func WriteContextBundle(path string, bundle ContextBundle) error {
	if err := bundle.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".utility-context-bundle-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
