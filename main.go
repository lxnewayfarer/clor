package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"time"
)

//go:embed web/index.html
var content embed.FS

//go:embed prompts/*.md
var promptsFS embed.FS

const version = "0.1.0"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version":
			fmt.Printf("clor %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
			return
		case "run":
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "usage: clor run <pipeline.json>")
				os.Exit(1)
			}
			runHeadless(os.Args[2])
			return
		}
	}

	port := flag.Int("p", 9980, "port")
	noBrowser := flag.Bool("no-browser", false, "don't open browser")
	flag.Parse()

	ensureConfigDir()
	cleanOldRuns()
	mux := setupRoutes()

	addr := fmt.Sprintf(":%d", *port)
	url := fmt.Sprintf("http://localhost:%d", *port)
	fmt.Printf("clor %s → %s\n", version, url)

	if !*noBrowser {
		go openBrowser(url)
	}

	log.Fatal(http.ListenAndServe(addr, mux))
}

func openBrowser(url string) {
	exec.Command("xdg-open", url).Start()
}

func ensureConfigDir() {
	home, _ := os.UserHomeDir()
	dirs := []string{
		home + "/.config/clor",
		home + "/.config/clor/pipelines",
		home + "/.config/clor/runs",
	}
	for _, d := range dirs {
		os.MkdirAll(d, 0755)
	}
}

func runHeadless(configPath string) {
	fmt.Printf("clor v%s — headless run\n", version)

	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading %s: %s\n", configPath, err)
		os.Exit(1)
	}

	var config PipelineConfig
	if err := json.Unmarshal(data, &config); err != nil {
		fmt.Fprintf(os.Stderr, "error parsing pipeline JSON: %s\n", err)
		os.Exit(1)
	}

	ensureConfigDir()

	// Count unique projects
	projSet := make(map[string]bool)
	for _, n := range config.Nodes {
		if n.Config.ProjectID != "" {
			projSet[n.Config.ProjectID] = true
		}
	}

	fmt.Printf("Pipeline: %s (%d nodes, %d edges)\n", config.Name, len(config.Nodes), len(config.Edges))

	pf := loadProjects()
	pm := make(map[string]Project)
	for _, p := range pf.Projects {
		pm[p.ID] = p
	}
	if len(projSet) > 0 {
		var names []string
		for pid := range projSet {
			if p, ok := pm[pid]; ok {
				names = append(names, fmt.Sprintf("%s (%s)", p.Alias, p.Path))
			}
		}
		fmt.Printf("Projects: %s\n", strings.Join(names, ", "))
	}
	fmt.Println()

	runID := generateID()
	orch := NewOrchestrator(config, runID)

	// Hook into status changes for printing
	orch.headless = true

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		fmt.Println("\ninterrupted — stopping agents...")
		cancel()
	}()

	start := time.Now()
	orch.Run(ctx)
	elapsed := time.Since(start)

	fmt.Printf("\nDone in %s. Logs: %s\n", formatDuration(elapsed), orch.logsDir)
}

func cleanOldRuns() {
	runsDir := configDir() + "/runs"
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.RemoveAll(runsDir + "/" + e.Name())
		}
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%ds", m, s)
}
