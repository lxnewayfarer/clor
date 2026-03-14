package main

import (
	"regexp"
	"strings"
)

var reviewStatusRe = regexp.MustCompile(`(?mi)^##\s*Review\s*(?:Status)?\s*:\s*(PASS|FAIL)\s*$`)
var reviewIssuesRe = regexp.MustCompile(`(?ms)^##\s*Issues\s*\n(.+)`)

type ReviewResult struct {
	Pass   bool
	Issues []string
}

// ParseReviewOutput extracts PASS/FAIL status and issues from reviewer output.
// Expected format:
//
//	## Review: PASS
//	or
//	## Review: FAIL
//	## Issues
//	1. Description...
//	2. ...
func ParseReviewOutput(output string) ReviewResult {
	result := ReviewResult{Pass: true}

	match := reviewStatusRe.FindStringSubmatch(output)
	if match == nil {
		// No review marker found — treat as pass (non-reviewer output)
		return result
	}

	result.Pass = strings.EqualFold(match[1], "PASS")
	if result.Pass {
		return result
	}

	// Extract issues
	issuesMatch := reviewIssuesRe.FindStringSubmatch(output)
	if issuesMatch != nil {
		lines := strings.Split(strings.TrimSpace(issuesMatch[1]), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				result.Issues = append(result.Issues, line)
			}
		}
	}

	return result
}

// FormatIssuesMarkdown formats review issues as markdown for writing to review.md.
func FormatIssuesMarkdown(issues []string, round, maxRounds int) string {
	var sb strings.Builder
	sb.WriteString("# Review Issues\n\n")
	sb.WriteString("## Round " + strings.TrimSpace(strings.Repeat(" ", 0)) + "\n\n")
	// Simple formatting
	sb.Reset()
	sb.WriteString("# Review Issues (round ")
	sb.WriteString(itoa(round))
	sb.WriteString("/")
	sb.WriteString(itoa(maxRounds))
	sb.WriteString(")\n\n")
	for _, issue := range issues {
		sb.WriteString(issue)
		sb.WriteString("\n")
	}
	sb.WriteString("\nPlease fix ALL issues listed above.\n")
	return sb.String()
}

func itoa(n int) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}
