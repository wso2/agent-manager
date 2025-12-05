// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
)

type configReader struct {
	errors []error
}

func (c *configReader) logAndExitIfErrorsFound() {
	if len(c.errors) > 0 {
		var errors []string
		for _, err := range c.errors {
			errors = append(errors, err.Error())
		}
		slog.Error("configReader: errors found while reading config", "errors", errors)
		os.Exit(1)
	}
}

func (c *configReader) readRequiredString(envVarName string) string {
	v := os.Getenv(envVarName)
	if v == "" {
		c.errors = append(c.errors, fmt.Errorf("required environment variable %s not found", envVarName))
	}
	return v
}

func (c *configReader) readOptionalString(envVarName string, defaultValue string) string {
	v := os.Getenv(envVarName)
	if v == "" {
		return defaultValue
	}
	return v
}

func (c *configReader) readOptionalInt64(envVarName string, defaultValue int64) int64 {
	v := os.Getenv(envVarName)
	if v == "" {
		return defaultValue
	}
	value, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		c.errors = append(c.errors, fmt.Errorf("optional environment variable %s is not a valid integer [%w]", envVarName, err))
		return 0
	}
	return value
}

func (c *configReader) readNullableInt64(envVarName string) *int64 {
	v := os.Getenv(envVarName)
	if v == "" {
		return nil
	}
	value, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		c.errors = append(c.errors, fmt.Errorf("nullable environment variable %s is not a valid integer [%w]", envVarName, err))
		return nil
	}
	return &value
}

func (c *configReader) readOptionalBool(envVarName string, defaultValue bool) bool {
	v := os.Getenv(envVarName)
	if v == "" {
		return defaultValue
	}
	value, err := strconv.ParseBool(v)
	if err != nil {
		return defaultValue
	}
	return value
}
