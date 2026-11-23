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

package gwutil

import "math/big"

// DecimalGreater reports whether decimal a is greater than decimal b, as money is stored
// (a decimal string such as "80.14"); an unreadable value counts as 0.
func DecimalGreater(a, b string) bool {
	x, ok := new(big.Rat).SetString(a)
	if !ok {
		x = new(big.Rat)
	}
	y, ok := new(big.Rat).SetString(b)
	if !ok {
		y = new(big.Rat)
	}
	return x.Cmp(y) > 0
}
