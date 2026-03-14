package main

import (
	"regexp"
	"strings"
)

var subtaskHeaderRe = regexp.MustCompile(`(?im)^##\s+(Subtasks|Sub-tasks|Task Breakdown)\s*$`)
var nextHeaderRe = regexp.MustCompile(`(?m)^##\s+`)
var listItemRe = regexp.MustCompile(`(?m)^\s*(?:\d+[.)]\s+|-\s+)(.+)`)

// ParseSubtasks extracts subtask descriptions from an architect's output.
// It looks for a ## Subtasks / ## Sub-tasks / ## Task Breakdown header,
// then collects numbered or bulleted items until the next ## header or end of text.
// Returns nil if no subtasks section is found.
func ParseSubtasks(output string) []string {
	loc := subtaskHeaderRe.FindStringIndex(output)
	if loc == nil {
		return nil
	}

	// Start after the header line
	rest := output[loc[1]:]

	// Find end boundary (next ## header or end)
	if endLoc := nextHeaderRe.FindStringIndex(rest); endLoc != nil {
		rest = rest[:endLoc[0]]
	}

	matches := listItemRe.FindAllStringSubmatch(rest, -1)
	if len(matches) == 0 {
		return nil
	}

	var subtasks []string
	for _, m := range matches {
		text := strings.TrimSpace(m[1])
		if text != "" {
			subtasks = append(subtasks, text)
		}
	}

	if len(subtasks) == 0 {
		return nil
	}
	return subtasks
}
