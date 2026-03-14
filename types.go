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
	Variables []Variable       `json:"variables,omitempty"`
	VarValues map[string]string `json:"var_values,omitempty"`
	Nodes     []NodeConfig     `json:"nodes"`
	Edges     []Edge           `json:"edges"`
	StartFrom string           `json:"start_from,omitempty"`
}

type Variable struct {
	Name        string `json:"name"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
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
	Text              string           `json:"text,omitempty"`
	Description       string           `json:"description,omitempty"`
	ProjectID         string           `json:"project_id,omitempty"`
	ProjectIDs        []string         `json:"project_ids,omitempty"`
	Prompt            string           `json:"prompt,omitempty"`
	AllowedTools      []string         `json:"allowed_tools,omitempty"`
	Model             string           `json:"model,omitempty"`
	OutputArtifacts   []OutputArtifact `json:"output_artifacts,omitempty"`
	TempFiles         []string         `json:"temp_files,omitempty"`
	Interactive       bool             `json:"interactive,omitempty"`
	Decompose         bool             `json:"decompose,omitempty"`
	MaxQuestionRounds int              `json:"max_question_rounds,omitempty"`
	MaxRetries        int              `json:"max_retries,omitempty"`
	RetryDelaySeconds int              `json:"retry_delay_seconds,omitempty"`
	ReviewerFor       string           `json:"reviewer_for,omitempty"`
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
	RetryAttempt  int        `json:"retry_attempt,omitempty"`
	ReviewRound   int        `json:"review_round,omitempty"`
	SubtaskIndex  int        `json:"subtask_index,omitempty"`
	SubtaskTotal  int        `json:"subtask_total,omitempty"`
	SubtaskLabel  string     `json:"subtask_label,omitempty"`
}
