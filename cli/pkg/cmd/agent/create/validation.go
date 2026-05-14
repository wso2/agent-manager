// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package create

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
)

var envKeyRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validate(opts *CreateOptions) error {
	var v []string

	if opts.Name == "" {
		v = append(v, "name argument is required")
	} else if strings.Contains(opts.Name, "/") {
		v = append(v, "name must not contain '/'")
	}
	if opts.DisplayName == "" {
		v = append(v, "--display-name is required")
	}

	switch opts.Provisioning {
	case provisioningExternal:
		return cmdutil.FlagErrorf("external provisioning is not yet supported by amctl agent create")
	case provisioningInternal:
		v = append(v, validateInternal(opts)...)
	default:
		v = append(v, fmt.Sprintf("--provisioning must be %q or %q, got %q", provisioningInternal, provisioningExternal, opts.Provisioning))
	}

	if len(v) == 0 {
		return nil
	}
	return cmdutil.FlagErrors(v)
}

func validateInternal(opts *CreateOptions) []string {
	var v []string

	if opts.RepoURL == "" {
		v = append(v, "--repo-url is required for internal provisioning")
	}
	if opts.RepoBranch == "" {
		v = append(v, "--repo-branch is required for internal provisioning")
	}
	if opts.RepoPath == "" {
		v = append(v, "--repo-path is required for internal provisioning")
	}

	switch opts.BuildType {
	case buildTypeBuildpack:
		if opts.Language == "" {
			v = append(v, "--build-type=buildpack requires --language")
		}
		if opts.LanguageVersion == "" {
			v = append(v, "--build-type=buildpack requires --language-version")
		}
		if opts.RunCommand == "" {
			v = append(v, "--build-type=buildpack requires --run-command")
		}
		if opts.Dockerfile != "" {
			v = append(v, "--build-type=buildpack conflicts with --dockerfile")
		}
	case buildTypeDocker:
		if opts.Dockerfile == "" {
			v = append(v, "--build-type=docker requires --dockerfile")
		}
		if opts.Language != "" {
			v = append(v, "--build-type=docker conflicts with --language")
		}
		if opts.LanguageVersion != "" {
			v = append(v, "--build-type=docker conflicts with --language-version")
		}
		if opts.RunCommand != "" {
			v = append(v, "--build-type=docker conflicts with --run-command")
		}
	case "":
		v = append(v, "--build-type is required for internal provisioning")
	default:
		v = append(v, fmt.Sprintf("--build-type must be %q or %q, got %q", buildTypeBuildpack, buildTypeDocker, opts.BuildType))
	}

	switch opts.SubType {
	case subTypeChatAPI, subTypeCustomAPI:
	case "":
		v = append(v, "--subtype is required (chat-api or custom-api)")
	default:
		v = append(v, fmt.Sprintf("--subtype must be %q or %q, got %q", subTypeChatAPI, subTypeCustomAPI, opts.SubType))
	}
	if opts.SubType == subTypeChatAPI {
		if opts.PortSet {
			v = append(v, "--port is not allowed for subtype chat-api")
		}
		if opts.BasePath != "" {
			v = append(v, "--base-path is not allowed for subtype chat-api")
		}
		if opts.OpenAPISpec != "" {
			v = append(v, "--openapi-spec is not allowed for subtype chat-api")
		}
	} else {
		if opts.Port < 1 || opts.Port > 65535 {
			v = append(v, fmt.Sprintf("--port must be 1..65535, got %d", opts.Port))
		}
		if opts.SubType == subTypeCustomAPI {
			if opts.BasePath == "" {
				v = append(v, "--subtype=custom-api requires --base-path")
			}
			if opts.OpenAPISpec == "" {
				v = append(v, "--subtype=custom-api requires --openapi-spec")
			}
		}
	}

	seen := map[string]string{}
	v = append(v, validateEnvSlice(opts.Env, "--env", seen)...)
	v = append(v, validateEnvSlice(opts.EnvSecret, "--env-secret", seen)...)
	v = append(v, validateEnvSlice(opts.EnvFromSecret, "--env-from-secret", seen)...)

	if opts.ModelConfigFile != "" {
		if _, err := loadModelConfig(opts.ModelConfigFile); err != nil {
			v = append(v, fmt.Sprintf("--model-config-file: %s", err))
		}
	}

	return v
}

func validateEnvSlice(entries []string, flag string, seen map[string]string) []string {
	var v []string
	for _, entry := range entries {
		key, err := parseEnvKey(entry)
		if err != nil {
			v = append(v, fmt.Sprintf("%s %q: %s", flag, entry, err))
			continue
		}
		if prev, dup := seen[key]; dup {
			v = append(v, fmt.Sprintf("duplicate env key %q (set by %s and %s)", key, prev, flag))
			continue
		}
		seen[key] = flag
	}
	return v
}

func parseEnvKey(entry string) (string, error) {
	idx := strings.IndexByte(entry, '=')
	if idx < 0 {
		return "", fmt.Errorf("missing '=' separator")
	}
	key := entry[:idx]
	if !envKeyRE.MatchString(key) {
		return "", fmt.Errorf("invalid key %q (must match [A-Za-z_][A-Za-z0-9_]*)", key)
	}
	return key, nil
}
