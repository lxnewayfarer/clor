package main

import (
	"fmt"
	"strings"
)

// FilterDownstream returns the subgraph reachable from startID (inclusive).
// Upstream task nodes are included since they are needed for prompt expansion.
func FilterDownstream(nodes []NodeConfig, edges []Edge, startID string) ([]NodeConfig, []Edge) {
	children := make(map[string][]string)
	for _, e := range edges {
		children[e.Source] = append(children[e.Source], e.Target)
	}

	reachable := make(map[string]bool)
	queue := []string{startID}
	reachable[startID] = true
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, ch := range children[cur] {
			if !reachable[ch] {
				reachable[ch] = true
				queue = append(queue, ch)
			}
		}
	}

	// Include all task nodes (they resolve instantly and provide {task} data)
	for _, n := range nodes {
		if n.Type == "task" {
			reachable[n.ID] = true
		}
	}

	var filteredNodes []NodeConfig
	for _, n := range nodes {
		if reachable[n.ID] {
			filteredNodes = append(filteredNodes, n)
		}
	}
	var filteredEdges []Edge
	for _, e := range edges {
		if reachable[e.Source] && reachable[e.Target] {
			filteredEdges = append(filteredEdges, e)
		}
	}
	return filteredNodes, filteredEdges
}

// FilterReviewBackEdges removes back-edges from reviewer nodes to their upstream targets.
// Review loops are handled programmatically by the orchestrator, so these edges
// must be excluded from topological sort to avoid false cycle detection.
func FilterReviewBackEdges(nodes []NodeConfig, edges []Edge) []Edge {
	nodeMap := make(map[string]NodeConfig)
	for _, n := range nodes {
		nodeMap[n.ID] = n
	}

	// Build set of upstream nodes for each node
	upstreams := make(map[string]map[string]bool)
	for _, e := range edges {
		if upstreams[e.Target] == nil {
			upstreams[e.Target] = make(map[string]bool)
		}
		upstreams[e.Target][e.Source] = true
	}

	var filtered []Edge
	for _, e := range edges {
		src := nodeMap[e.Source]
		// Skip edges from a reviewer node back to its upstream (back-edges)
		if isReviewerNode(src) && upstreams[src.ID] != nil && upstreams[src.ID][e.Target] {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered
}

// ComputeWaves does topological sort and groups independent nodes
// into waves for parallel execution. Returns an error if a cycle is detected.
func ComputeWaves(nodes []NodeConfig, edges []Edge) ([][]string, error) {
	// Deduplicate node IDs (defensive: frontend may produce duplicate IDs)
	seen := make(map[string]bool)
	var ids []string
	for _, n := range nodes {
		if !seen[n.ID] {
			seen[n.ID] = true
			ids = append(ids, n.ID)
		}
	}

	inDegree := make(map[string]int)
	dependsOn := make(map[string]map[string]bool)
	for _, id := range ids {
		inDegree[id] = 0
		dependsOn[id] = make(map[string]bool)
	}
	for _, e := range edges {
		inDegree[e.Target]++
		dependsOn[e.Target][e.Source] = true
	}

	done := make(map[string]bool)
	var waves [][]string

	for len(done) < len(ids) {
		var wave []string
		for _, id := range ids {
			if done[id] {
				continue
			}
			ready := true
			for dep := range dependsOn[id] {
				if !done[dep] {
					ready = false
					break
				}
			}
			if ready {
				wave = append(wave, id)
			}
		}
		if len(wave) == 0 {
			// Collect nodes in cycle
			var cycleNodes []string
			for _, id := range ids {
				if !done[id] {
					cycleNodes = append(cycleNodes, id)
				}
			}
			return waves, fmt.Errorf("cycle detected involving nodes: %s", strings.Join(cycleNodes, ", "))
		}
		waves = append(waves, wave)
		for _, id := range wave {
			done[id] = true
		}
	}
	return waves, nil
}
