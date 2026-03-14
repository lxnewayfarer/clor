package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var readRe = regexp.MustCompile(`\{read:([^}]+)\}`)
var filesRe = regexp.MustCompile(`\{files:([^}]+)\}`)
var varRe = regexp.MustCompile(`\{var:([^}]+)\}`)

// safePath resolves a file path under baseDir and ensures it does not escape via traversal.
func safePath(baseDir, file string) (string, bool) {
	abs := filepath.Join(baseDir, filepath.Clean(file))
	absBase, _ := filepath.Abs(baseDir)
	absTarget, _ := filepath.Abs(abs)
	if !strings.HasPrefix(absTarget, absBase+string(filepath.Separator)) && absTarget != absBase {
		return "", false
	}
	return absTarget, true
}

func expandReadVars(prompt, workdir string, projects map[string]Project) string {
	return readRe.ReplaceAllStringFunc(prompt, func(match string) string {
		ref := readRe.FindStringSubmatch(match)[1]

		if strings.Contains(ref, ":") {
			parts := strings.SplitN(ref, ":", 2)
			alias, file := parts[0], parts[1]
			for _, p := range projects {
				if p.Alias == alias {
					resolved, ok := safePath(p.Path, file)
					if !ok {
						return "[path traversal blocked: " + ref + "]"
					}
					data, err := os.ReadFile(resolved)
					if err != nil {
						return "[not found: " + ref + "]"
					}
					return string(data)
				}
			}
			return "[project not found: " + alias + "]"
		}

		resolved, ok := safePath(workdir, ref)
		if !ok {
			return "[path traversal blocked: " + ref + "]"
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			return "[not found: " + ref + "]"
		}
		return string(data)
	})
}

func expandFilesVars(prompt, workdir string) string {
	return filesRe.ReplaceAllStringFunc(prompt, func(match string) string {
		pattern := filesRe.FindStringSubmatch(match)[1]
		resolved, ok := safePath(workdir, pattern)
		if !ok {
			return "[path traversal blocked: " + pattern + "]"
		}
		var files []string
		filepath.Walk(resolved, func(path string, info os.FileInfo, err error) error {
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
		resolved, ok := safePath(workdir, "review.md")
		if !ok {
			return ""
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			return ""
		}
		return string(data)
	}())
}

func expandVars(prompt string, variables []Variable, values map[string]string) string {
	return varRe.ReplaceAllStringFunc(prompt, func(match string) string {
		name := varRe.FindStringSubmatch(match)[1]
		if val, ok := values[name]; ok && val != "" {
			return val
		}
		// Fall back to default
		for _, v := range variables {
			if v.Name == name && v.Default != "" {
				return v.Default
			}
		}
		return match // leave unexpanded
	})
}

func findByType(nodes []NodeConfig, nodeType string) *NodeConfig {
	for _, n := range nodes {
		if n.Type == nodeType {
			return &n
		}
	}
	return nil
}
