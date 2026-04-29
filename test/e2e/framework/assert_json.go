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

package framework

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// AssertJSONMatch compares an actual response against an expected JSON file.
// Only fields present in the expected file are compared — extra fields in the
// actual response are ignored. This allows omitting dynamic fields like uuid,
// timestamps, and status from the expected file.
//
// The expected file path is relative to the testdata directory.
// The actual value can be any struct or map that marshals to JSON.
func AssertJSONMatch(t *testing.T, testdataPath string, actual any) {
	t.Helper()

	expectedData, err := os.ReadFile(filepath.Join("testdata", testdataPath))
	if err != nil {
		t.Fatalf("failed to read expected response file %s: %v", testdataPath, err)
	}

	var expected map[string]any
	if err := json.Unmarshal(expectedData, &expected); err != nil {
		t.Fatalf("failed to unmarshal expected response %s: %v", testdataPath, err)
	}

	actualJSON, err := json.Marshal(actual)
	if err != nil {
		t.Fatalf("failed to marshal actual response: %v", err)
	}

	var actualMap map[string]any
	if err := json.Unmarshal(actualJSON, &actualMap); err != nil {
		t.Fatalf("failed to unmarshal actual response: %v", err)
	}

	errs := matchFields("", expected, actualMap)
	for _, e := range errs {
		t.Errorf("response mismatch: %s", e)
	}
}

// matchFields recursively compares expected fields against actual values.
// Only fields present in expected are checked.
func matchFields(prefix string, expected, actual map[string]any) []string {
	var errs []string

	for key, expectedVal := range expected {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}

		actualVal, exists := actual[key]
		if !exists {
			errs = append(errs, fmt.Sprintf("%s: field missing in response", path))
			continue
		}

		switch ev := expectedVal.(type) {
		case map[string]any:
			av, ok := actualVal.(map[string]any)
			if !ok {
				errs = append(errs, fmt.Sprintf("%s: expected object, got %T", path, actualVal))
				continue
			}
			errs = append(errs, matchFields(path, ev, av)...)

		case []any:
			av, ok := actualVal.([]any)
			if !ok {
				errs = append(errs, fmt.Sprintf("%s: expected array, got %T", path, actualVal))
				continue
			}
			if len(ev) != len(av) {
				errs = append(errs, fmt.Sprintf("%s: expected array length %d, got %d", path, len(ev), len(av)))
				continue
			}
			for i := range ev {
				evItem, evOk := ev[i].(map[string]any)
				avItem, avOk := av[i].(map[string]any)
				if evOk && avOk {
					errs = append(errs, matchFields(fmt.Sprintf("%s[%d]", path, i), evItem, avItem)...)
				} else if !reflect.DeepEqual(ev[i], av[i]) {
					errs = append(errs, fmt.Sprintf("%s[%d]: expected %v, got %v", path, i, ev[i], av[i]))
				}
			}

		default:
			if !reflect.DeepEqual(expectedVal, actualVal) {
				errs = append(errs, fmt.Sprintf("%s: expected %v, got %v", path, expectedVal, actualVal))
			}
		}
	}

	return errs
}
