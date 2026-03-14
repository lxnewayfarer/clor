package main

import (
	"fmt"
	"regexp"
	"strings"
)

var questionsRe = regexp.MustCompile(`(?s)## Questions\n(.+?)(?:\n## |\z)`)
var numberedLineRe = regexp.MustCompile(`^\d+\.\s+(.+)`)

// parseQuestions extracts a ## Questions section from agent output.
// Returns nil if no questions found.
func parseQuestions(output string) []Question {
	m := questionsRe.FindStringSubmatch(output)
	if m == nil {
		return nil
	}

	var questions []Question
	for _, line := range strings.Split(strings.TrimSpace(m[1]), "\n") {
		line = strings.TrimSpace(line)
		if sub := numberedLineRe.FindStringSubmatch(line); sub != nil {
			questions = append(questions, Question{
				ID:   fmt.Sprintf("q%d", len(questions)+1),
				Text: sub[1],
			})
		}
	}
	if len(questions) == 0 {
		return nil
	}
	return questions
}

// formatAnswers renders Q&A pairs for prompt injection.
func formatAnswers(questions []Question) string {
	var sb strings.Builder
	for _, q := range questions {
		fmt.Fprintf(&sb, "Q: %s\nA: %s\n\n", q.Text, q.Answer)
	}
	return sb.String()
}
