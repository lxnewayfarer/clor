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
	Version   int              `json:"version"`
	Name      string           `json:"name"`
	Settings  PipelineSettings `json:"settings"`
	Nodes     []NodeConfig     `json:"nodes"`
	Edges     []Edge           `json:"edges"`
	StartFrom string           `json:"start_from,omitempty"`
}

type PipelineSettings struct {
	Model          string `json:"model"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

type NodeConfig struct {
	ID     string     `json:"id"`
	Type   string     `json:"type"`
	Label  string     `json:"label"`
	Config NodeDetail `json:"config"`
}

type NodeDetail struct {
	Description       string           `json:"description,omitempty"`
	ProjectID         string           `json:"project_id,omitempty"`
	ProjectIDs        []string         `json:"project_ids,omitempty"`
	Prompt            string           `json:"prompt,omitempty"`
	AllowedTools      []string         `json:"allowed_tools,omitempty"`
	Model             string           `json:"model,omitempty"`
	OutputArtifacts   []OutputArtifact `json:"output_artifacts,omitempty"`
	Interactive       bool             `json:"interactive,omitempty"`
	MaxQuestionRounds int              `json:"max_question_rounds,omitempty"`
}

type OutputArtifact struct {
	File      string   `json:"file"`
	DeliverTo []string `json:"deliver_to"`
}

type Edge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// ── Questions (interactive nodes) ───────────

type Question struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Answer string `json:"answer,omitempty"`
}

// ── Run status ──────────────────────────────

type NodeStatus struct {
	Status        string     `json:"status"`
	Message       string     `json:"message"`
	StartedAt     float64    `json:"started_at,omitempty"`
	Elapsed       int        `json:"elapsed"`
	Questions     []Question `json:"questions,omitempty"`
	QuestionRound int        `json:"question_round,omitempty"`
}
