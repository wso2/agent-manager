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

package gatewaytoken

import (
	"strings"
	"testing"
)

func TestNewTokensParse(t *testing.T) {
	token, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prefix, err := Prefix(token)
	if err != nil {
		t.Fatalf("Prefix(%q): %v", token, err)
	}
	if !strings.HasPrefix(token, prefix+"-") {
		t.Fatalf("prefix %q is not the start of %q", prefix, token)
	}
}

func TestPrefixRejectsMalformedTokens(t *testing.T) {
	good, _ := New()
	for _, token := range []string{
		"", "abc", good + "x", "not-a-uuid-at-all-0000000000" + good[28:],
		good[:37] + strings.Repeat("!", 43),
	} {
		if _, err := Prefix(token); err == nil {
			t.Errorf("Prefix(%q) accepted it", token)
		}
	}
}
