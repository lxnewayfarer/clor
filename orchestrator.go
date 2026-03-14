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
	config      PipelineConfig
	runID       string
	runDir      string
	logsDir     string
	sharedDir   string
	projects    map[string]Project
	nodesMap    map[string]NodeConfig
	statuses    map[string]NodeStatus
	answerChs   map[string]chan []Question
	logStreams  map[string]*LogStream
	subscribers []chan map[string]NodeStatus
	mu          sync.Mutex
	headless    bool
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
		answerChs:  make(map[string]chan []Question),
		logStreams: make(map[string]*LogStream),
	}
}

func (o *Orchestrator) Run(ctx context.Context) {
	os.MkdirAll(o.logsDir, 0755)
	os.MkdirAll(o.sharedDir, 0755)

	for _, n := range o.config.Nodes {
		o.setStatus(n.ID, "idle", "")
	}

	waves, err := ComputeWaves(o.config.Nodes, o.config.Edges)
	if err != nil {
		// Mark all nodes as error on cycle
		for _, n := range o.config.Nodes {
			o.setStatus(n.ID, "error", err.Error())
		}
		return
	}

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
			break
		}
	}

	o.cleanupTempFiles()
}

func (o *Orchestrator) executeNode(ctx context.Context, nodeID string) {
	node := o.nodesMap[nodeID]

	if node.Type == "task" {
		o.setStatus(nodeID, "done", "Task provided")
		return
	}

	if isCommitterNode(node) {
		o.cleanupTempFiles()
	}

	proj, ok := o.projects[node.Config.ProjectID]
	workdir := "."
	if ok {
		workdir = proj.Path
	}

	// Create log stream for live output
	logStream := NewLogStream()
	o.mu.Lock()
	o.logStreams[nodeID] = logStream
	o.mu.Unlock()
	o.watchLogStream(nodeID, logStream)

	o.setStatus(nodeID, "running", "Working...")
	o.startHeartbeat(ctx, nodeID)

	prompt := o.expandPromptForNode(nodeID, node.Config.Prompt, workdir)
	model := node.Config.Model
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

	maxRetries := node.Config.MaxRetries
	retryDelay := node.Config.RetryDelaySeconds
	if retryDelay <= 0 {
		retryDelay = 5
	}

	// Decompose mode: run subtasks from upstream architect output
	if node.Config.Decompose {
		o.executeDecomposed(ctx, nodeID, node, prompt, workdir, model, logStream)
		return
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: delay * 2^(attempt-1)
			delay := time.Duration(retryDelay) * time.Second * (1 << (attempt - 1))
			o.setStatus(nodeID, "running", fmt.Sprintf("Retry %d/%d in %s...", attempt, maxRetries, delay))
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				o.setStatus(nodeID, "error", "Cancelled during retry wait")
				return
			}
			// Reset log stream for retry
			logStream = NewLogStream()
			o.mu.Lock()
			o.logStreams[nodeID] = logStream
			ns := o.statuses[nodeID]
			ns.RetryAttempt = attempt
			o.statuses[nodeID] = ns
			o.mu.Unlock()
			o.watchLogStream(nodeID, logStream)
			o.setStatus(nodeID, "running", fmt.Sprintf("Retry %d/%d running...", attempt, maxRetries))
		}

		success := o.runAgentWithInteractive(ctx, nodeID, node, prompt, workdir, model, maxRounds, logStream)
		if success {
			lastErr = nil
			break
		}

		// Check if node ended in error (retryable) vs cancelled
		o.mu.Lock()
		ns := o.statuses[nodeID]
		o.mu.Unlock()
		if ns.Status == "error" {
			lastErr = fmt.Errorf("%s", ns.Message)
			if attempt < maxRetries {
				continue
			}
		}
		return // non-retryable or max retries exhausted
	}

	if lastErr != nil {
		o.setStatus(nodeID, "error", fmt.Sprintf("Failed after %d retries: %s", maxRetries, lastErr))
		return
	}

	for _, art := range node.Config.OutputArtifacts {
		deliverFile(workdir, art.File, art.DeliverTo, o.projects, o.sharedDir)
	}

	// Clean up log stream
	o.mu.Lock()
	if ls, ok := o.logStreams[nodeID]; ok {
		ls.Close()
		delete(o.logStreams, nodeID)
	}
	o.mu.Unlock()

	// Review loop: auto-detect reviewer by label, target is upstream node
	if isReviewerNode(node) {
		o.handleReviewLoop(ctx, nodeID, node)
		return
	}

	o.setStatus(nodeID, "done", "Complete")
}

// getUpstreamOutput reads the log of the first upstream node (by edges).
func (o *Orchestrator) getUpstreamOutput(nodeID string) string {
	for _, e := range o.config.Edges {
		if e.Target == nodeID {
			logPath := filepath.Join(o.logsDir, e.Source+".log")
			data, err := os.ReadFile(logPath)
			if err == nil {
				return string(data)
			}
		}
	}
	return ""
}

// executeDecomposed reads the upstream architect's output, parses subtasks,
// and runs them in parallel.
func (o *Orchestrator) executeDecomposed(ctx context.Context, nodeID string, node NodeConfig, prompt, workdir, model string, logStream *LogStream) {
	upstreamOutput := o.getUpstreamOutput(nodeID)
	subtasks := ParseSubtasks(upstreamOutput)

	// If no subtasks in upstream log, try plan.md in workdir
	if len(subtasks) == 0 {
		planData, err := os.ReadFile(filepath.Join(workdir, "plan.md"))
		if err == nil {
			subtasks = ParseSubtasks(string(planData))
		}
	}

	if len(subtasks) == 0 {
		// Fallback: no subtasks found, run as normal agent
		maxRounds := node.Config.MaxQuestionRounds
		if maxRounds <= 0 {
			maxRounds = 3
		}
		success := o.runAgentWithInteractive(ctx, nodeID, node, prompt, workdir, model, maxRounds, logStream)
		if success {
			for _, art := range node.Config.OutputArtifacts {
				deliverFile(workdir, art.File, art.DeliverTo, o.projects, o.sharedDir)
			}
			o.mu.Lock()
			if ls, ok := o.logStreams[nodeID]; ok {
				ls.Close()
				delete(o.logStreams, nodeID)
			}
			o.mu.Unlock()
			o.setStatus(nodeID, "done", "Complete (no subtasks found)")
		}
		return
	}

	total := len(subtasks)

	// Initialize subtask statuses
	stStatuses := make([]SubtaskStatus, total)
	for i, st := range subtasks {
		stStatuses[i] = SubtaskStatus{Index: i + 1, Label: st, Status: "pending"}
	}
	o.mu.Lock()
	ns := o.statuses[nodeID]
	ns.SubtaskTotal = total
	ns.Subtasks = stStatuses
	o.statuses[nodeID] = ns
	o.mu.Unlock()
	o.updateDecomposeMessage(nodeID)

	// Results storage
	type subtaskResult struct {
		index  int
		result string
		err    error
	}
	results := make([]subtaskResult, total)

	// Run all subtasks in parallel
	var wg sync.WaitGroup
	for i, subtask := range subtasks {
		wg.Add(1)
		go func(idx int, st string) {
			defer wg.Done()

			if ctx.Err() != nil {
				results[idx] = subtaskResult{index: idx, err: fmt.Errorf("cancelled")}
				return
			}

			// Mark running
			o.mu.Lock()
			ns := o.statuses[nodeID]
			ns.Subtasks[idx].Status = "running"
			ns.SubtaskIndex = o.countSubtasksByStatus(ns.Subtasks, "done") + 1
			o.statuses[nodeID] = ns
			o.mu.Unlock()
			o.updateDecomposeMessage(nodeID)

			subtaskLogStream := NewLogStream()
			// Store per-subtask log stream with a unique key
			stKey := fmt.Sprintf("%s.st%d", nodeID, idx+1)
			o.mu.Lock()
			o.logStreams[stKey] = subtaskLogStream
			o.mu.Unlock()
			o.watchLogStream(nodeID, subtaskLogStream)

			subtaskPrompt := expandSubtask(prompt, st, idx+1, total)
			result, err := runAgent(ctx, subtaskPrompt, node.Config.AllowedTools,
				workdir, model, o.config.Settings.TimeoutSeconds, subtaskLogStream)

			// Close this subtask's log stream
			o.mu.Lock()
			if ls, ok := o.logStreams[stKey]; ok {
				ls.Close()
				delete(o.logStreams, stKey)
			}
			o.mu.Unlock()

			if err != nil {
				o.mu.Lock()
				ns := o.statuses[nodeID]
				ns.Subtasks[idx].Status = "error"
				o.statuses[nodeID] = ns
				o.mu.Unlock()
				o.updateDecomposeMessage(nodeID)
				results[idx] = subtaskResult{index: idx, err: err}
				return
			}

			// Mark done
			o.mu.Lock()
			ns = o.statuses[nodeID]
			ns.Subtasks[idx].Status = "done"
			ns.SubtaskIndex = o.countSubtasksByStatus(ns.Subtasks, "done")
			o.statuses[nodeID] = ns
			o.mu.Unlock()
			o.updateDecomposeMessage(nodeID)

			results[idx] = subtaskResult{index: idx, result: result}

			// Write individual subtask log
			os.WriteFile(filepath.Join(o.logsDir, fmt.Sprintf("%s.st%d.log", nodeID, idx+1)),
				[]byte(result), 0644)
		}(i, subtask)
	}
	wg.Wait()

	// Build cumulative log and check for errors
	var cumulativeLog strings.Builder
	var failed []int
	for i, r := range results {
		cumulativeLog.WriteString(fmt.Sprintf("\n\n=== Subtask %d/%d: %s ===\n\n", i+1, total, subtasks[i]))
		if r.err != nil {
			cumulativeLog.WriteString(fmt.Sprintf("ERROR: %s\n", r.err))
			failed = append(failed, i+1)
		} else {
			cumulativeLog.WriteString(r.result)
		}
	}
	os.WriteFile(filepath.Join(o.logsDir, nodeID+".log"), []byte(cumulativeLog.String()), 0644)

	if len(failed) > 0 {
		o.setStatus(nodeID, "error", fmt.Sprintf("%d/%d subtasks failed", len(failed), total))
		return
	}

	// Deliver artifacts
	for _, art := range node.Config.OutputArtifacts {
		deliverFile(workdir, art.File, art.DeliverTo, o.projects, o.sharedDir)
	}

	// Clean up
	o.mu.Lock()
	if ls, ok := o.logStreams[nodeID]; ok {
		ls.Close()
		delete(o.logStreams, nodeID)
	}
	ns = o.statuses[nodeID]
	ns.SubtaskIndex = total
	ns.SubtaskTotal = total
	ns.SubtaskLabel = "All complete"
	o.statuses[nodeID] = ns
	o.mu.Unlock()

	o.setStatus(nodeID, "done", fmt.Sprintf("Complete (%d subtasks)", total))
}

// updateDecomposeMessage updates the node message based on subtask statuses.
func (o *Orchestrator) updateDecomposeMessage(nodeID string) {
	o.mu.Lock()
	ns := o.statuses[nodeID]
	running := o.countSubtasksByStatus(ns.Subtasks, "running")
	done := o.countSubtasksByStatus(ns.Subtasks, "done")
	errored := o.countSubtasksByStatus(ns.Subtasks, "error")
	total := len(ns.Subtasks)
	ns.Message = fmt.Sprintf("%d running · %d done · %d error — %d total", running, done, errored, total)
	ns.Status = "running"
	o.statuses[nodeID] = ns
	data, _ := json.MarshalIndent(o.statuses, "", "  ")
	os.WriteFile(filepath.Join(o.runDir, "status.json"), data, 0644)
	o.broadcast()
	o.mu.Unlock()
}

// countSubtasksByStatus counts subtasks with a given status.
func (o *Orchestrator) countSubtasksByStatus(subtasks []SubtaskStatus, status string) int {
	count := 0
	for _, s := range subtasks {
		if s.Status == status {
			count++
		}
	}
	return count
}

// isReviewerNode checks if a node is a reviewer by its label.
func isReviewerNode(n NodeConfig) bool {
	label := strings.ToLower(n.Label)
	return strings.Contains(label, "review")
}

// isCommitterNode checks if a node is a committer by its label.
func isCommitterNode(n NodeConfig) bool {
	label := strings.ToLower(n.Label)
	return strings.Contains(label, "commit")
}

// getUpstreamNodeID returns the first upstream node ID for a given node.
func (o *Orchestrator) getUpstreamNodeID(nodeID string) string {
	for _, e := range o.config.Edges {
		if e.Target == nodeID {
			return e.Source
		}
	}
	return ""
}

// handleReviewLoop implements the review cycle: reviewer checks target node's output,
// if FAIL — writes issues to target's review.md, re-runs target, then re-runs reviewer.
func (o *Orchestrator) handleReviewLoop(ctx context.Context, reviewerID string, reviewer NodeConfig) {
	targetID := o.getUpstreamNodeID(reviewerID)
	if targetID == "" {
		o.setStatus(reviewerID, "error", "reviewer has no upstream node to review")
		return
	}
	targetNode, ok := o.nodesMap[targetID]
	if !ok {
		o.setStatus(reviewerID, "error", "reviewer target node not found: "+targetID)
		return
	}

	maxRetries := reviewer.Config.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	for round := 1; round <= maxRetries; round++ {
		// Read reviewer's output
		logPath := filepath.Join(o.logsDir, reviewerID+".log")
		logData, err := os.ReadFile(logPath)
		if err != nil {
			o.setStatus(reviewerID, "error", "cannot read reviewer log: "+err.Error())
			return
		}

		result := ParseReviewOutput(string(logData))
		if result.Pass {
			o.setStatus(reviewerID, "done", fmt.Sprintf("Review PASS (round %d)", round))
			return
		}

		// FAIL: write issues to target project's review.md
		targetProj, ok := o.projects[targetNode.Config.ProjectID]
		if !ok {
			o.setStatus(reviewerID, "error", "target node has no project")
			return
		}

		issuesMD := FormatIssuesMarkdown(result.Issues, round, maxRetries)
		os.WriteFile(filepath.Join(targetProj.Path, "review.md"), []byte(issuesMD), 0644)

		o.setStatus(reviewerID, "running", fmt.Sprintf("Review FAIL — re-running %s (round %d/%d)", targetNode.Label, round, maxRetries))

		// Update target's review round in status
		o.mu.Lock()
		ns := o.statuses[targetID]
		ns.ReviewRound = round
		o.statuses[targetID] = ns
		o.mu.Unlock()

		// Re-run target node
		o.executeNode(ctx, targetID)

		if ctx.Err() != nil {
			return
		}

		// Check if target failed
		o.mu.Lock()
		targetStatus := o.statuses[targetID].Status
		o.mu.Unlock()
		if targetStatus == "error" {
			o.setStatus(reviewerID, "error", "target node failed during review round "+itoa(round))
			return
		}

		// Re-run reviewer itself
		o.setStatus(reviewerID, "running", fmt.Sprintf("Re-reviewing (round %d/%d)...", round+1, maxRetries))

		reviewerProj, ok := o.projects[reviewer.Config.ProjectID]
		reviewWorkdir := "."
		if ok {
			reviewWorkdir = reviewerProj.Path
		}

		logStream := NewLogStream()
		o.mu.Lock()
		o.logStreams[reviewerID] = logStream
		o.mu.Unlock()
		o.watchLogStream(reviewerID, logStream)

		prompt := o.expandPromptForNode(reviewerID, reviewer.Config.Prompt, reviewWorkdir)
		model := reviewer.Config.Model
		if model == "" {
			model = o.config.Settings.Model
		}

		result2, err := runAgent(ctx, prompt, reviewer.Config.AllowedTools,
			reviewWorkdir, model, o.config.Settings.TimeoutSeconds, logStream)
		if err != nil {
			o.setStatus(reviewerID, "error", err.Error())
			return
		}
		os.WriteFile(filepath.Join(o.logsDir, reviewerID+".log"), []byte(result2), 0644)

		o.mu.Lock()
		if ls, ok := o.logStreams[reviewerID]; ok {
			ls.Close()
			delete(o.logStreams, reviewerID)
		}
		o.mu.Unlock()
	}

	// Exhausted all review rounds
	o.setStatus(reviewerID, "done", fmt.Sprintf("Review completed after %d rounds (last was FAIL)", maxRetries))
}

// runAgentWithInteractive runs the agent with interactive question support.
// Returns true on success, false on error.
func (o *Orchestrator) runAgentWithInteractive(ctx context.Context, nodeID string, node NodeConfig, prompt, workdir, model string, maxRounds int, logStream *LogStream) bool {
	currentPrompt := prompt
	for round := 0; round <= maxRounds; round++ {
		result, err := runAgent(ctx, currentPrompt, node.Config.AllowedTools,
			workdir, model, o.config.Settings.TimeoutSeconds, logStream)

		if err != nil {
			o.setStatus(nodeID, "error", err.Error())
			os.WriteFile(filepath.Join(o.logsDir, nodeID+".error.log"),
				[]byte(err.Error()), 0644)
			return false
		}

		os.WriteFile(filepath.Join(o.logsDir, nodeID+".log"), []byte(result), 0644)

		// Check for interactive questions
		if !node.Config.Interactive {
			return true
		}

		questions := parseQuestions(result)
		if questions == nil {
			return true
		}

		if round == maxRounds {
			return true
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
		o.broadcast()
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

		select {
		case answered := <-ch:
			currentPrompt = prompt +
				"\n\n## Previous output\n" + result +
				"\n\n## Answers to your questions\n" + formatAnswers(answered) +
				"\n\nNow continue with your task using these answers. Do not ask the same questions again."
			o.setStatus(nodeID, "running", fmt.Sprintf("Continuing with answers (round %d/%d)", round+1, maxRounds))
		case <-ctx.Done():
			o.setStatus(nodeID, "error", "Cancelled while waiting for answers")
			return false
		}
	}
	return true
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

func (o *Orchestrator) cleanupTempFiles() {
	for _, node := range o.config.Nodes {
		if len(node.Config.TempFiles) == 0 {
			continue
		}
		proj, ok := o.projects[node.Config.ProjectID]
		if !ok {
			continue
		}
		for _, f := range node.Config.TempFiles {
			path := filepath.Join(proj.Path, f)
			os.Remove(path)
		}
	}
}

func (o *Orchestrator) expandPrompt(prompt string, workdir string) string {
	return o.expandPromptForNode("", prompt, workdir)
}

func (o *Orchestrator) expandPromptForNode(nodeID, prompt, workdir string) string {
	var taskText string
	if nodeID != "" {
		// Collect texts from all upstream task nodes
		var parts []string
		for _, e := range o.config.Edges {
			if e.Target == nodeID {
				src := o.nodesMap[e.Source]
				if src.Type == "task" {
					t := src.Config.Text
					if t == "" {
						t = src.Config.Description
					}
					if t != "" {
						parts = append(parts, t)
					}
				}
			}
		}
		if len(parts) > 0 {
			taskText = strings.Join(parts, "\n\n")
		}
	}
	// Fallback: first task node in pipeline (backward compat)
	if taskText == "" {
		taskNode := findByType(o.config.Nodes, "task")
		if taskNode != nil {
			taskText = taskNode.Config.Text
			if taskText == "" {
				taskText = taskNode.Config.Description
			}
		}
	}
	prompt = strings.ReplaceAll(prompt, "{task}", taskText)
	prompt = expandReadVars(prompt, workdir, o.projects)
	prompt = expandFilesVars(prompt, workdir)
	prompt = expandReviewIssues(prompt, workdir)
	prompt = expandVars(prompt, o.config.Variables, o.config.VarValues)
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

	o.broadcast()

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

// RetryNode manually retries a failed node.
func (o *Orchestrator) RetryNode(nodeID string) {
	o.mu.Lock()
	ns, exists := o.statuses[nodeID]
	o.mu.Unlock()
	if !exists || ns.Status != "error" {
		return
	}
	ctx := context.Background()
	o.executeNode(ctx, nodeID)
}

// GetLogStream returns the live log stream for a node, or nil if not running.
func (o *Orchestrator) GetLogStream(nodeID string) *LogStream {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.logStreams[nodeID]
}

// Subscribe returns a channel that receives status updates.
func (o *Orchestrator) Subscribe() chan map[string]NodeStatus {
	ch := make(chan map[string]NodeStatus, 16)
	o.mu.Lock()
	o.subscribers = append(o.subscribers, ch)
	o.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel.
func (o *Orchestrator) Unsubscribe(ch chan map[string]NodeStatus) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for i, sub := range o.subscribers {
		if sub == ch {
			o.subscribers = append(o.subscribers[:i], o.subscribers[i+1:]...)
			close(ch)
			return
		}
	}
}

func (o *Orchestrator) broadcast() {
	snapshot := make(map[string]NodeStatus, len(o.statuses))
	for k, v := range o.statuses {
		snapshot[k] = v
	}
	for _, ch := range o.subscribers {
		select {
		case ch <- snapshot:
		default:
			// slow consumer, skip
		}
	}
}

func contextWithCancel() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

// watchLogStream starts a goroutine that reads chunks from ls and updates
// NodeStatus.Message with the last non-empty output line for nodeID.
// The goroutine exits when ls is closed.
func (o *Orchestrator) watchLogStream(nodeID string, ls *LogStream) {
	go func() {
		ch := ls.Subscribe()
		for chunk := range ch {
			line := lastNonEmptyLine(chunk)
			if line == "" {
				continue
			}
			o.mu.Lock()
			ns := o.statuses[nodeID]
			if ns.Status == "running" {
				ns.Message = line
				o.statuses[nodeID] = ns
				o.broadcast()
			}
			o.mu.Unlock()
		}
	}()
}

// startHeartbeat starts a goroutine that broadcasts elapsed time updates every 2s
// while nodeID is in "running" state. It stops when ctx is done or the node
// leaves the "running" state.
func (o *Orchestrator) startHeartbeat(ctx context.Context, nodeID string) {
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				o.mu.Lock()
				ns, ok := o.statuses[nodeID]
				if !ok || ns.Status != "running" {
					o.mu.Unlock()
					return
				}
				if ns.StartedAt > 0 {
					ns.Elapsed = int(float64(time.Now().Unix()) - ns.StartedAt)
					o.statuses[nodeID] = ns
				}
				o.broadcast()
				o.mu.Unlock()
			case <-ctx.Done():
				return
			}
		}
	}()
}

// lastNonEmptyLine returns the last non-empty line from s.
func lastNonEmptyLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
