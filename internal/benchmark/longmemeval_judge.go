package benchmark

import (
	"fmt"
	"strings"
	"unicode"
)

const (
	longMemEvalJudgeGenericTemplate    = "I will give you a question, a correct answer, and a response from a model. Please answer yes if the response contains the correct answer. Otherwise, answer no. If the response is equivalent to the correct answer or contains all the intermediate steps to get the correct answer, you should also answer yes. If the response only contains a subset of the information required by the answer, answer no. \n\nQuestion: %s\n\nCorrect Answer: %s\n\nModel Response: %s\n\nIs the model response correct? Answer yes or no only."
	longMemEvalJudgeTemporalTemplate   = "I will give you a question, a correct answer, and a response from a model. Please answer yes if the response contains the correct answer. Otherwise, answer no. If the response is equivalent to the correct answer or contains all the intermediate steps to get the correct answer, you should also answer yes. If the response only contains a subset of the information required by the answer, answer no. In addition, do not penalize off-by-one errors for the number of days. If the question asks for the number of days/weeks/months, etc., and the model makes off-by-one errors (e.g., predicting 19 days when the answer is 18), the model's response is still correct. \n\nQuestion: %s\n\nCorrect Answer: %s\n\nModel Response: %s\n\nIs the model response correct? Answer yes or no only."
	longMemEvalJudgeUpdateTemplate     = "I will give you a question, a correct answer, and a response from a model. Please answer yes if the response contains the correct answer. Otherwise, answer no. If the response contains some previous information along with an updated answer, the response should be considered as correct as long as the updated answer is the required answer.\n\nQuestion: %s\n\nCorrect Answer: %s\n\nModel Response: %s\n\nIs the model response correct? Answer yes or no only."
	longMemEvalJudgePreferenceTemplate = "I will give you a question, a rubric for desired personalized response, and a response from a model. Please answer yes if the response satisfies the desired response. Otherwise, answer no. The model does not need to reflect all the points in the rubric. The response is correct as long as it recalls and utilizes the user's personal information correctly.\n\nQuestion: %s\n\nRubric: %s\n\nModel Response: %s\n\nIs the model response correct? Answer yes or no only."
	longMemEvalJudgeAbstentionTemplate = "I will give you an unanswerable question, an explanation, and a response from a model. Please answer yes if the model correctly identifies the question as unanswerable. The model could say that the information is incomplete, or some other information is given but the asked information is not.\n\nQuestion: %s\n\nExplanation: %s\n\nModel Response: %s\n\nDoes the model correctly identify the question as unanswerable? Answer yes or no only."
)

func LongMemEvalJudgePrompt(record LongMemEvalRecord, response string) (string, error) {
	question := strings.TrimSpace(record.Question)
	answer := strings.TrimSpace(record.Answer)
	response = strings.TrimSpace(response)
	if strings.TrimSpace(record.QuestionID) == "" || strings.TrimSpace(record.QuestionType) == "" {
		return "", fmt.Errorf("LongMemEval judge record ID and question type are required")
	}
	if question == "" || answer == "" || response == "" {
		return "", fmt.Errorf("LongMemEval judge question, answer, and response are required")
	}
	if strings.Contains(record.QuestionID, "_abs") {
		return fmt.Sprintf(longMemEvalJudgeAbstentionTemplate, question, answer, response), nil
	}
	var template string
	switch record.QuestionType {
	case "single-session-user", "single-session-assistant", "multi-session":
		template = longMemEvalJudgeGenericTemplate
	case "temporal-reasoning":
		template = longMemEvalJudgeTemporalTemplate
	case "knowledge-update":
		template = longMemEvalJudgeUpdateTemplate
	case "single-session-preference":
		template = longMemEvalJudgePreferenceTemplate
	default:
		return "", fmt.Errorf("unsupported LongMemEval judge question type %q", record.QuestionType)
	}
	return fmt.Sprintf(template, question, answer, response), nil
}

func ParseLongMemEvalJudgeLabel(output string) (bool, error) {
	normalized := strings.ToLower(strings.TrimSpace(output))
	normalized = strings.TrimSpace(strings.TrimRightFunc(normalized, unicode.IsPunct))
	switch normalized {
	case "yes":
		return true, nil
	case "no":
		return false, nil
	default:
		return false, fmt.Errorf("invalid LongMemEval judge label %q", strings.TrimSpace(output))
	}
}
