package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var readRe = regexp.MustCompile(`\{read:([^}]+)\}`)
var filesRe = regexp.MustCompile(`\{files:([^}]+)\}`)

func expandReadVars(prompt, workdir string, projects map[string]Project) string {
	return readRe.ReplaceAllStringFunc(prompt, func(match string) string {
		ref := readRe.FindStringSubmatch(match)[1]

		if strings.Contains(ref, ":") {
			parts := strings.SplitN(ref, ":", 2)
			alias, file := parts[0], parts[1]
			for _, p := range projects {
				if p.Alias == alias {
					data, err := os.ReadFile(filepath.Join(p.Path, file))
					if err != nil {
						return "[not found: " + ref + "]"
					}
					return string(data)
				}
			}
			return "[project not found: " + alias + "]"
		}

		data, err := os.ReadFile(filepath.Join(workdir, ref))
		if err != nil {
			return "[not found: " + ref + "]"
		}
		return string(data)
	})
}

func expandFilesVars(prompt, workdir string) string {
	return filesRe.ReplaceAllStringFunc(prompt, func(match string) string {
		pattern := filesRe.FindStringSubmatch(match)[1]
		var files []string
		filepath.Walk(filepath.Join(workdir, pattern), func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				rel, _ := filepath.Rel(workdir, path)
				files = append(files, rel)
			}
			return nil
		})
		return strings.Join(files, "\n")
	})
}

func expandReviewIssues(prompt, workdir string) string {
	return strings.ReplaceAll(prompt, "{review_issues}", func() string {
		review, err := parseReview(filepath.Join(workdir, "review.md"))
		if err != nil {
			return ""
		}
		var all []string
		for _, issues := range review.Issues {
			all = append(all, issues...)
		}
		return strings.Join(all, "\n")
	}())
}

func findByType(nodes []NodeConfig, nodeType string) *NodeConfig {
	for _, n := range nodes {
		if n.Type == nodeType {
			return &n
		}
	}
	return nil
}
