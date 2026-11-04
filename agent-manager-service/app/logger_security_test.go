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

package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/config"
)

// Exercise setupLogger itself so changing the application's output format
// cannot silently invalidate the protection used by structured logging calls.
func TestSetupLogger_ContainsUntrustedValuesInJSONRecords(t *testing.T) {
	output, err := os.CreateTemp(t.TempDir(), "logs")
	require.NoError(t, err)
	previousOutput, previousLogger := os.Stdout, slog.Default()
	previousWriter, previousFlags := log.Writer(), log.Flags()
	t.Cleanup(func() {
		require.Same(t, previousWriter, log.Writer(), "standard log output must be restored")
		require.Equal(t, previousFlags, log.Flags(), "standard log flags must be restored")
	})
	t.Cleanup(func() {
		os.Stdout = previousOutput
		slog.SetDefault(previousLogger)
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
		require.NoError(t, output.Close())
	})
	os.Stdout = output
	setupLogger(&config.Config{LogLevel: "DEBUG"})

	payloads := []string{
		"value\r\nforged event",
		`value","level":"ERROR","msg":"forged`,
		"quotes\" backslash\\ tab\t NUL\x00 escape\x1b",
		"unicode\u2028separator\u2029value",
	}
	for _, value := range payloads {
		slog.Info("direct", "ouID", "trusted-org", "requestID", "trusted-request", "value", value)
		slog.With("ouID", "trusted-org", "requestID", "trusted-request", "value", value).Warn("inherited")
		slog.Error("error", "ouID", "trusted-org", "requestID", "trusted-request", "error", fmt.Errorf("upstream: %w", errors.New(value)))
		slog.Debug("collection", "ouID", "trusted-org", "requestID", "trusted-request", "errors", []string{value, "second"})
	}
	_, err = output.Seek(0, 0)
	require.NoError(t, err)
	scanner := bufio.NewScanner(output)
	count := 0
	for scanner.Scan() {
		var record map[string]any
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &record), "each physical line must be a complete JSON event")
		require.NotEmpty(t, record["time"])
		if count == 0 {
			require.Equal(t, "Logger configured", record["msg"])
		} else {
			require.LessOrEqual(t, count, len(payloads)*4)
			value := payloads[(count-1)/4]
			require.Equal(t, "trusted-org", record["ouID"])
			require.Equal(t, "trusted-request", record["requestID"])
			switch (count - 1) % 4 {
			case 0:
				require.Equal(t, "INFO", record["level"])
				require.Equal(t, "direct", record["msg"])
				require.Equal(t, value, record["value"])
			case 1:
				require.Equal(t, "WARN", record["level"])
				require.Equal(t, "inherited", record["msg"])
				require.Equal(t, value, record["value"])
			case 2:
				require.Equal(t, "ERROR", record["level"])
				require.Equal(t, "error", record["msg"])
				require.Equal(t, "upstream: "+value, record["error"])
			case 3:
				require.Equal(t, "DEBUG", record["level"])
				require.Equal(t, "collection", record["msg"])
				require.Equal(t, []any{value, "second"}, record["errors"])
			}
		}
		count++
	}
	require.NoError(t, scanner.Err())
	require.Equal(t, 1+len(payloads)*4, count)
}
