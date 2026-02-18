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

package repositories

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEscapeLikeWildcards(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "No special characters",
			input:    "openai",
			expected: "openai",
		},
		{
			name:     "Escape percent sign",
			input:    "50%",
			expected: "50\\%",
		},
		{
			name:     "Escape underscore",
			input:    "test_name",
			expected: "test\\_name",
		},
		{
			name:     "Escape backslash",
			input:    "path\\to\\file",
			expected: "path\\\\to\\\\file",
		},
		{
			name:     "Escape all special characters",
			input:    "test_%\\pattern",
			expected: "test\\_\\%\\\\pattern",
		},
		{
			name:     "SQL injection attempt with percent",
			input:    "%' OR '1'='1",
			expected: "\\%' OR '1'='1",
		},
		{
			name:     "SQL injection attempt with underscore",
			input:    "_' OR '1'='1",
			expected: "\\_' OR '1'='1",
		},
		{
			name:     "Multiple backslashes",
			input:    "\\\\test",
			expected: "\\\\\\\\test",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Only special characters",
			input:    "%_\\",
			expected: "\\%\\_\\\\",
		},
		{
			name:     "Unicode characters should pass through",
			input:    "test™®©",
			expected: "test™®©",
		},
		{
			name:     "Complex pattern with all wildcards",
			input:    "test%name_with\\backslash",
			expected: "test\\%name\\_with\\\\backslash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeLikeWildcards(tt.input)
			assert.Equal(t, tt.expected, result,
				"escapeLikeWildcards(%q) = %q, want %q", tt.input, result, tt.expected)
		})
	}
}

func TestEscapeLikeWildcardsDoSPrevention(t *testing.T) {
	// Test with very long string to ensure escaping doesn't cause performance issues
	longString := generateLongString(10000, 'a')
	result := escapeLikeWildcards(longString)
	assert.Equal(t, longString, result, "Long string without special chars should be unchanged")

	// Test with many special characters
	specialString := generateLongString(1000, '%')
	result = escapeLikeWildcards(specialString)
	// Each % becomes \%, so length should be 2x
	assert.Len(t, result, 2000, "Each special character should be escaped")
}

func TestEscapeLikeWildcardsPreservesOrder(t *testing.T) {
	// Ensure that escaping backslash first doesn't double-escape other characters
	input := "\\%_"
	result := escapeLikeWildcards(input)
	expected := "\\\\\\%\\_"
	assert.Equal(t, expected, result,
		"Backslash escaping should not double-escape other characters")
}

func generateLongString(length int, char rune) string {
	result := make([]rune, length)
	for i := range result {
		result[i] = char
	}
	return string(result)
}
