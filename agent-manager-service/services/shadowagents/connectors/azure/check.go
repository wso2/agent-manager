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

package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// testMaxPages bounds the pages a connection test reads to find an account
// or a project to probe.
const testMaxPages = 5

// tester collects the checks of a connection test. Once a check fails
// authentication, the remaining ones are skipped.
type tester struct {
	checks     []models.ShadowConnectionCheck
	authFailed bool
}

func newTester() *tester { return &tester{} }

// run runs one check unless skip is set (then it is recorded as skipped with
// skipReason) or authentication already failed.
func (t *tester) run(name string, unlocks []string, skip bool, skipReason string, fn func() (int, error)) {
	check := models.ShadowConnectionCheck{Name: name, Unlocks: unlocks}
	switch {
	case t.authFailed:
		check.Outcome, check.Message = models.ShadowCallSkipped, "not called: authentication failed"
	case skip:
		check.Outcome, check.Message = models.ShadowCallSkipped, skipReason
	default:
		n, err := fn()
		check.Outcome, check.Message = sa.OutcomeOf(err), sa.MessageOf(err)
		if err == nil {
			check.Message = itemsRead(n)
		} else if isAuthFailure(err) {
			t.authFailed = true
		}
	}
	t.checks = append(t.checks, check)
}

// itemsRead says how many items a passing check read; the outcome itself is
// shown next to it, so the message carries only the count.
func itemsRead(n int) string {
	if n == 1 {
		return "1 item read"
	}
	return fmt.Sprintf("%d items read", n)
}

// ok reports whether the named check passed.
func (t *tester) ok(name string) bool {
	for _, c := range t.checks {
		if c.Name == name {
			return c.Outcome == models.ShadowCallOK
		}
	}
	return false
}

func (t *tester) info(required ...string) models.ShadowConnectionTestInfo {
	ok := true
	for _, r := range required {
		ok = ok && t.ok(r)
	}
	return models.ShadowConnectionTestInfo{OK: ok, Checks: t.checks}
}

// subscriptionAndAccounts runs the sign-in/subscriptions and accounts
// checks and returns the first subscription and its accounts of kinds.
func (t *tester) subscriptionAndAccounts(ctx context.Context, arm *armClient, conn sa.Connection, kinds, unlocks []string) (string, []*account) {
	sub := ""
	if conn.Config.AllSubscriptions {
		t.run(checkSignIn, unlocks, false, "", func() (int, error) {
			var p armPage[subscription]
			err := arm.client.GetJSON(ctx, arm.url("/subscriptions", qp{"api-version", apiSubscriptions}), &p)
			for _, s := range p.Value {
				if sub == "" && s.SubscriptionID != "" && strings.EqualFold(s.State, "Enabled") {
					sub = s.SubscriptionID
				}
			}
			if err == nil && sub == "" {
				return 0, &sa.CallError{Outcome: models.ShadowCallNotFound, Message: "the app can read no enabled subscription; assign it a role on the subscriptions to scan"}
			}
			return len(p.Value), err
		})
	} else {
		for _, s := range conn.Config.SubscriptionIDs {
			if s = strings.TrimSpace(s); s != "" {
				sub = s
				break
			}
		}
	}
	var accs []*account
	t.run(checkAccounts, unlocks, sub == "", "no subscription to read", func() (int, error) {
		// Page until an account of kinds is found (bounded).
		u := arm.url("/subscriptions/"+sub+"/providers/Microsoft.CognitiveServices/accounts", qp{"api-version", apiCognitive})
		n, _, err := armList(ctx, arm, u, testMaxPages, func(a account) {
			if len(accs) > 0 || a.ID == "" {
				return
			}
			for _, k := range kinds {
				if strings.EqualFold(a.Kind, k) {
					a.subscriptionID = sub
					accs = append(accs, &a)
					return
				}
			}
		})
		return n, err
	})
	return sub, accs
}

// Connection test check names shared by the Foundry checks.
const (
	checkSignIn   = "Microsoft Entra sign-in and subscriptions"
	checkAccounts = "Cognitive Services accounts (Accounts_List)"
)

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
