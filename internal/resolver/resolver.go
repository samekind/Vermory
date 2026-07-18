package resolver

import (
	"path/filepath"
	"strings"

	"vermory/internal/domain"
)

type ResolutionStatus string

const (
	ResolutionResolved          ResolutionStatus = "resolved"
	ResolutionNeedsConfirmation ResolutionStatus = "needs_confirmation"
	ResolutionUnresolved        ResolutionStatus = "unresolved"
)

type WorkspaceInput struct {
	CWD                string
	ExplicitBindingID  string
	CandidateRepoRoot  string
	KnownWorkspaceName string
	Candidates         []WorkspaceCandidate
}

type WorkspaceCandidate struct {
	ID   string
	Path string
}

type ConversationInput struct {
	ThreadID       string
	Channel        string
	TopicName      string
	SuggestedLinks []ConversationCandidate
}

type ConversationCandidate struct {
	ID    string
	Label string
	Score float64
}

type Resolution struct {
	Status         ResolutionStatus
	Line           domain.ContinuityLine
	SpaceID        string
	Name           string
	Anchor         string
	AnchorStrength domain.AnchorStrength
	Candidates     []WorkspaceCandidate
	SuggestedLinks []ConversationCandidate
}

func ResolveWorkspace(input WorkspaceInput) Resolution {
	if strings.TrimSpace(input.ExplicitBindingID) != "" {
		for _, candidate := range input.Candidates {
			if candidate.ID == input.ExplicitBindingID &&
				strings.TrimSpace(input.CandidateRepoRoot) != "" &&
				filepathClean(candidate.Path) == filepathClean(input.CandidateRepoRoot) {
				return Resolution{
					Status:         ResolutionResolved,
					Line:           domain.ContinuityLineWorkspace,
					SpaceID:        candidate.ID,
					Name:           input.KnownWorkspaceName,
					Anchor:         candidate.Path,
					AnchorStrength: domain.AnchorStrengthStrong,
				}
			}
		}
		return Resolution{
			Status:         ResolutionNeedsConfirmation,
			Line:           domain.ContinuityLineWorkspace,
			Anchor:         firstNonEmpty(input.CandidateRepoRoot, input.CWD),
			AnchorStrength: domain.AnchorStrengthStrong,
			Candidates:     append([]WorkspaceCandidate(nil), input.Candidates...),
		}
	}

	if len(input.Candidates) > 1 {
		return Resolution{
			Status:         ResolutionNeedsConfirmation,
			Line:           domain.ContinuityLineWorkspace,
			Anchor:         input.CWD,
			AnchorStrength: domain.AnchorStrengthStrong,
			Candidates:     append([]WorkspaceCandidate(nil), input.Candidates...),
		}
	}

	if len(input.Candidates) == 1 {
		candidate := input.Candidates[0]
		return Resolution{
			Status:         ResolutionResolved,
			Line:           domain.ContinuityLineWorkspace,
			SpaceID:        candidate.ID,
			Anchor:         candidate.Path,
			AnchorStrength: domain.AnchorStrengthStrong,
		}
	}

	if strings.TrimSpace(input.CandidateRepoRoot) != "" {
		return Resolution{
			Status:         ResolutionNeedsConfirmation,
			Line:           domain.ContinuityLineWorkspace,
			Name:           input.KnownWorkspaceName,
			Anchor:         input.CandidateRepoRoot,
			AnchorStrength: domain.AnchorStrengthStrong,
		}
	}

	return Resolution{
		Status:         ResolutionUnresolved,
		Line:           domain.ContinuityLineWorkspace,
		Anchor:         input.CWD,
		AnchorStrength: domain.AnchorStrengthStrong,
	}
}

func filepathClean(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return filepath.Clean(value)
}

func ResolveConversation(input ConversationInput) Resolution {
	channel := strings.TrimSpace(input.Channel)
	if channel == "" {
		channel = "default"
	}
	if strings.TrimSpace(input.TopicName) != "" {
		return Resolution{
			Status:         ResolutionResolved,
			Line:           domain.ContinuityLineConversation,
			SpaceID:        "topic:" + channel + ":" + sanitizeID(input.TopicName),
			Name:           input.TopicName,
			Anchor:         input.TopicName,
			AnchorStrength: domain.AnchorStrengthWeak,
			SuggestedLinks: append([]ConversationCandidate(nil), input.SuggestedLinks...),
		}
	}
	if strings.TrimSpace(input.ThreadID) == "" {
		return Resolution{
			Status:         ResolutionUnresolved,
			Line:           domain.ContinuityLineConversation,
			AnchorStrength: domain.AnchorStrengthWeak,
			SuggestedLinks: append([]ConversationCandidate(nil), input.SuggestedLinks...),
		}
	}
	return Resolution{
		Status:         ResolutionResolved,
		Line:           domain.ContinuityLineConversation,
		SpaceID:        "thread:" + channel + ":" + input.ThreadID,
		Anchor:         input.ThreadID,
		AnchorStrength: domain.AnchorStrengthWeak,
		SuggestedLinks: append([]ConversationCandidate(nil), input.SuggestedLinks...),
	}
}

func AllowsGlobalDefault(claim domain.Claim) bool {
	switch claim.Type {
	case domain.ClaimTypePreference:
		return true
	case domain.ClaimTypeConstraint:
		content := strings.ToLower(claim.Content)
		return strings.Contains(content, "privacy") ||
			strings.Contains(content, "confirmation") ||
			strings.Contains(content, "style") ||
			strings.Contains(content, "format")
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func sanitizeID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "-")
	value = strings.ReplaceAll(value, "/", "-")
	return value
}
