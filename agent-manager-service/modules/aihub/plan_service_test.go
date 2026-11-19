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
	"testing"

	"github.com/stretchr/testify/assert"
)

// The search matches names case-insensitively beyond ASCII, as stored in display_name_folded, and takes LIKE
// wildcards literally.
func TestSearchFoldsUnicodeAndEscapesWildcards(t *testing.T) {
	assert.Equal(t, foldName("équipe"), foldName("ÉQUIPE"))
	assert.Equal(t, foldName("équipe"), foldName("équipe"), "one normal form")
	clause, args := searchClause(" ÉQUIPE_1% ")
	assert.Contains(t, clause, "display_name_folded LIKE ?")
	assert.Equal(t, []any{"%équipe\\_1\\%%", "%équipe\\_1\\%%"}, args)
}
