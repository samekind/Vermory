package packet

import (
	"fmt"
	"strings"

	"vermory/internal/domain"
	"vermory/internal/redaction"
)

type Packet struct {
	ProfileID      ProfileID
	Title          string
	Body           string
	RedactionCount int
	TokenEstimate  int
}

func Build(profile ProfileID, projectName string, purpose string, claims []domain.Claim) Packet {
	var builder strings.Builder

	builder.WriteString("# " + projectName + "\n\n")
	builder.WriteString("用途：" + purpose + "\n\n")
	builder.WriteString(profileIntro(profile))
	builder.WriteString("\n\n## 已确认上下文\n")

	for _, claim := range claims {
		if claim.Status != domain.ClaimStatusConfirmed && claim.Status != domain.ClaimStatusActive {
			continue
		}
		if !claim.VerifiedByUser {
			continue
		}

		builder.WriteString(fmt.Sprintf("- [%s] %s\n", claim.Type, claim.Content))
	}

	redacted := redaction.Redact(builder.String())
	redactedTitle := redaction.Redact(projectName + " " + string(profile))

	return Packet{
		ProfileID:      profile,
		Title:          redactedTitle.Text,
		Body:           redacted.Text,
		RedactionCount: redacted.Count + redactedTitle.Count,
		TokenEstimate:  estimateTokens(redacted.Text),
	}
}

func profileIntro(profile ProfileID) string {
	switch profile {
	case ProfileGeneralChineseChat:
		return "目标平台：通用中文对话平台，如通义千问、豆包、Kimi、文心、智谱、腾讯元宝。"
	case ProfileCodingAgent:
		return "目标平台：代码开发工具，如 Codex、Cursor、Claude Code、Grok、通义灵码、CodeGeeX、Comate、Trae、MarsCode。"
	case ProfileTeamHandoff:
		return "目标平台：团队交接。请先说明当前目标、已确认决策和下一步。"
	default:
		return "目标平台：通用上下文包。"
	}
}

func estimateTokens(text string) int {
	return len([]rune(text))/2 + 1
}
