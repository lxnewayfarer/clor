package main

// ComputeWaves does topological sort and groups independent nodes
// into waves for parallel execution.
func ComputeWaves(nodes []NodeConfig, edges []Edge) [][]string {
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
			break // cycle detected
		}
		waves = append(waves, wave)
		for _, id := range wave {
			done[id] = true
		}
	}
	return waves
}
