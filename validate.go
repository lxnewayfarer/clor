package main

import (
	"os"
	"strings"
)

type ValidationError struct {
	NodeID  string `json:"node_id,omitempty"`
	Level   string `json:"level"` // "error", "warning", "info"
	Message string `json:"message"`
}

// ValidatePipeline checks a pipeline config for common issues.
func ValidatePipeline(cfg PipelineConfig, projects []Project) []ValidationError {
	var errs []ValidationError

	projMap := make(map[string]Project)
	for _, p := range projects {
		projMap[p.ID] = p
	}

	aliasMap := make(map[string]Project)
	for _, p := range projects {
		if p.Alias != "" {
			aliasMap[p.Alias] = p
		}
	}

	// Check for cycles (exclude review back-edges which are handled by the orchestrator)
	_, cycleErr := ComputeWaves(cfg.Nodes, FilterReviewBackEdges(cfg.Nodes, cfg.Edges))
	if cycleErr != nil {
		errs = append(errs, ValidationError{Level: "error", Message: cycleErr.Error()})
	}

	// Build connectivity info
	connected := make(map[string]bool)
	hasUpstream := make(map[string]bool)
	for _, e := range cfg.Edges {
		connected[e.Source] = true
		connected[e.Target] = true
		hasUpstream[e.Target] = true
	}

	for _, n := range cfg.Nodes {
		// Empty prompt for agent nodes
		if n.Type != "task" && strings.TrimSpace(n.Config.Prompt) == "" {
			errs = append(errs, ValidationError{
				NodeID:  n.ID,
				Level:   "error",
				Message: n.Label + ": empty prompt",
			})
		}

		// No project assigned to agent nodes
		if n.Type != "task" {
			pid := n.Config.ProjectID
			if len(n.Config.ProjectIDs) > 0 {
				pid = n.Config.ProjectIDs[0]
			}
			if pid == "" {
				errs = append(errs, ValidationError{
					NodeID:  n.ID,
					Level:   "warning",
					Message: n.Label + ": no project assigned",
				})
			} else if proj, ok := projMap[pid]; ok {
				// Check project path exists
				if _, err := os.Stat(proj.Path); err != nil {
					errs = append(errs, ValidationError{
						NodeID:  n.ID,
						Level:   "warning",
						Message: n.Label + ": project path does not exist: " + proj.Path,
					})
				}
			}
		}

		// Task node with no text
		if n.Type == "task" {
			text := n.Config.Text
			if text == "" {
				text = n.Config.Description
			}
			if strings.TrimSpace(text) == "" {
				errs = append(errs, ValidationError{
					NodeID:  n.ID,
					Level:   "warning",
					Message: n.Label + ": task has no text",
				})
			}
		}

		// Check {read:alias:file} references
		if n.Config.Prompt != "" {
			matches := readRe.FindAllStringSubmatch(n.Config.Prompt, -1)
			for _, m := range matches {
				ref := m[1]
				if strings.Contains(ref, ":") {
					parts := strings.SplitN(ref, ":", 2)
					alias := parts[0]
					if _, ok := aliasMap[alias]; !ok {
						errs = append(errs, ValidationError{
							NodeID:  n.ID,
							Level:   "warning",
							Message: n.Label + ": unknown project alias '" + alias + "' in {read:" + ref + "}",
						})
					}
				}
			}
		}

		// Decompose without upstream
		if n.Config.Decompose && !hasUpstream[n.ID] {
			errs = append(errs, ValidationError{
				NodeID:  n.ID,
				Level:   "warning",
				Message: n.Label + ": decompose enabled but no upstream edges (no architect output to parse)",
			})
		}

		// Isolated nodes (no connections)
		if !connected[n.ID] && len(cfg.Nodes) > 1 {
			errs = append(errs, ValidationError{
				NodeID:  n.ID,
				Level:   "info",
				Message: n.Label + ": isolated node (no connections)",
			})
		}
	}

	return errs
}
