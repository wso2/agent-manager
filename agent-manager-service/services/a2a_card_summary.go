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

import "encoding/json"

type a2aCardSummary struct {
	Name        string
	Description string
	Version     string
	Skills      []string
}

func a2aCardSummaryOf(card json.RawMessage) a2aCardSummary {
	summary := a2aCardSummary{Skills: []string{}}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(card, &fields); err != nil {
		return summary
	}
	summary.Name, _ = jsonString(fields["name"])
	summary.Description, _ = jsonString(fields["description"])
	summary.Version, _ = jsonString(fields["version"])
	summary.Skills = skillNamesOf(fields["skills"])
	return summary
}

func skillNamesOf(raw json.RawMessage) []string {
	names := []string{}
	var skills []json.RawMessage
	if err := json.Unmarshal(raw, &skills); err != nil {
		return names
	}
	for _, rawSkill := range skills {
		var skill map[string]json.RawMessage
		if err := json.Unmarshal(rawSkill, &skill); err != nil {
			continue
		}
		if name, ok := jsonString(skill["name"]); ok && name != "" {
			names = append(names, name)
		}
	}
	return names
}
