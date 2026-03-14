package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var safeNameRe = regexp.MustCompile(`^[a-zA-Z0-9_\-]+$`)

func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "clor")
}

func setupRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	// Serve embedded frontend
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		data, err := content.ReadFile("web/index.html")
		if err != nil {
			http.Error(w, "frontend not found", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write(data)
	})

	// Projects
	mux.HandleFunc("GET /api/projects", handleGetProjects)
	mux.HandleFunc("POST /api/projects", handleSaveProjects)
	mux.HandleFunc("GET /api/projects/{id}/claude", handleGetClaudeMD)

	// Pipelines
	mux.HandleFunc("GET /api/pipelines", handleListPipelines)
	mux.HandleFunc("GET /api/pipelines/{name}", handleGetPipeline)
	mux.HandleFunc("POST /api/pipelines/{name}", handleSavePipeline)
	mux.HandleFunc("DELETE /api/pipelines/{name}", handleDeletePipeline)

	// Validation
	mux.HandleFunc("POST /api/validate", handleValidate)

	// Runs
	mux.HandleFunc("POST /api/run", handleStartRun)
	mux.HandleFunc("GET /api/run/{id}/status", handleRunStatus)
	mux.HandleFunc("GET /api/run/{id}/events", handleRunEvents)
	mux.HandleFunc("POST /api/run/{id}/stop", handleStopRun)
	mux.HandleFunc("GET /api/run/{id}/logs/{nodeId}", handleNodeLog)
	mux.HandleFunc("GET /api/run/{id}/logs/{nodeId}/stream", handleNodeLogStream)
	mux.HandleFunc("POST /api/run/{id}/retry/{nodeId}", handleRetryNode)
	mux.HandleFunc("POST /api/run/{id}/answer/{nodeId}", handleSubmitAnswer)

	return mux
}

// ── Projects ────────────────────────────────

func projectsPath() string {
	return filepath.Join(configDir(), "projects.json")
}

func loadProjects() ProjectsFile {
	var pf ProjectsFile
	data, err := os.ReadFile(projectsPath())
	if err != nil {
		return pf
	}
	json.Unmarshal(data, &pf)
	return pf
}

func handleGetProjects(w http.ResponseWriter, r *http.Request) {
	pf := loadProjects()
	for i := range pf.Projects {
		claudePath := filepath.Join(pf.Projects[i].Path, "CLAUDE.md")
		_, err := os.Stat(claudePath)
		pf.Projects[i].HasClaudeMD = err == nil
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pf)
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[1:])
	}
	return path
}

func handleSaveProjects(w http.ResponseWriter, r *http.Request) {
	var pf ProjectsFile
	json.NewDecoder(r.Body).Decode(&pf)
	for i := range pf.Projects {
		pf.Projects[i].Path = expandHome(pf.Projects[i].Path)
	}
	data, _ := json.MarshalIndent(pf, "", "  ")
	os.WriteFile(projectsPath(), data, 0644)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func handleGetClaudeMD(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !safeNameRe.MatchString(id) {
		http.Error(w, "invalid id", 400)
		return
	}
	pf := loadProjects()
	for _, p := range pf.Projects {
		if p.ID == id {
			content, err := os.ReadFile(filepath.Join(p.Path, "CLAUDE.md"))
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"content": nil})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"content": string(content)})
			return
		}
	}
	http.NotFound(w, r)
}

// ── Pipelines ───────────────────────────────

func pipelinesDir() string {
	return filepath.Join(configDir(), "pipelines")
}

func handleListPipelines(w http.ResponseWriter, r *http.Request) {
	entries, _ := os.ReadDir(pipelinesDir())
	names := []string{}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			names = append(names, strings.TrimSuffix(e.Name(), ".json"))
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"pipelines": names})
}

func handleGetPipeline(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !safeNameRe.MatchString(name) {
		http.Error(w, "invalid name", 400)
		return
	}
	data, err := os.ReadFile(filepath.Join(pipelinesDir(), name+".json"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func handleSavePipeline(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !safeNameRe.MatchString(name) {
		http.Error(w, "invalid name", 400)
		return
	}
	data, _ := io.ReadAll(r.Body)
	os.WriteFile(filepath.Join(pipelinesDir(), name+".json"), data, 0644)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func handleDeletePipeline(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !safeNameRe.MatchString(name) {
		http.Error(w, "invalid name", 400)
		return
	}
	os.Remove(filepath.Join(pipelinesDir(), name+".json"))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// ── Validation ─────────────────────────────

func handleValidate(w http.ResponseWriter, r *http.Request) {
	var config PipelineConfig
	json.NewDecoder(r.Body).Decode(&config)
	pf := loadProjects()
	errs := ValidatePipeline(config, pf.Projects)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"errors": errs})
}

// ── Runs ────────────────────────────────────

var activeRuns = make(map[string]*RunHandle)

type RunHandle struct {
	Cancel func()
	Orch   *Orchestrator
}

func handleStartRun(w http.ResponseWriter, r *http.Request) {
	var config PipelineConfig
	json.NewDecoder(r.Body).Decode(&config)
	if config.StartFrom != "" {
		config.Nodes, config.Edges = FilterDownstream(config.Nodes, config.Edges, config.StartFrom)
	}
	runID := generateID()
	orch := NewOrchestrator(config, runID)
	ctx, cancel := contextWithCancel()
	activeRuns[runID] = &RunHandle{Cancel: cancel, Orch: orch}
	go func() {
		orch.Run(ctx)
		delete(activeRuns, runID)
	}()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"run_id": runID})
}

func handleRunStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !safeNameRe.MatchString(id) {
		http.Error(w, "invalid id", 400)
		return
	}
	path := filepath.Join(configDir(), "runs", id, "status.json")
	data, err := os.ReadFile(path)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{}"))
		return
	}

	var statuses map[string]NodeStatus
	if err := json.Unmarshal(data, &statuses); err == nil {
		now := float64(time.Now().Unix())
		for k, s := range statuses {
			if s.Status == "running" && s.StartedAt > 0 {
				s.Elapsed = int(now - s.StartedAt)
				statuses[k] = s
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(statuses)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func handleRunEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !safeNameRe.MatchString(id) {
		http.Error(w, "invalid id", 400)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", 500)
		return
	}

	rh, ok := activeRuns[id]
	if !ok {
		http.Error(w, "run not found", 404)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := rh.Orch.Subscribe()
	defer rh.Orch.Unsubscribe(ch)

	ctx := r.Context()
	for {
		select {
		case statuses, open := <-ch:
			if !open {
				return
			}
			// Update elapsed for running nodes
			now := float64(time.Now().Unix())
			for k, s := range statuses {
				if s.Status == "running" && s.StartedAt > 0 {
					s.Elapsed = int(now - s.StartedAt)
					statuses[k] = s
				}
			}
			data, _ := json.Marshal(statuses)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-ctx.Done():
			return
		}
	}
}

func handleStopRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !safeNameRe.MatchString(id) {
		http.Error(w, "invalid id", 400)
		return
	}
	if rh, ok := activeRuns[id]; ok {
		rh.Cancel()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func handleNodeLog(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	nodeId := r.PathValue("nodeId")
	if !safeNameRe.MatchString(id) || !safeNameRe.MatchString(nodeId) {
		http.Error(w, "invalid id", 400)
		return
	}
	path := filepath.Join(configDir(), "runs", id, "logs", nodeId+".log")
	logContent, err := os.ReadFile(path)
	if err != nil {
		logContent = []byte("No log yet.")
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"log": string(logContent)})
}

func handleNodeLogStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	nodeId := r.PathValue("nodeId")
	if !safeNameRe.MatchString(id) || !safeNameRe.MatchString(nodeId) {
		http.Error(w, "invalid id", 400)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", 500)
		return
	}

	rh, ok := activeRuns[id]
	if !ok {
		http.Error(w, "run not found", 404)
		return
	}

	logStream := rh.Orch.GetLogStream(nodeId)
	if logStream == nil {
		http.Error(w, "no active stream for this node", 404)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := logStream.Subscribe()
	defer logStream.Unsubscribe(ch)

	ctx := r.Context()
	for {
		select {
		case chunk, open := <-ch:
			if !open {
				return
			}
			// Escape newlines for SSE
			escaped := strings.ReplaceAll(chunk, "\n", "\\n")
			fmt.Fprintf(w, "data: %s\n\n", escaped)
			flusher.Flush()
		case <-ctx.Done():
			return
		}
	}
}

func handleRetryNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	nodeId := r.PathValue("nodeId")
	if !safeNameRe.MatchString(id) || !safeNameRe.MatchString(nodeId) {
		http.Error(w, "invalid id", 400)
		return
	}

	rh, ok := activeRuns[id]
	if !ok {
		http.Error(w, "run not found", 404)
		return
	}

	go rh.Orch.RetryNode(nodeId)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func handleSubmitAnswer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	nodeId := r.PathValue("nodeId")

	rh, ok := activeRuns[id]
	if !ok {
		http.Error(w, "run not found", 404)
		return
	}

	var body struct {
		Answers []Question `json:"answers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", 400)
		return
	}

	if !rh.Orch.SubmitAnswer(nodeId, body.Answers) {
		http.Error(w, "node not waiting for input", 409)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
