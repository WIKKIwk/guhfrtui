package update

import (
	"strings"

	"new_era_go/internal/discovery"
	"new_era_go/internal/reader"
)

func ClampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func BuildConnectPlan(candidates []discovery.Candidate, preferredIndex int, scanPorts []int) []reader.Endpoint {
	if len(candidates) == 0 {
		return nil
	}

	portFallback := []int{2022, 5000, 27011, 6000, 4001, 10001}
	indexOrder := CandidateConnectOrder(len(candidates), preferredIndex)
	plan := make([]reader.Endpoint, 0, len(indexOrder)*4)
	seen := make(map[string]struct{}, len(indexOrder)*8)

	addEndpoint := func(host string, port int) {
		if strings.TrimSpace(host) == "" || port <= 0 {
			return
		}
		ep := reader.Endpoint{Host: host, Port: port}
		key := ep.Address()
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		plan = append(plan, ep)
	}

	for _, idx := range indexOrder {
		candidate := candidates[idx]
		host := candidate.Host
		ports := MergePortOrder(append([]int{candidate.Port}, scanPorts...), portFallback)
		for _, port := range ports {
			addEndpoint(host, port)
		}
	}

	return plan
}

func CandidateConnectOrder(total int, preferred int) []int {
	if total <= 0 {
		return nil
	}
	order := make([]int, 0, total)
	seen := make(map[int]struct{}, total)

	appendIndex := func(i int) {
		if i < 0 || i >= total {
			return
		}
		if _, ok := seen[i]; ok {
			return
		}
		seen[i] = struct{}{}
		order = append(order, i)
	}

	appendIndex(preferred)
	for i := 0; i < total; i++ {
		appendIndex(i)
	}
	return order
}

func MergePortOrder(primary []int, fallback []int) []int {
	out := make([]int, 0, len(primary)+len(fallback))
	seen := make(map[int]struct{}, len(primary)+len(fallback))

	appendPort := func(port int) {
		if port <= 0 {
			return
		}
		if _, ok := seen[port]; ok {
			return
		}
		seen[port] = struct{}{}
		out = append(out, port)
	}

	for _, port := range primary {
		appendPort(port)
	}
	for _, port := range fallback {
		appendPort(port)
	}

	return out
}

func NextInventoryAntenna(mask byte, start int) (byte, int) {
	if mask == 0 {
		mask = 0x01
	}
	start = ((start % 8) + 8) % 8

	for i := 0; i < 8; i++ {
		idx := (start + i) % 8
		if mask&(byte(1)<<idx) != 0 {
			return byte(0x80 | idx), (idx + 1) % 8
		}
	}
	return 0x80, start
}

func PreferredVerifiedCandidateIndex(candidates []discovery.Candidate) int {
	if len(candidates) == 0 {
		return -1
	}
	for i, candidate := range candidates {
		if candidate.Verified {
			return i
		}
	}
	return -1
}

func PreferredCandidateIndex(candidates []discovery.Candidate) int {
	if len(candidates) == 0 {
		return -1
	}
	if idx := PreferredVerifiedCandidateIndex(candidates); idx >= 0 {
		return idx
	}
	return 0
}

func CountVerifiedCandidates(candidates []discovery.Candidate) int {
	count := 0
	for _, candidate := range candidates {
		if candidate.Verified {
			count++
		}
	}
	return count
}
