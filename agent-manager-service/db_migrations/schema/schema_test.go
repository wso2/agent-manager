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

package schema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The committed baselines must match lite.yaml. On failure, run `make schema`.
func TestBaselinesAreGenerated(t *testing.T) {
	s, err := Lite()
	require.NoError(t, err)
	for _, dialect := range Dialects {
		want, err := s.Render(dialect)
		require.NoError(t, err)
		got, err := os.ReadFile(filepath.Join("..", "..", BaselinePath(dialect)))
		require.NoError(t, err)
		assert.Equal(t, want, string(got), "%s baseline is stale: run `make schema`", dialect)
	}
}

func TestValidateRejectsRuleBreaks(t *testing.T) {
	tests := map[string]string{
		"nullable unique key": `
tables:
  - name: t
    columns: [{name: id, type: id}, {name: code, type: text, size: 10, nullable: true}]
    primary_key: [id]
    unique: [[code]]`,
		"two cascades": `
tables:
  - name: p
    columns: [{name: id, type: id}]
    primary_key: [id]
  - name: c
    columns: [{name: id, type: id}, {name: a, type: id}, {name: b, type: id}]
    primary_key: [id]
    foreign_keys:
      - {columns: [a], references: p, ref_columns: [id], on_delete: cascade}
      - {columns: [b], references: p, ref_columns: [id], on_delete: cascade}`,
		"foreign key to a column that is not a key": `
tables:
  - name: p
    columns: [{name: id, type: id}, {name: other, type: id}]
    primary_key: [id]
  - name: c
    columns: [{name: id, type: id}, {name: p_other, type: id}]
    primary_key: [id]
    foreign_keys: [{columns: [p_other], references: p, ref_columns: [other]}]`,
		"indexed long text": `
tables:
  - name: t
    columns: [{name: id, type: id}, {name: body, type: longtext}]
    primary_key: [id]
    indexes: [[body]]`,
		"unknown field": `
tables:
  - name: t
    columns: [{name: id, type: id, null: true}]
    primary_key: [id]`,
	}
	for name, doc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(doc))
			assert.Error(t, err)
		})
	}
}

func TestConstraintNamesStayShort(t *testing.T) {
	long := constraintName("idx", "a_rather_long_table_name", []string{"first_long_column", "second_long_column", "third"})
	assert.LessOrEqual(t, len(long), maxNameLength)
	assert.NotEqual(t, long, constraintName("idx", "a_rather_long_table_name", []string{"first_long_column", "second_long_column", "fourth"}))
	assert.Equal(t, "pk_agents", constraintName("pk", "agents", nil))
}

func TestNullableColumnsRender(t *testing.T) {
	s, err := Lite()
	require.NoError(t, err)
	ddl, err := s.Render(DialectPostgres)
	require.NoError(t, err)
	assert.Contains(t, ddl, "    description VARCHAR(1000),\n")
	assert.True(t, strings.Contains(ddl, "    handle VARCHAR(63) NOT NULL,\n"))
}
