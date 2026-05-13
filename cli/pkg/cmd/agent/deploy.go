// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
// Licensed under the Apache License, Version 2.0.

package agent

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

// envConflict describes a single key where the CLI's --env value differs from
// what's currently configured on the agent.
type envConflict struct {
	Key              string
	CurrentValue     string
	NewValue         string
	CurrentSensitive bool
}

func parseEnvFlag(inputs []string) (map[string]string, error) {
	out := make(map[string]string, len(inputs))
	for _, raw := range inputs {
		idx := strings.IndexByte(raw, '=')
		if idx < 0 {
			return nil, fmt.Errorf("invalid --env %q: expected KEY=VALUE", raw)
		}
		key := raw[:idx]
		if key == "" {
			return nil, fmt.Errorf("invalid --env %q: empty key", raw)
		}
		out[key] = raw[idx+1:]
	}
	return out, nil
}

// findLowestEnvironment returns the entry environment of the deployment
// pipeline — the SourceEnvironmentRef that does not appear anywhere as a
// target. Mirrors the server-side selection rule; replicated client-side so we
// can fetch GetAgentConfigurations for that env before the deploy call.
func findLowestEnvironment(paths []gen.PromotionPath) string {
	targets := make(map[string]struct{})
	for _, p := range paths {
		for _, t := range p.TargetEnvironmentRefs {
			targets[t.Name] = struct{}{}
		}
	}
	for _, p := range paths {
		if _, isTarget := targets[p.SourceEnvironmentRef]; !isTarget {
			return p.SourceEnvironmentRef
		}
	}
	return ""
}

func mergeEnv(current []gen.ConfigurationItem, cli map[string]string) ([]gen.EnvironmentVariable, []envConflict) {
	final := make([]gen.EnvironmentVariable, 0, len(current)+len(cli))
	conflicts := make([]envConflict, 0, len(current)+len(cli))
	seen := make(map[string]struct{}, len(current))

	for _, c := range current {
		seen[c.Key] = struct{}{}
		isSensitive := c.IsSensitive != nil && *c.IsSensitive
		newVal, hasNew := cli[c.Key]
		switch {
		case !hasNew:
			val := c.Value
			ev := gen.EnvironmentVariable{Key: c.Key, Value: &val}
			if isSensitive {
				ev.IsSensitive = boolPtrLocal(true)
				if c.SecretRef != nil {
					ev.SecretRef = c.SecretRef
				}
				ev.Value = nil
			}
			final = append(final, ev)
		case isSensitive:
			v := newVal
			final = append(final, gen.EnvironmentVariable{Key: c.Key, Value: &v})
			conflicts = append(conflicts, envConflict{
				Key: c.Key, CurrentValue: "", NewValue: newVal, CurrentSensitive: true,
			})
		case newVal == c.Value:
			v := newVal
			final = append(final, gen.EnvironmentVariable{Key: c.Key, Value: &v})
		default:
			v := newVal
			final = append(final, gen.EnvironmentVariable{Key: c.Key, Value: &v})
			conflicts = append(conflicts, envConflict{
				Key: c.Key, CurrentValue: c.Value, NewValue: newVal, CurrentSensitive: false,
			})
		}
	}

	addedKeys := make([]string, 0, len(cli))
	for k := range cli {
		if _, ok := seen[k]; !ok {
			addedKeys = append(addedKeys, k)
		}
	}
	sort.Strings(addedKeys)
	for _, k := range addedKeys {
		v := cli[k]
		final = append(final, gen.EnvironmentVariable{Key: k, Value: &v})
	}

	return final, conflicts
}

func renderConflictTable(io *iostreams.IOStreams, conflicts []envConflict) {
	w := io.ErrOut

	sensitiveKeys := make([]string, 0)
	for _, c := range conflicts {
		if c.CurrentSensitive {
			sensitiveKeys = append(sensitiveKeys, c.Key)
		}
	}
	if len(sensitiveKeys) > 0 {
		fmt.Fprintf(w,
			"Warning: %s is currently stored as a secret. Replacing it via --env will "+
				"store the new value as plain text. Use the platform UI to keep it as a secret.\n\n",
			strings.Join(sensitiveKeys, ", "))
	}

	fmt.Fprintln(w, "The following env vars will be replaced:")
	fmt.Fprintln(w)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  KEY\tCURRENT\tNEW")
	for _, c := range conflicts {
		cur := c.CurrentValue
		newV := c.NewValue
		if c.CurrentSensitive {
			cur = "(secret)"
			newV = "***"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\n", c.Key, cur, newV)
	}
	_ = tw.Flush()
	fmt.Fprintln(w)
}

func boolPtrLocal(b bool) *bool { return &b }
