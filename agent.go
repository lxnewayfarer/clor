package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// LogStream captures agent output and allows live streaming to subscribers.
type LogStream struct {
	mu          sync.Mutex
	buf         bytes.Buffer
	subscribers []chan string
}

func NewLogStream() *LogStream {
	return &LogStream{}
}

func (ls *LogStream) Write(p []byte) (int, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	n, err := ls.buf.Write(p)
	chunk := string(p[:n])
	for _, ch := range ls.subscribers {
		select {
		case ch <- chunk:
		default:
		}
	}
	return n, err
}

func (ls *LogStream) String() string {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.buf.String()
}

func (ls *LogStream) Subscribe() chan string {
	ch := make(chan string, 64)
	ls.mu.Lock()
	// Send existing content as first chunk
	if ls.buf.Len() > 0 {
		ch <- ls.buf.String()
	}
	ls.subscribers = append(ls.subscribers, ch)
	ls.mu.Unlock()
	return ch
}

func (ls *LogStream) Unsubscribe(ch chan string) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	for i, sub := range ls.subscribers {
		if sub == ch {
			ls.subscribers = append(ls.subscribers[:i], ls.subscribers[i+1:]...)
			close(ch)
			return
		}
	}
}

// Close closes all subscriber channels, signaling watchers to stop.
func (ls *LogStream) Close() {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	for _, ch := range ls.subscribers {
		close(ch)
	}
	ls.subscribers = nil
}

func runAgent(ctx context.Context, prompt string, tools []string, workdir string, model string, timeout int, logStream *LogStream) (string, error) {
	if timeout <= 0 {
		timeout = 600
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	args := []string{"-p", "--output-format", "text"}
	if model != "" {
		args = append(args, "--model", model)
	}
	if len(tools) > 0 {
		args = append(args, "--allowedTools", strings.Join(tools, ","))
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = workdir
	cmd.Stdin = bytes.NewBufferString(prompt)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Stream stdout to both capture and logStream
	var stdout bytes.Buffer
	if logStream != nil {
		cmd.Stdout = io.MultiWriter(&stdout, logStream)
	} else {
		cmd.Stdout = &stdout
	}

	err := cmd.Run()
	if err != nil {
		errMsg := fmt.Sprintf("agent error: %s (stderr: %s)", err, stderr.String())
		if logStream != nil {
			logStream.Write([]byte("\n\n--- ERROR ---\n" + errMsg))
		}
		return "", fmt.Errorf("%s", errMsg)
	}

	// Write final log file
	if logStream != nil {
		// Ensure log file has full output
		os.WriteFile(workdir+"/.clor-last-output", stdout.Bytes(), 0644)
	}

	return strings.TrimSpace(stdout.String()), nil
}
