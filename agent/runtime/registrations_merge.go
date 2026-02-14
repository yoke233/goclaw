package runtime

import (
	"strings"

	sdkapi "github.com/cexll/agentsdk-go/pkg/api"
)

func mergeSkillRegistrations(base, override []sdkapi.SkillRegistration) []sdkapi.SkillRegistration {
	merged := make([]sdkapi.SkillRegistration, 0, len(base)+len(override))
	index := map[string]int{}

	add := func(reg sdkapi.SkillRegistration) {
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
