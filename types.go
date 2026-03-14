package main

// ── Project ─────────────────────────────────

type Project struct {
	ID          string `json:"id"`
	Alias       string `json:"alias"`
	Path        string `json:"path"`
	Color       string `json:"color"`
	Description string `json:"description,omitempty"`
	HasClaudeMD bool   `json:"has_claude_md"`
}

type ProjectsFile struct {
	Projects []Project `json:"projects"`
}

// ── Pipeline config (what the editor produces) ──

type PipelineConfig struct {
	Version  int              `json:"version"`
	Name     string           `json:"name"`
	Settings PipelineSettings `json:"settings"`
	Nodes    []NodeConfig     `json:"nodes"`
	Edges    []Edge           `json:"edges"`
}

type PipelineSettings struct {
	Model            string `json:"model"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
	MaxReviewRetries int    `json:"max_review_retries"`
}

type NodeConfig struct {
	ID     string     `json:"id"`
	Type   string     `json:"type"`
	Label  string     `json:"label"`
	Config NodeDetail `json:"config"`
}

type NodeDetail struct {
	Description     string           `json:"description,omitempty"`
	ProjectID       string           `json:"project_id,omitempty"`
	Prompt          string           `json:"prompt,omitempty"`
	AllowedTools    []string         `json:"allowed_tools,omitempty"`
	ModelOverride   string           `json:"model_override,omitempty"`
	OutputArtifacts []OutputArtifact `json:"output_artifacts,omitempty"`
	ReviewMode      *ReviewMode      `json:"review_mode,omitempty"`
}

type OutputArtifact struct {
	File      string   `json:"file"`
	DeliverTo []string `json:"deliver_to"`
}

type ReviewMode struct {
	Enabled     bool              `json:"enabled"`
	PassPattern string            `json:"pass_pattern"`
	FailPattern string            `json:"fail_pattern"`
	FixTargets  map[string]string `json:"fix_targets"`
}

type Edge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// ── Run status ──────────────────────────────

type NodeStatus struct {
	Status    string  `json:"status"`
	Message   string  `json:"message"`
	StartedAt float64 `json:"started_at,omitempty"`
	Elapsed   int     `json:"elapsed"`
}
