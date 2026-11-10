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

package a2a

import (
	"fmt"
	"time"

	. "github.com/onsi/gomega"
)

const statusFailed = "failed"

// waitForStatus polls get every 2s until it reports the wanted status. A
// not-found result counts as not yet. A failed status stops at once with the
// last error; a timeout reports the last seen status and error.
func waitForStatus(get func() (status, lastError string, found bool), want, label string, timeout time.Duration) {
	Eventually(func(g Gomega) {
		status, lastError, found := get()
		if found && status == statusFailed {
			StopTrying(fmt.Sprintf("%s failed: %s", label, lastError)).Now()
		}
		g.Expect(found).To(BeTrue(), "%s not found yet", label)
		g.Expect(status).To(Equal(want), "%s status %q, lastError %q", label, status, lastError)
	}).WithTimeout(timeout).WithPolling(2 * time.Second).Should(Succeed())
}
