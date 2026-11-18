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

package db

import (
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/config"
)

// TestLockPoolConfigsDropsOpenCap guards the point of the lock pool: however
// the main pool is capped, lock holders must never wait on a capped pool.
func TestLockPoolConfigsDropsOpenCap(t *testing.T) {
	maxOpen, maxIdle, lifetime := int64(5), int64(10), int64(300)
	main := config.DbConfigs{
		SlowThresholdMilliseconds: 200,
		MaxOpenCount:              &maxOpen,
		MaxIdleCount:              &maxIdle,
		MaxLifetimeSeconds:        &lifetime,
	}

	got := lockPoolConfigs(main)

	if got.MaxOpenCount != nil {
		t.Errorf("MaxOpenCount = %d, want nil (no cap)", *got.MaxOpenCount)
	}
	if got.MaxIdleCount == nil || *got.MaxIdleCount != lockPoolMaxIdle {
		t.Errorf("MaxIdleCount = %v, want %d", got.MaxIdleCount, lockPoolMaxIdle)
	}
	if got.MaxLifetimeSeconds != &lifetime || got.SlowThresholdMilliseconds != 200 {
		t.Error("lifetime and logging settings must carry over from the main pool")
	}
	if main.MaxOpenCount != &maxOpen || *main.MaxOpenCount != 5 || main.MaxIdleCount != &maxIdle || *main.MaxIdleCount != 10 {
		t.Error("deriving the lock pool's settings must not change the main pool's")
	}
}
