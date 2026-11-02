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

package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestExponentialBackoff(t *testing.T) {
	assert.Equal(t, 5*time.Second, ExponentialBackoff(0, 5*time.Second, 5*time.Minute))
	assert.Equal(t, 10*time.Second, ExponentialBackoff(1, 5*time.Second, 5*time.Minute))
	assert.Equal(t, 160*time.Second, ExponentialBackoff(5, 5*time.Second, 5*time.Minute))
	assert.Equal(t, 5*time.Minute, ExponentialBackoff(6, 5*time.Second, 5*time.Minute))
	assert.Equal(t, 5*time.Minute, ExponentialBackoff(1000, 5*time.Second, 5*time.Minute), "no overflow")
}
