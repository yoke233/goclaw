package agent

import (
	"strings"

	sdkapi "github.com/cexll/agentsdk-go/pkg/api"
)

func mergeCommandRegistrations(base, override []sdkapi.CommandRegistration) []sdkapi.CommandRegistration {
	merged := make([]sdkapi.CommandRegistration, 0, len(base)+len(override))
	index := map[string]int{}

	add := func(reg sdkapi.CommandRegistration) {
		key := strings.ToLower(strings.TrimSpace(reg.Definition.Name))
		if key == "" {
			return
		}
		if idx, ok := index[key]; ok {
			merged[idx] = reg
			return
		}
		index[key] = len(merged)
		merged = append(merged, reg)
	}

	for _, reg := range base {
		add(reg)
	}
	for _, reg := range override {
		add(reg)
	}
	return merged
}

func mergeSubagentRegistrations(base, override []sdkapi.SubagentRegistration) []sdkapi.SubagentRegistration {
	merged := make([]sdkapi.SubagentRegistration, 0, len(base)+len(override))
	index := map[string]int{}

	add := func(reg sdkapi.SubagentRegistration) {
		key := strings.ToLower(strings.TrimSpace(reg.Definition.Name))
		if key == "" {
			return
		}
		if idx, ok := index[key]; ok {
			merged[idx] = reg
			return
		}
		index[key] = len(merged)
		merged = append(merged, reg)
	}

	for _, reg := range base {
		add(reg)
	}
	for _, reg := range override {
		add(reg)
	}
	return merged
}
