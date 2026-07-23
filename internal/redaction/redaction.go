package redaction

import "regexp"

type Result struct {
	Text  string
	Count int
}

type rule struct {
	re          *regexp.Regexp
	replacement string
}

var rules = []rule{
	{re: regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`), replacement: "[REDACTED_API_KEY]"},
	{re: regexp.MustCompile(`(?i)\b(?:api[_-]?token|api[_-]?key|secret|token)\s*[:=]\s*[^\s,;]{8,}`), replacement: "[REDACTED_CREDENTIAL]"},
	{re: regexp.MustCompile(`-----BEGIN(?: [A-Z0-9]+)* PRIVATE KEY-----`), replacement: "[REDACTED_PRIVATE_KEY]"},
	{re: regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`), replacement: "[REDACTED_EMAIL]"},
	{re: regexp.MustCompile(`\b1[3-9]\d{9}\b`), replacement: "[REDACTED_PHONE]"},
}

func Redact(input string) Result {
	text := input
	count := 0
	for _, currentRule := range rules {
		matches := currentRule.re.FindAllString(text, -1)
		count += len(matches)
		text = currentRule.re.ReplaceAllString(text, currentRule.replacement)
	}

	return Result{
		Text:  text,
		Count: count,
	}
}

func ContainsSensitive(input string) bool {
	for _, currentRule := range rules {
		if currentRule.re.MatchString(input) {
			return true
		}
	}

	return false
}
