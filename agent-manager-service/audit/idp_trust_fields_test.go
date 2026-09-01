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

package audit

import (
	"context"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// TestIdentityProviderTrustInputsAreRecorded pins the fields that decide which
// tokens a gateway will accept.
//
// The issuer alone does not answer that: jwksUri says where the signing keys
// are fetched from, and skipTlsVerify says whether that fetch validates the
// certificate. An operator pointing a trusted issuer at a new JWKS URI with TLS
// verification off is the change worth catching, and without these two keys the
// record showed only that the issuer was "updated".
func TestIdentityProviderTrustInputsAreRecorded(t *testing.T) {
	for _, action := range []Action{ActionGatewaySetIdentityProvider, ActionGatewayRemoveIdentityProvider} {
		e := Event{
			Action: action,
			Details: map[string]any{
				"identityProviderName": "corp-idp",
				"issuer":               "https://idp.example",
				"jwksUri":              "https://idp.example/.well-known/jwks.json",
				"skipTlsVerify":        true,
			},
		}

		redact(&e)

		for _, key := range []string{"identityProviderName", "issuer", "jwksUri", "skipTlsVerify"} {
			if _, ok := e.Details[key]; !ok {
				t.Errorf("%s: trust input %q was dropped; declare it in idpFields", action, key)
			}
		}
	}
}

// TestEnvelopeNamesAnActorTheHandlerAuthenticated is the regression for a
// coverage record that called an authenticated caller anonymous.
//
// On the internal gateway server there is no JWT: the audit Source is built
// before the handler runs, and the handler is what verifies the api-key. The
// envelope therefore recorded actorType "anonymous" with no id for a request
// that had authenticated — while the semantic record for the same surface,
// which passes an explicit Actor, named the gateway correctly. A reader could
// not tell which gateway pulled key material, which is the one question those
// records exist to answer.
func TestEnvelopeNamesAnActorTheHandlerAuthenticated(t *testing.T) {
	ctx := WithSource(context.Background(), Source{
		Surface: SurfaceInternal,
		IP:      "10.0.0.7",
		// No ActorID/ActorType/AuthMethod: this is what the middleware leaves
		// on a surface it cannot authenticate itself.
	})
	ctx, _ = NewRequestScope(ctx)

	before := BuildEvent(ctx, ActionAPIKeySync)
	if before.ActorType != ActorAnonymous || before.ActorID != "" {
		t.Fatalf("precondition: expected an unnamed actor before the handler runs, got %q/%q",
			before.ActorType, before.ActorID)
	}

	IdentifyActor(ctx, ActorGateway, "gw-42", "api-key")

	e := BuildEvent(ctx, ActionAPIKeySync)
	if e.ActorID != "gw-42" {
		t.Errorf("ActorID = %q, want the gateway the handler authenticated", e.ActorID)
	}
	if e.ActorType != ActorGateway {
		t.Errorf("ActorType = %q, want %q", e.ActorType, ActorGateway)
	}
	if e.AuthMethod != "api-key" {
		t.Errorf("AuthMethod = %q, want %q", e.AuthMethod, "api-key")
	}
}

// TestExplicitActorBeatsTheScope keeps precedence right: a semantic emit that
// names its own actor must not be overwritten by the scope.
func TestExplicitActorBeatsTheScope(t *testing.T) {
	ctx := WithSource(context.Background(), Source{Surface: SurfaceInternal})
	ctx, _ = NewRequestScope(ctx)
	// A different type from the explicit one below, so the assertion can tell
	// "the option won" from "both happened to agree".
	IdentifyActor(ctx, ActorService, "gw-scope", "api-key")

	e := BuildEvent(ctx, ActionGatewayPushManifest,
		Actor(ActorGateway, "gw-explicit", ""), AuthMethod("mtls"))

	if e.ActorID != "gw-explicit" {
		t.Errorf("ActorID = %q; an explicit Actor option must win over the scope", e.ActorID)
	}
	if e.ActorType != ActorGateway {
		t.Errorf("ActorType = %q, want %q; the scope must not override an explicit actor",
			e.ActorType, ActorGateway)
	}
	if e.AuthMethod != "mtls" {
		t.Errorf("AuthMethod = %q, want %q; the scope must not override an explicit one",
			e.AuthMethod, "mtls")
	}
}

// TestJWTActorIsNotClobberedByAnEmptyScope guards the ordinary API surface:
// nothing calls IdentifyActor there, and the token subject must survive.
func TestJWTActorIsNotClobberedByAnEmptyScope(t *testing.T) {
	ctx := WithSource(context.Background(), Source{
		Surface: SurfaceAPI, ActorID: "alice@example.com", ActorType: ActorUser, AuthMethod: "jwt-bearer",
	})
	ctx, _ = NewRequestScope(ctx)

	e := BuildEvent(ctx, ActionAPIKeySync)
	if e.ActorID != "alice@example.com" {
		t.Errorf("ActorID = %q; an unused scope must not clear it", e.ActorID)
	}
	if e.ActorType != ActorUser {
		t.Errorf("ActorType = %q, want %q; an unused scope must not clear it", e.ActorType, ActorUser)
	}
	if e.AuthMethod != "jwt-bearer" {
		t.Errorf("AuthMethod = %q, want %q; an unused scope must not clear it",
			e.AuthMethod, "jwt-bearer")
	}
}

// TestSemanticRecordsCarryTheGatingPermission is the regression for half a
// claim.
//
// The design says every record carries rbacEnforced "alongside the
// requiredPermission that would have applied". The coverage tier passed the
// permission explicitly, but a semantic emit does not know its route, so those
// records carried rbacEnforced:false and no permission at all.
//
// Found by running with RBAC_ENABLED=false: a git-secret:create that would have
// been refused recorded that no check happened, without saying which check —
// and semantic records are exactly the severity-4 credential and privilege
// operations where that matters most.
func TestSemanticRecordsCarryTheGatingPermission(t *testing.T) {
	ctx := WithSource(context.Background(), Source{
		Surface:            SurfaceAPI,
		Pattern:            "/orgs/{orgName}/git-secrets",
		Method:             "POST",
		RBACEnforced:       false,
		RequiredPermission: "amp:git-secret:create",
	})
	ctx, _ = NewRequestScope(ctx)

	e := BuildEvent(ctx, ActionGitSecretCreate)
	if e.RequiredPermission != "amp:git-secret:create" {
		t.Errorf("RequiredPermission = %q; a semantic record must say which check "+
			"would have applied, especially when rbacEnforced is false", e.RequiredPermission)
	}
	if e.RBACEnforced {
		t.Error("RBACEnforced should have come from the source")
	}
}

// TestScopesOfRendersPermissionsAsRecorded pins the shared rendering, since the
// middleware and the option must produce the same string for the same route.
func TestScopesOfRendersPermissionsAsRecorded(t *testing.T) {
	if got := ScopesOf(nil); got != "" {
		t.Errorf("ScopesOf(nil) = %q, want empty", got)
	}
	one := ScopesOf([]rbac.Permission{rbac.GitSecretCreate})
	if one != rbac.GitSecretCreate.Scope() {
		t.Errorf("ScopesOf(one) = %q, want %q", one, rbac.GitSecretCreate.Scope())
	}

	// The serialised form is asserted literally, not just for agreement between
	// the two paths: RequiredPermissions delegates to ScopesOf, so comparing
	// them is comparing a function to itself and would accept an empty or
	// malformed value from both. A multi-permission route records one
	// space-separated string, which is what a SIEM query has to match.
	many := []rbac.Permission{rbac.GitSecretCreate, rbac.GitSecretDelete}
	const wantMany = "amp:git-secret:create amp:git-secret:delete"

	viaHelper := ScopesOf(many)
	if viaHelper != wantMany {
		t.Errorf("ScopesOf(many) = %q, want %q", viaHelper, wantMany)
	}

	viaOption := Event{}
	RequiredPermissions(many...)(&viaOption)
	if viaOption.RequiredPermission != wantMany {
		t.Errorf("RequiredPermissions(many) recorded %q, want %q",
			viaOption.RequiredPermission, wantMany)
	}

	// Kept so the two stay tied together if one is ever reimplemented.
	if viaHelper != viaOption.RequiredPermission {
		t.Errorf("helper %q and option %q disagree; the same route would record "+
			"two different strings depending on which tier wrote it",
			viaHelper, viaOption.RequiredPermission)
	}
}
