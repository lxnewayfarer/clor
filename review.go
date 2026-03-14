package main

import (
	"os"
	"regexp"
	"strings"
)

type ReviewResult struct {
	Passed bool
	Issues map[string][]string
}

func parseReview(path string) (ReviewResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ReviewResult{Passed: true}, err
	}
	text := string(data)

	passed, _ := regexp.MatchString(`(?i)review\s*status:\s*pass`, text)

	result := ReviewResult{
		Passed: passed,
		Issues: map[string][]string{
			"code_issues": extractSection(text, "## Code Issues"),
			"test_issues": extractSection(text, "## Test Issues"),
		},
	}
	return result, nil
}

func extractSection(text, header string) []string {
	var lines []string
	capturing := false
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, header) {
			capturing = true
			continue
		}
		if capturing && strings.HasPrefix(trimmed, "## ") {
			break
		}
		if capturing && strings.HasPrefix(trimmed, "- ") {
			lines = append(lines, trimmed)
		}
	}
	return lines
}
