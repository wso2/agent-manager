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

package utils

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

type extractionIdentifierYAML struct {
	Location   string `yaml:"location"`
	Identifier string `yaml:"identifier"`
}

type llmProviderTemplateAuthYAML struct {
	Type        string `yaml:"type"`
	Header      string `yaml:"header"`
	ValuePrefix string `yaml:"valuePrefix"`
}

type llmProviderTemplateMetadataYAML struct {
	EndpointURL    string                       `yaml:"endpointUrl"`
	Auth           *llmProviderTemplateAuthYAML `yaml:"auth"`
	LogoURL        string                       `yaml:"logoUrl"`
	OpenapiSpecURL string                       `yaml:"openapiSpecUrl"`
}

type llmProviderTemplateYAML struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		DisplayName      string                           `yaml:"displayName"`
		Metadata         *llmProviderTemplateMetadataYAML `yaml:"metadata"`
		PromptTokens     *extractionIdentifierYAML        `yaml:"promptTokens"`
		CompletionTokens *extractionIdentifierYAML        `yaml:"completionTokens"`
		TotalTokens      *extractionIdentifierYAML        `yaml:"totalTokens"`
		RemainingTokens  *extractionIdentifierYAML        `yaml:"remainingTokens"`
		RequestModel     *extractionIdentifierYAML        `yaml:"requestModel"`
		ResponseModel    *extractionIdentifierYAML        `yaml:"responseModel"`
	} `yaml:"spec"`
}

// LoadLLMProviderTemplatesFromDirectory loads LLM provider templates from a directory of YAML files
func LoadLLMProviderTemplatesFromDirectory(dirPath string) ([]*models.LLMProviderTemplate, error) {
	if strings.TrimSpace(dirPath) == "" {
		return nil, fmt.Errorf("template directory path is empty")
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read template directory %s: %w", dirPath, err)
	}

	res := make([]*models.LLMProviderTemplate, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".yaml") && !strings.HasSuffix(lower, ".yml") {
			continue
		}

		filePath := filepath.Join(dirPath, name)
		content, readErr := os.ReadFile(filePath)
		if readErr != nil {
			return nil, fmt.Errorf("failed to read template file %s: %w", filePath, readErr)
		}

		var doc llmProviderTemplateYAML
		if unmarshalErr := yaml.Unmarshal(content, &doc); unmarshalErr != nil {
			return nil, fmt.Errorf("failed to parse YAML template %s: %w", filePath, unmarshalErr)
		}

		if strings.TrimSpace(doc.Metadata.Name) == "" {
			return nil, fmt.Errorf("template file %s is missing metadata.name", filePath)
		}
		if strings.TrimSpace(doc.Spec.DisplayName) == "" {
			return nil, fmt.Errorf("template file %s is missing spec.displayName", filePath)
		}

		res = append(res, &models.LLMProviderTemplate{
			Handle:           doc.Metadata.Name,
			Name:             doc.Spec.DisplayName,
			Metadata:         mapTemplateMetadata(doc.Spec.Metadata),
			PromptTokens:     mapExtractionIdentifier(doc.Spec.PromptTokens),
			CompletionTokens: mapExtractionIdentifier(doc.Spec.CompletionTokens),
			TotalTokens:      mapExtractionIdentifier(doc.Spec.TotalTokens),
			RemainingTokens:  mapExtractionIdentifier(doc.Spec.RemainingTokens),
			RequestModel:     mapExtractionIdentifier(doc.Spec.RequestModel),
			ResponseModel:    mapExtractionIdentifier(doc.Spec.ResponseModel),
		})
	}

	b, _ := json.Marshal(res)

	slog.Info(string(b))

	return res, nil
}

func mapExtractionIdentifier(in *extractionIdentifierYAML) *models.ExtractionIdentifier {
	if in == nil {
		return nil
	}
	if strings.TrimSpace(in.Location) == "" || strings.TrimSpace(in.Identifier) == "" {
		return nil
	}
	return &models.ExtractionIdentifier{Location: in.Location, Identifier: in.Identifier}
}

func mapTemplateMetadata(in *llmProviderTemplateMetadataYAML) *models.LLMProviderTemplateMetadata {
	if in == nil {
		return nil
	}
	var auth *models.LLMProviderTemplateAuth
	if in.Auth != nil {
		auth = &models.LLMProviderTemplateAuth{
			Type:        strings.TrimSpace(in.Auth.Type),
			Header:      strings.TrimSpace(in.Auth.Header),
			ValuePrefix: in.Auth.ValuePrefix,
		}
	}
	out := &models.LLMProviderTemplateMetadata{
		EndpointURL:    strings.TrimSpace(in.EndpointURL),
		Auth:           auth,
		LogoURL:        strings.TrimSpace(in.LogoURL),
		OpenapiSpecURL: strings.TrimSpace(in.OpenapiSpecURL),
	}
	if out.EndpointURL == "" && out.LogoURL == "" && out.Auth == nil && out.OpenapiSpecURL == "" {
		return nil
	}
	return out
}
