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

// ComputeWaves does topological sort and groups independent nodes
// into waves for parallel execution. Returns an error if a cycle is detected.
func ComputeWaves(nodes []NodeConfig, edges []Edge) ([][]string, error) {
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID
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
