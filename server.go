package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

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

	// Runs
	mux.HandleFunc("POST /api/run", handleStartRun)
	mux.HandleFunc("GET /api/run/{id}/status", handleRunStatus)
	mux.HandleFunc("POST /api/run/{id}/stop", handleStopRun)
	mux.HandleFunc("GET /api/run/{id}/logs/{nodeId}", handleNodeLog)

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

func handleSaveProjects(w http.ResponseWriter, r *http.Request) {
	var pf ProjectsFile
	json.NewDecoder(r.Body).Decode(&pf)
	data, _ := json.MarshalIndent(pf, "", "  ")
	os.WriteFile(projectsPath(), data, 0644)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func handleGetClaudeMD(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
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
	data, _ := io.ReadAll(r.Body)
	os.WriteFile(filepath.Join(pipelinesDir(), name+".json"), data, 0644)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func handleDeletePipeline(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	os.Remove(filepath.Join(pipelinesDir(), name+".json"))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// ── Runs ────────────────────────────────────

var activeRuns = make(map[string]*RunHandle)

type RunHandle struct {
	Cancel func()
}

func handleStartRun(w http.ResponseWriter, r *http.Request) {
	var config PipelineConfig
	json.NewDecoder(r.Body).Decode(&config)
	runID := generateID()
	orch := NewOrchestrator(config, runID)
	ctx, cancel := contextWithCancel()
	activeRuns[runID] = &RunHandle{Cancel: cancel}
	go func() {
		orch.Run(ctx)
		delete(activeRuns, runID)
	}()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"run_id": runID})
}

func handleRunStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	path := filepath.Join(configDir(), "runs", id, "status.json")
	data, err := os.ReadFile(path)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{}"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func handleStopRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if rh, ok := activeRuns[id]; ok {
		rh.Cancel()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func handleNodeLog(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	nodeId := r.PathValue("nodeId")
	path := filepath.Join(configDir(), "runs", id, "logs", nodeId+".log")
	logContent, err := os.ReadFile(path)
	if err != nil {
		logContent = []byte("No log yet.")
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"log": string(logContent)})
}
