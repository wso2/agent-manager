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

package aihub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
)

// Condition types. A plan has at most one condition of each built-in type. The model is the same as the API
// Platform's subscription plans, so the same gateway policies enforce it.
const (
	ConditionTypeRequestCount  = "request-count"
	ConditionTypeToolCallCount = "tool-call-count"
	ConditionTypeTokenCount    = "token-count"
	ConditionTypeCustom        = "custom"
)

// Fixed ids of the built-in conditions, the same in every plan.
const (
	ConditionIDRequests  = "requests"
	ConditionIDToolCalls = "tool-calls"
	ConditionIDTokens    = "tokens"
)

var builtInConditionIDs = map[string]string{
	ConditionTypeRequestCount:  ConditionIDRequests,
	ConditionTypeToolCallCount: ConditionIDToolCalls,
	ConditionTypeTokenCount:    ConditionIDTokens,
}

// What happens when a window is used up: reject the request, or let it through as overage.
const (
	OnExceedReject = "reject"
	OnExceedAllow  = "allow"
)

// The tokens a token-count condition counts.
const (
	TokenTypeTotal      = "total"
	TokenTypePrompt     = "prompt"
	TokenTypeCompletion = "completion"
)

// Limits on what a plan can hold.
const (
	maxPlanConditions             = 20
	maxConditionWindows           = 10
	maxCustomConditionParamsBytes = 4096
)

var (
	customConditionIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	timeUnits                = map[string]bool{"MINUTE": true, "HOUR": true, "DAY": true, "MONTH": true}
)

// Condition is one condition of a plan.
type Condition struct {
	ID     string          `json:"id"`
	Type   string          `json:"type"`
	Params json.RawMessage `json:"params"`
}

// Limits is the params of a built-in condition. Every window applies.
type Limits struct {
	Limits    []Window `json:"limits"`
	OnExceed  string   `json:"onExceed"`
	TokenType string   `json:"tokenType,omitempty"`
}

// Window allows Count units per TimeAmount TimeUnit. MONTH is 30 days.
type Window struct {
	Count      int    `json:"count"`
	TimeAmount int    `json:"timeAmount"`
	TimeUnit   string `json:"timeUnit"`
}

// validationError is a 400 with a message the caller may see.
type validationError struct{ msg string }

func (e *validationError) Error() string { return e.msg }

func invalid(format string, args ...any) error {
	return &validationError{msg: fmt.Sprintf(format, args...)}
}

// normalizeConditions validates a plan's conditions and returns them with defaults filled in and the JSON written
// one way, so equal conditions are stored as equal bytes. No conditions means an unlimited plan.
func normalizeConditions(conditions []Condition) ([]Condition, error) {
	if len(conditions) > maxPlanConditions {
		return nil, invalid("a plan can have at most %d conditions", maxPlanConditions)
	}
	out := make([]Condition, 0, len(conditions))
	seen := map[string]bool{}
	for i, c := range conditions {
		field := fmt.Sprintf("conditions[%d]", i)
		c.Type = strings.TrimSpace(c.Type)
		c.ID = strings.TrimSpace(c.ID)
		var err error
		switch c.Type {
		case ConditionTypeRequestCount, ConditionTypeToolCallCount, ConditionTypeTokenCount:
			id := builtInConditionIDs[c.Type]
			if c.ID != "" && c.ID != id {
				return nil, invalid("%s: the id of a %s condition is %s", field, c.Type, id)
			}
			if seen[id] {
				return nil, invalid("%s: a plan can have only one %s condition", field, c.Type)
			}
			c.ID = id
			c.Params, err = normalizeLimitParams(field, c.Type, c.Params)
		case ConditionTypeCustom:
			if !customConditionIDPattern.MatchString(c.ID) {
				return nil, invalid("%s: a custom condition needs an id of lowercase letters, digits and hyphens, "+
					"starting with a letter (at most 63 characters)", field)
			}
			if isBuiltInConditionID(c.ID) {
				return nil, invalid("%s: %s is the id of a built-in condition", field, c.ID)
			}
			if seen[c.ID] {
				return nil, invalid("%s: the plan already has a condition %s", field, c.ID)
			}
			c.Params, err = normalizeCustomParams(field, c.Params)
		case "":
			return nil, invalid("%s: type is required", field)
		default:
			return nil, invalid("%s: type must be one of request-count, tool-call-count, token-count or custom", field)
		}
		if err != nil {
			return nil, err
		}
		seen[c.ID] = true
		out = append(out, c)
	}
	return out, nil
}

func isBuiltInConditionID(id string) bool {
	for _, builtIn := range builtInConditionIDs {
		if id == builtIn {
			return true
		}
	}
	return false
}

// normalizeLimitParams validates the params of a built-in condition and fills in the defaults: timeAmount 0 is 1,
// onExceed defaults to reject, and tokenType (token-count only) to total.
func normalizeLimitParams(field, conditionType string, raw json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, invalid("%s: params with at least one limit is required", field)
	}
	var params Limits
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&params); err != nil {
		return nil, invalid("%s: invalid params: %s", field, err.Error())
	}
	if len(params.Limits) == 0 {
		return nil, invalid("%s: params.limits needs at least one limit", field)
	}
	if len(params.Limits) > maxConditionWindows {
		return nil, invalid("%s: a condition can have at most %d limits", field, maxConditionWindows)
	}
	seen := map[string]bool{}
	for j := range params.Limits {
		w := &params.Limits[j]
		limitField := fmt.Sprintf("%s.params.limits[%d]", field, j)
		if w.Count < 1 {
			return nil, invalid("%s: count must be at least 1", limitField)
		}
		if w.TimeAmount == 0 {
			w.TimeAmount = 1
		} else if w.TimeAmount < 0 {
			return nil, invalid("%s: timeAmount must be at least 1", limitField)
		}
		w.TimeUnit = strings.ToUpper(strings.TrimSpace(w.TimeUnit))
		if !timeUnits[w.TimeUnit] {
			return nil, invalid("%s: timeUnit must be one of MINUTE, HOUR, DAY or MONTH", limitField)
		}
		key := windowKey(w.TimeAmount, w.TimeUnit)
		if seen[key] {
			return nil, invalid("%s: the condition already has a limit over %d %s; each limit needs a window of its own length",
				limitField, w.TimeAmount, w.TimeUnit)
		}
		seen[key] = true
	}
	switch params.OnExceed {
	case "":
		params.OnExceed = OnExceedReject
	case OnExceedReject, OnExceedAllow:
	default:
		return nil, invalid("%s: params.onExceed must be reject or allow", field)
	}
	if conditionType == ConditionTypeTokenCount {
		switch params.TokenType {
		case "":
			params.TokenType = TokenTypeTotal
		case TokenTypeTotal, TokenTypePrompt, TokenTypeCompletion:
		default:
			return nil, invalid("%s: params.tokenType must be total, prompt or completion", field)
		}
	} else if params.TokenType != "" {
		return nil, invalid("%s: params.tokenType is for token-count conditions only", field)
	}
	return json.Marshal(params)
}

// normalizeCustomParams validates the key-value pairs of a custom condition: a JSON object of any values.
func normalizeCustomParams(field string, raw json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, invalid("%s: params is required; send {} for a custom condition with no values", field)
	}
	var params map[string]any
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	if err := dec.Decode(&params); err != nil || params == nil {
		return nil, invalid("%s: params must be a JSON object of key-value pairs", field)
	}
	for key := range params {
		if strings.TrimSpace(key) == "" {
			return nil, invalid("%s: params has an empty key", field)
		}
	}
	out, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	if len(out) > maxCustomConditionParamsBytes {
		return nil, invalid("%s: params must be at most %d bytes", field, maxCustomConditionParamsBytes)
	}
	return out, nil
}

// sameConditions reports whether two normalized condition lists are equal, in order.
func sameConditions(a, b []Condition) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Type != b[i].Type || !bytes.Equal(a[i].Params, b[i].Params) {
			return false
		}
	}
	return true
}

// windowKey names a window by its length, so 60 MINUTE and 1 HOUR match. Months match only months.
func windowKey(amount int, unit string) string {
	minutes := map[string]int{"MINUTE": 1, "HOUR": 60, "DAY": 24 * 60}[unit]
	if minutes == 0 {
		return fmt.Sprintf("M%d", amount)
	}
	// A length in minutes past the int range can't equal any other window: keep it apart.
	if amount > math.MaxInt/minutes || amount < math.MinInt/minutes {
		return fmt.Sprintf("%s%d", unit, amount)
	}
	return fmt.Sprintf("m%d", amount*minutes)
}
