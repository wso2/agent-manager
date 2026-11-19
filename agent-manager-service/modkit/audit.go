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

package modkit

import "context"

// AuditScope is the audit record of one API request, MCP tool call or
// internal-route request. The core puts it on the request's context; a module
// adds to it with AuditDetail, AuditResource and AuditInternalCaller.
type AuditScope interface {
	AuditDetail(key string, value any)
	AuditResource(kind, id, name string)
	AuditInternalCaller(ctx context.Context, c InternalCaller)
}

type auditScopeKey struct{}

// WithAuditScope returns ctx carrying s, for the core to set on requests.
func WithAuditScope(ctx context.Context, s AuditScope) context.Context {
	return context.WithValue(ctx, auditScopeKey{}, s)
}

func auditScope(ctx context.Context) AuditScope {
	s, _ := ctx.Value(auditScopeKey{}).(AuditScope)
	return s
}

// AuditDetail adds a field to the audit record of the current request or tool
// call. The key must be one the record's action declares in its
// AuditAction.Details; any other key is dropped when the record is written,
// and a URL detail loses its credentials and query. value is a string, bool,
// number or []string. Outside a request it does nothing.
func AuditDetail(ctx context.Context, key string, value any) {
	if s := auditScope(ctx); s != nil {
		s.AuditDetail(key, value)
	}
}

// AuditResource names what the current request or tool call acted on, by a
// kind of the module's ("gateway"), an id and a display name. Outside a
// request it does nothing.
func AuditResource(ctx context.Context, kind, id, name string) {
	if s := auditScope(ctx); s != nil {
		s.AuditResource(kind, id, name)
	}
}

// InternalCaller is the caller of a HandleInternalAt or HandleWebhook route,
// which the handler authenticates itself.
type InternalCaller struct {
	// Kind is the kind of caller ("gateway"), ID its id once known.
	Kind string
	ID   string
	// AuthMethod is how it authenticated ("api-key").
	AuthMethod string
	// Refused is set when the handler refused the caller: a short reason
	// ("invalid-api-key"), never the credential.
	Refused string
	// CredentialPresent is whether a refused request carried a credential.
	CredentialPresent bool
}

// AuditInternalCaller names the caller of a HandleInternalAt or HandleWebhook
// route once the handler authenticated it, so the route's record names it; a
// refused caller is recorded as an authentication failure, at most once a
// minute per source and reason.
func AuditInternalCaller(ctx context.Context, c InternalCaller) {
	if s := auditScope(ctx); s != nil {
		s.AuditInternalCaller(ctx, c)
	}
}
