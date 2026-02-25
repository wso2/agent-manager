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

package models

import (
	"github.com/google/uuid"
)

// EvaluatorConfigParam represents a single configuration parameter for an evaluator.
// JSON tags use snake_case to match the Python-generated catalog output.
type EvaluatorConfigParam struct {
	Key         string      `json:"key"`
	Type        string      `json:"type"` // string, integer, float, boolean, array, enum
	Description string      `json:"description"`
	Required    bool        `json:"required"`
	Default     interface{} `json:"default,omitempty"`
	Min         *float64    `json:"min,omitempty"`
	Max         *float64    `json:"max,omitempty"`
	EnumValues  []string    `json:"enum_values,omitempty"`
}

// EvaluatorResponse is the API response DTO for an evaluator
type EvaluatorResponse struct {
	ID           uuid.UUID              `json:"id"`
	Identifier   string                 `json:"identifier"`
	DisplayName  string                 `json:"displayName"`
	Description  string                 `json:"description"`
	Version      string                 `json:"version"`
	Provider     string                 `json:"provider"`
	Level        string                 `json:"level"`
	Tags         []string               `json:"tags"`
	IsBuiltin    bool                   `json:"isBuiltin"`
	ConfigSchema []EvaluatorConfigParam `json:"configSchema"`
}
