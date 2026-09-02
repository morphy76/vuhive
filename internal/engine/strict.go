package engine

import "sort"

// UnusedParams returns scenario parameter keys that are declared in the YAML configuration
// but were never accessed via Param(), ParamInt(), or ParamDuration() during execution.
// Returns nil if ctx is not a *scenarioContext or if no unused params exist.
func UnusedParams(ctx ScenarioContext) []string {
	sc, ok := ctx.(*scenarioContext)
	if !ok || sc.accessedParams == nil || sc.params == nil {
		return nil
	}

	var unused []string
	for key := range sc.params {
		if _, accessed := sc.accessedParams.Load(key); !accessed {
			unused = append(unused, key)
		}
	}

	if len(unused) == 0 {
		return nil
	}

	sort.Strings(unused)
	return unused
}
