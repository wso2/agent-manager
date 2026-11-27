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

package tokens

import (
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

var _ = Describe("SEC-AUTH-001: token rejection", Label("security"), func() {
	// forged tokens — built from scratch, no genuine signature involved.
	DescribeTable("rejects a forged token with 401",
		func(ctx SpecContext, build func() (string, error)) {
			token, err := build()
			Expect(err).NotTo(HaveOccurred(), "failed to build the forged token")

			resp := get(ctx, token)
			defer resp.Body.Close()
			framework.ExpectUnauthorized(Default, resp, "forged token")
		},

		Entry("alg: none, claiming every scope", func() (string, error) {
			return framework.UnsignedToken(framework.PlausibleClaims(Cfg, nil))
		}),

		Entry("HS256 signed with an attacker-chosen key", func() (string, error) {
			return framework.HS256Token(framework.PlausibleClaims(Cfg, nil), "attacker-chosen-key")
		}),

		// The service must reject a symmetric algorithm on signing-method
		// grounds alone. If it did not, the public JWKS modulus could be used
		// as an HMAC key — the classic algorithm-confusion forgery.
		Entry("HS256 signed with the literal string 'secret'", func() (string, error) {
			return framework.HS256Token(framework.PlausibleClaims(Cfg, nil), "secret")
		}),

		Entry("alg: none with an expired exp", func() (string, error) {
			return framework.UnsignedToken(framework.PlausibleClaims(Cfg, map[string]any{
				"exp": time.Now().Add(-time.Hour).Unix(),
			}))
		}),

		Entry("alg: none with an audience the service does not allow", func() (string, error) {
			return framework.UnsignedToken(framework.PlausibleClaims(Cfg, map[string]any{
				"aud": []string{"some-other-resource-server"},
			}))
		}),

		Entry("alg: none with an untrusted issuer", func() (string, error) {
			return framework.UnsignedToken(framework.PlausibleClaims(Cfg, map[string]any{
				"iss": "https://attacker.example.com",
			}))
		}),
	)

	// tampered tokens — a genuine token whose bytes were edited. This is the
	// realistic attack: take the token you were legitimately issued and try to
	// widen it.
	DescribeTable("rejects a tampered genuine token with 401",
		func(ctx SpecContext, tamper func(genuine string) (string, error)) {
			token, err := tamper(Client.Token())
			Expect(err).NotTo(HaveOccurred(), "failed to tamper the genuine token")
			Expect(token).NotTo(Equal(Client.Token()), "the tamper produced an identical token")

			resp := get(ctx, token)
			defer resp.Body.Close()
			framework.ExpectUnauthorized(Default, resp, "tampered token")
		},

		// The headline case: escalate your own token by editing its scopes.
		Entry("payload rewritten to add amp:org:assign-role", func(genuine string) (string, error) {
			return framework.TamperClaims(genuine, func(claims map[string]any) {
				existing, _ := claims["scope"].(string)
				claims["scope"] = existing + " amp:org:assign-role"
			})
		}),

		Entry("payload rewritten to a different org", func(genuine string) (string, error) {
			return framework.TamperClaims(genuine, func(claims map[string]any) {
				claims["ouHandle"] = "some-other-org"
				claims["ouId"] = "some-other-ou-id"
			})
		}),

		Entry("payload rewritten to extend exp", func(genuine string) (string, error) {
			return framework.TamperClaims(genuine, func(claims map[string]any) {
				claims["exp"] = time.Now().Add(100 * 365 * 24 * time.Hour).Unix()
			})
		}),

		Entry("header rewritten to alg: none", func(genuine string) (string, error) {
			return framework.TamperHeader(genuine, func(header map[string]any) {
				header["alg"] = "none"
			})
		}),

		Entry("header rewritten to an unknown kid", func(genuine string) (string, error) {
			return framework.TamperHeader(genuine, func(header map[string]any) {
				header["kid"] = "sec-test-unknown-kid"
			})
		}),

		// A kid that fails the service's validKidPattern regex, which short
		// circuits before any JWKS refresh is attempted.
		Entry("header rewritten to a malformed kid", func(genuine string) (string, error) {
			return framework.TamperHeader(genuine, func(header map[string]any) {
				header["kid"] = "../../etc/passwd\x00"
			})
		}),

		Entry("signature replaced with garbage", func(genuine string) (string, error) {
			parts := strings.Split(genuine, ".")
			return parts[0] + "." + parts[1] + ".bm90LWEtc2lnbmF0dXJl", nil
		}),

		Entry("signature stripped entirely", func(genuine string) (string, error) {
			parts := strings.Split(genuine, ".")
			return parts[0] + "." + parts[1] + ".", nil
		}),
	)

	// malformed input — nothing resembling a JWT.
	DescribeTable("rejects malformed credentials with 401",
		func(ctx SpecContext, token string) {
			resp := get(ctx, token)
			defer resp.Body.Close()
			framework.ExpectUnauthorized(Default, resp, "malformed credential")
		},

		Entry("empty bearer value", ""),
		Entry("not a JWT at all", "this-is-not-a-token"),
		Entry("two segments only", "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ4In0"),
		Entry("four segments", "a.b.c.d"),
		Entry("segments that are not base64", "!!!.???.###"),
		Entry("only dots", ".."),
	)

	// Kept out of the table above because the correct answer is NOT 401. Go's
	// net/http rejects an oversized header with 431 before the handler runs, so
	// the authentication layer never parses the attacker-controlled bytes at
	// all. That is strictly better than a 401 — it is the outcome to assert.
	// The requirement is "refused without being authenticated", which both
	// codes satisfy; anything else means megabytes of attacker input reached
	// the JWT parser.
	It("refuses an oversized token without parsing it", func(ctx SpecContext) {
		resp := get(ctx, strings.Repeat("A", 100_000))
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(BeElementOf(
			http.StatusRequestHeaderFieldsTooLarge, http.StatusUnauthorized),
			"an oversized bearer token must be refused (431 at the HTTP layer, or 401 at the "+
				"auth layer), got %d", resp.StatusCode)
	})

	It("rejects a request with no Authorization header", func(ctx SpecContext) {
		resp, err := Client.GetUnauthenticatedWithContext(ctx, guardedPath)
		Expect(err).NotTo(HaveOccurred(), "request failed")
		defer resp.Body.Close()
		framework.ExpectUnauthorized(Default, resp, "no Authorization header")
	})

	// Accepting a token from the query string would leak credentials into
	// access logs, proxy logs, and browser history. The genuine token is used
	// here deliberately: the point is that even a VALID token must not
	// authenticate when presented this way.
	It("does not accept a valid token passed as a query parameter", func(ctx SpecContext) {
		path := guardedPath + "?access_token=" + Client.Token()
		resp, err := Client.GetUnauthenticatedWithContext(ctx, path)
		Expect(err).NotTo(HaveOccurred(), "request failed")
		defer resp.Body.Close()
		framework.ExpectUnauthorized(Default, resp, "token in query parameter")
	})
})
