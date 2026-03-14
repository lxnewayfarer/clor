package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Orchestrator struct {
	config    PipelineConfig
	runID     string
	runDir    string
	logsDir   string
	sharedDir string
	projects  map[string]Project
	nodesMap  map[string]NodeConfig
	statuses  map[string]NodeStatus
	answerChs map[string]chan []Question
	mu        sync.Mutex
	headless  bool
}

func NewOrchestrator(config PipelineConfig, runID string) *Orchestrator {
	runDir := filepath.Join(configDir(), "runs", runID)
	pf := loadProjects()
	pm := make(map[string]Project)
	for _, p := range pf.Projects {
		pm[p.ID] = p
	}
	nm := make(map[string]NodeConfig)
	for _, n := range config.Nodes {
		nm[n.ID] = n
	}
	return &Orchestrator{
		config:    config,
		runID:     runID,
		runDir:    runDir,
		logsDir:   filepath.Join(runDir, "logs"),
		sharedDir: filepath.Join(runDir, "shared"),
		projects:  pm,
		nodesMap:  nm,
		statuses:  make(map[string]NodeStatus),
		answerChs: make(map[string]chan []Question),
	}
}

func (o *Orchestrator) Run(ctx context.Context) {
	os.MkdirAll(o.logsDir, 0755)
	os.MkdirAll(o.sharedDir, 0755)

	for _, n := range o.config.Nodes {
		o.setStatus(n.ID, "idle", "")
	}

	waves := ComputeWaves(o.config.Nodes, o.config.Edges)

	for wi, wave := range waves {
		if o.headless {
			labels := make([]string, len(wave))
			for i, nid := range wave {
				labels[i] = o.nodesMap[nid].Label
				if labels[i] == "" {
					labels[i] = nid
				}
			}
			fmt.Printf("[%s] Wave %d: %s → running...\n",
				time.Now().Format("15:04:05"), wi+1, strings.Join(labels, ", "))
		}

		for _, nid := range wave {
			o.setStatus(nid, "queued", "Waiting...")
		}

		var wg sync.WaitGroup
		for _, nid := range wave {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				o.executeNode(ctx, id)
			}(nid)
		}
		wg.Wait()

		if ctx.Err() != nil {
			return
		}

		for _, nid := range wave {
			node := o.nodesMap[nid]
			if node.Config.ReviewMode != nil && node.Config.ReviewMode.Enabled {
				o.reviewLoop(ctx, node)
			}
		}
	}
}

func (o *Orchestrator) executeNode(ctx context.Context, nodeID string) {
	node := o.nodesMap[nodeID]

	if node.Type == "task" {
		o.setStatus(nodeID, "done", "Task provided")
		return
	}

	proj, ok := o.projects[node.Config.ProjectID]
	workdir := "."
	if ok {
		workdir = proj.Path
	}

	o.setStatus(nodeID, "running", "Working...")

	prompt := o.expandPrompt(node.Config.Prompt, workdir)
	model := node.Config.ModelOverride
	if model == "" {
		model = o.config.Settings.Model
	}

	maxRounds := node.Config.MaxQuestionRounds
	if maxRounds <= 0 {
		maxRounds = 3
	}

	// Create answer channel for interactive nodes
	if node.Config.Interactive {
		o.mu.Lock()
		o.answerChs[nodeID] = make(chan []Question, 1)
		o.mu.Unlock()
	}

	currentPrompt := prompt
	for round := 0; round <= maxRounds; round++ {
		result, err := runAgent(ctx, currentPrompt, node.Config.AllowedTools,
			workdir, model, o.config.Settings.TimeoutSeconds)

		if err != nil {
			o.setStatus(nodeID, "error", err.Error())
			os.WriteFile(filepath.Join(o.logsDir, nodeID+".error.log"),
				[]byte(err.Error()), 0644)
			return
		}

		os.WriteFile(filepath.Join(o.logsDir, nodeID+".log"), []byte(result), 0644)

		// Check for interactive questions
		if !node.Config.Interactive {
			break
		}

		questions := parseQuestions(result)
		if questions == nil {
			break
		}

		if round == maxRounds {
			// Max rounds reached, finish anyway
			break
		}

		// Set waiting_for_input status with questions
		o.mu.Lock()
		ns := o.statuses[nodeID]
		ns.Status = "waiting_for_input"
		ns.Message = fmt.Sprintf("Has %d question(s) (round %d/%d)", len(questions), round+1, maxRounds)
		ns.Questions = questions
		ns.QuestionRound = round + 1
		o.statuses[nodeID] = ns
		data, _ := json.MarshalIndent(o.statuses, "", "  ")
		os.WriteFile(filepath.Join(o.runDir, "status.json"), data, 0644)
		ch := o.answerChs[nodeID]
		o.mu.Unlock()

		if o.headless {
			label := node.Label
			if label == "" {
				label = nodeID
			}
			fmt.Printf("[%s] %s waiting for answers (round %d/%d, %d questions)\n",
				time.Now().Format("15:04:05"), label, round+1, maxRounds, len(questions))
		}

		// Block until answers arrive or context is cancelled
		select {
		case answered := <-ch:
			// Build augmented prompt with answers
			currentPrompt = prompt +
				"\n\n## Previous output\n" + result +
				"\n\n## Answers to your questions\n" + formatAnswers(answered) +
				"\n\nNow continue with your task using these answers. Do not ask the same questions again."
			o.setStatus(nodeID, "running", fmt.Sprintf("Continuing with answers (round %d/%d)", round+1, maxRounds))
		case <-ctx.Done():
			o.setStatus(nodeID, "error", "Cancelled while waiting for answers")
			return
		}
	}

	for _, art := range node.Config.OutputArtifacts {
		deliverFile(workdir, art.File, art.DeliverTo, o.projects, o.sharedDir)
	}

	o.setStatus(nodeID, "done", "Complete")
}

// SubmitAnswer delivers user answers to a waiting interactive node.
func (o *Orchestrator) SubmitAnswer(nodeID string, answers []Question) bool {
	o.mu.Lock()
	ch, ok := o.answerChs[nodeID]
	o.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case ch <- answers:
		return true
	default:
		return false
	}
}

func (o *Orchestrator) reviewLoop(ctx context.Context, reviewer NodeConfig) {
	rm := reviewer.Config.ReviewMode
	maxRetries := o.config.Settings.MaxReviewRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	proj := o.projects[reviewer.Config.ProjectID]
	reviewPath := filepath.Join(proj.Path, "review.md")

	for round := 1; round <= maxRetries; round++ {
		if ctx.Err() != nil {
			return
		}

		review, err := parseReview(reviewPath)
		if err != nil || review.Passed {
			return
		}

		o.setStatus(reviewer.ID, "running",
			fmt.Sprintf("Fix round %d/%d", round, maxRetries))

		var wg sync.WaitGroup
		for section, targetID := range rm.FixTargets {
			issues := review.Issues[section]
			if len(issues) == 0 {
				continue
			}
			wg.Add(1)
			go func(tid string, iss []string) {
				defer wg.Done()
				target := o.nodesMap[tid]
				tproj := o.projects[target.Config.ProjectID]

				reviewContent, _ := os.ReadFile(reviewPath)
				fixPrompt := fmt.Sprintf(
					"Fix these issues found in code review:\n\n%s\n\nFull review:\n%s",
					strings.Join(iss, "\n"), string(reviewContent))

				o.setStatus(tid, "running",
					fmt.Sprintf("Fixing (round %d)", round))

				model := target.Config.ModelOverride
				if model == "" {
					model = o.config.Settings.Model
				}
				runAgent(ctx, fixPrompt, target.Config.AllowedTools,
					tproj.Path, model, o.config.Settings.TimeoutSeconds)
				o.setStatus(tid, "done", "Fixed")
			}(targetID, issues)
		}
		wg.Wait()

		o.executeNode(ctx, reviewer.ID)
	}
}

func (o *Orchestrator) expandPrompt(prompt string, workdir string) string {
	taskNode := findByType(o.config.Nodes, "task")
	if taskNode != nil {
		prompt = strings.ReplaceAll(prompt, "{task}", taskNode.Config.Description)
	}
	prompt = expandReadVars(prompt, workdir, o.projects)
	prompt = expandFilesVars(prompt, workdir)
	prompt = expandReviewIssues(prompt, workdir)
	return prompt
}

func (o *Orchestrator) setStatus(nodeID, status, message string) {
	o.mu.Lock()
	defer o.mu.Unlock()

	ns := o.statuses[nodeID]
	ns.Status = status
	ns.Message = message
	if status == "running" && ns.StartedAt == 0 {
		ns.StartedAt = float64(time.Now().Unix())
	}
	if ns.StartedAt > 0 {
		ns.Elapsed = int(float64(time.Now().Unix()) - ns.StartedAt)
	}
	o.statuses[nodeID] = ns

	data, _ := json.MarshalIndent(o.statuses, "", "  ")
	os.WriteFile(filepath.Join(o.runDir, "status.json"), data, 0644)

	if o.headless && (status == "done" || status == "error") {
		label := o.nodesMap[nodeID].Label
		if label == "" {
			label = nodeID
		}
		icon := "✓"
		if status == "error" {
			icon = "✗"
		}
		elapsed := ""
		if ns.Elapsed > 0 {
			elapsed = fmt.Sprintf(" (%ds)", ns.Elapsed)
		}
		extra := ""
		if message != "" && message != "Complete" && message != "Task provided" {
			extra = " — " + message
		}
		fmt.Printf("[%s] %s %s%s%s\n", time.Now().Format("15:04:05"), label, icon, elapsed, extra)
	}
}

func contextWithCancel() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
