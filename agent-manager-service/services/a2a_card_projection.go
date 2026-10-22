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

package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// a2aCardProjection derives the fingerprint and search text stored alongside a card.
// The fingerprint ignores interface URLs, which differ between environments.
func a2aCardProjection(card json.RawMessage) (models.A2ACardProjection, error) {
	if len(card) == 0 {
		return models.A2ACardProjection{}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(card))
	dec.UseNumber()
	var decoded interface{}
	if err := dec.Decode(&decoded); err != nil {
		return models.A2ACardProjection{}, fmt.Errorf("failed to project agent card: %w", err)
	}
	object, isObject := decoded.(map[string]interface{})
	if !isObject {
		fingerprint, err := a2aCardHash(card)
		if err != nil {
			return models.A2ACardProjection{}, fmt.Errorf("failed to project agent card: %w", err)
		}
		return models.A2ACardProjection{Fingerprint: fingerprint}, nil
	}

	searchText := a2aCardSearchText(object)
	removeInterfaceURLs(object)
	canonical, err := json.Marshal(object)
	if err != nil {
		return models.A2ACardProjection{}, fmt.Errorf("failed to project agent card: %w", err)
	}
	fingerprint, err := a2aCardHash(canonical)
	if err != nil {
		return models.A2ACardProjection{}, fmt.Errorf("failed to project agent card: %w", err)
	}
	return models.A2ACardProjection{Fingerprint: fingerprint, SearchText: searchText}, nil
}

func removeInterfaceURLs(card map[string]interface{}) {
	interfaces, _ := card["supportedInterfaces"].([]interface{})
	for _, entry := range interfaces {
		if iface, ok := entry.(map[string]interface{}); ok {
			delete(iface, "url")
		}
	}
}

func a2aCardSearchText(card map[string]interface{}) string {
	var parts []string
	parts = appendString(parts, card["name"])
	parts = appendString(parts, card["description"])
	for _, skill := range readableSkills(card["skills"]) {
		parts = appendString(parts, skill["name"])
		parts = appendString(parts, skill["description"])
		parts = appendStrings(parts, skill["tags"])
		parts = appendStrings(parts, skill["examples"])
	}
	return strings.Join(parts, "\n")
}

// readableSkills returns no skills at all unless every element is an object.
func readableSkills(value interface{}) []map[string]interface{} {
	elements, _ := value.([]interface{})
	skills := make([]map[string]interface{}, 0, len(elements))
	for _, element := range elements {
		skill, ok := element.(map[string]interface{})
		if !ok {
			return nil
		}
		skills = append(skills, skill)
	}
	return skills
}

func appendString(parts []string, value interface{}) []string {
	if text, ok := value.(string); ok && text != "" {
		return append(parts, text)
	}
	return parts
}

func appendStrings(parts []string, value interface{}) []string {
	elements, _ := value.([]interface{})
	for _, element := range elements {
		parts = appendString(parts, element)
	}
	return parts
}
