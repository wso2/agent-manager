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

package iostreams

const (
	ansiReset    = "\033[0m"
	ansiBold     = "\033[1m"
	ansiFgRed    = "\033[31m"
	ansiFgGreen  = "\033[32m"
	ansiFgCyan   = "\033[36m"
	ansiFgGray   = "\033[90m"
)

type ColorScheme struct {
	Enabled bool
}

func (cs *ColorScheme) apply(code, t string) string {
	if !cs.Enabled {
		return t
	}
	return code + t + ansiReset
}

func (cs *ColorScheme) Bold(t string) string  { return cs.apply(ansiBold, t) }
func (cs *ColorScheme) Red(t string) string   { return cs.apply(ansiFgRed, t) }
func (cs *ColorScheme) Green(t string) string { return cs.apply(ansiFgGreen, t) }
func (cs *ColorScheme) Cyan(t string) string  { return cs.apply(ansiFgCyan, t) }
func (cs *ColorScheme) Gray(t string) string  { return cs.apply(ansiFgGray, t) }

func (cs *ColorScheme) SuccessIcon() string { return cs.Green("✓") }
func (cs *ColorScheme) FailureIcon() string { return cs.Red("X") }

func (cs *ColorScheme) TableHeader(t string) string {
	if !cs.Enabled {
		return t
	}
	return ansiBold + t + ansiReset
}
