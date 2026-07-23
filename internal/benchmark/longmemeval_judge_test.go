package benchmark

import (
	"strings"
	"testing"
)

func TestLongMemEvalJudgePromptMatchesPinnedUpstreamBranches(t *testing.T) {
	question := "What is the answer?"
	answer := "The complete answer."
	response := "A model response."
	generic := "I will give you a question, a correct answer, and a response from a model. Please answer yes if the response contains the correct answer. Otherwise, answer no. If the response is equivalent to the correct answer or contains all the intermediate steps to get the correct answer, you should also answer yes. If the response only contains a subset of the information required by the answer, answer no. \n\nQuestion: " + question + "\n\nCorrect Answer: " + answer + "\n\nModel Response: " + response + "\n\nIs the model response correct? Answer yes or no only."
	temporal := "I will give you a question, a correct answer, and a response from a model. Please answer yes if the response contains the correct answer. Otherwise, answer no. If the response is equivalent to the correct answer or contains all the intermediate steps to get the correct answer, you should also answer yes. If the response only contains a subset of the information required by the answer, answer no. In addition, do not penalize off-by-one errors for the number of days. If the question asks for the number of days/weeks/months, etc., and the model makes off-by-one errors (e.g., predicting 19 days when the answer is 18), the model's response is still correct. \n\nQuestion: " + question + "\n\nCorrect Answer: " + answer + "\n\nModel Response: " + response + "\n\nIs the model response correct? Answer yes or no only."
	update := "I will give you a question, a correct answer, and a response from a model. Please answer yes if the response contains the correct answer. Otherwise, answer no. If the response contains some previous information along with an updated answer, the response should be considered as correct as long as the updated answer is the required answer.\n\nQuestion: " + question + "\n\nCorrect Answer: " + answer + "\n\nModel Response: " + response + "\n\nIs the model response correct? Answer yes or no only."
	preference := "I will give you a question, a rubric for desired personalized response, and a response from a model. Please answer yes if the response satisfies the desired response. Otherwise, answer no. The model does not need to reflect all the points in the rubric. The response is correct as long as it recalls and utilizes the user's personal information correctly.\n\nQuestion: " + question + "\n\nRubric: " + answer + "\n\nModel Response: " + response + "\n\nIs the model response correct? Answer yes or no only."
	abstention := "I will give you an unanswerable question, an explanation, and a response from a model. Please answer yes if the model correctly identifies the question as unanswerable. The model could say that the information is incomplete, or some other information is given but the asked information is not.\n\nQuestion: " + question + "\n\nExplanation: " + answer + "\n\nModel Response: " + response + "\n\nDoes the model correctly identify the question as unanswerable? Answer yes or no only."

	tests := []struct {
		name         string
		questionType string
		recordID     string
		want         string
	}{
		{"single-session-user", "single-session-user", "user-record", generic},
		{"single-session-assistant", "single-session-assistant", "assistant-record", generic},
		{"multi-session", "multi-session", "multi-record", generic},
		{"temporal-reasoning", "temporal-reasoning", "temporal-record", temporal},
		{"knowledge-update", "knowledge-update", "update-record", update},
		{"single-session-preference", "single-session-preference", "preference-record", preference},
		{"abstention", "single-session-user", "user-record_abs", abstention},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := LongMemEvalRecord{QuestionID: test.recordID, QuestionType: test.questionType, Question: question, Answer: answer}
			got, err := LongMemEvalJudgePrompt(record, response)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("prompt differs from pinned upstream branch:\nwant=%q\ngot =%q", test.want, got)
			}
		})
	}
}

func TestLongMemEvalJudgePromptRejectsUnsupportedOrIncompleteInput(t *testing.T) {
	tests := []LongMemEvalRecord{
		{QuestionID: "record", QuestionType: "unsupported", Question: "q", Answer: "a"},
		{QuestionID: "record", QuestionType: "multi-session", Question: "", Answer: "a"},
		{QuestionID: "record", QuestionType: "multi-session", Question: "q", Answer: ""},
	}
	for _, record := range tests {
		if _, err := LongMemEvalJudgePrompt(record, "response"); err == nil {
			t.Fatalf("expected prompt rejection for %#v", record)
		}
	}
	if _, err := LongMemEvalJudgePrompt(LongMemEvalRecord{QuestionID: "record", QuestionType: "multi-session", Question: "q", Answer: "a"}, " "); err == nil {
		t.Fatal("expected empty response rejection")
	}
}

func TestParseLongMemEvalJudgeLabelIsStrict(t *testing.T) {
	valid := map[string]bool{
		"yes":    true,
		" Yes. ": true,
		"YES!\n": true,
		"no":     false,
		" NO. ":  false,
		"no?":    false,
	}
	for input, want := range valid {
		got, err := ParseLongMemEvalJudgeLabel(input)
		if err != nil || got != want {
			t.Fatalf("parse %q = %t, %v; want %t", input, got, err, want)
		}
	}
	for _, input := range []string{"", "yes because it matches", "no because it is incomplete", "yes and no", "maybe", strings.Repeat("yes ", 2)} {
		if _, err := ParseLongMemEvalJudgeLabel(input); err == nil {
			t.Fatalf("expected strict rejection for %q", input)
		}
	}
}
