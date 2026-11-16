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

// Package shadowagents is the provider-facing half of Shadow Agent Discovery.
// Each platform has a Connector in a sub-package that calls the platform's
// read-only agent listing APIs (OpenAI and Anthropic agents, Amazon Bedrock
// AgentCore runtimes, Agent Registry records and classic agents, Google
// Agent Runtime engines and Agent Registry agents, Microsoft Foundry agents)
// and reports each agent as a provider-neutral Finding, with the evidence
// that lists it. This package holds what they share: the types, the evidence
// catalog, the forms, the HTTP client and the cloud credentials, and the
// retention sanitiser everything passes before storage.
//
// Data retention: a connector never returns a secret, a prompt, an
// instruction text or a tool definition body. See
// docs/shadow-agent-discovery.md.
package shadowagents

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// Connection is a connection ready to scan: its settings and its decrypted
// secret. It lives only in memory while a connector runs.
type Connection struct {
	ID       uuid.UUID
	OUID     string
	Name     string
	Provider string
	Config   models.ShadowConnectionConfig
	Secret   models.ShadowConnectionSecret
}

// Connector scans one platform. Implementations must be safe for concurrent
// use by several scans, each with its own Connection.
type Connector interface {
	// Provider is the platform this connector scans (models.ShadowProvider*).
	Provider() string
	// Scan lists the agents the connection's settings reach. A failing call
	// never fails the scan: it is recorded in the result's calls and the
	// scan goes on with what it has.
	Scan(ctx context.Context, conn Connection) *Result
	// Test checks the credential with one cheap read per listing and says
	// which evidence each listing provides.
	Test(ctx context.Context, conn Connection) models.ShadowConnectionTestInfo
}

// Result is what one connection's scan found.
type Result struct {
	Findings []*Finding
	Calls    []models.ShadowScanCallResult
}

// Finding is an agent as a connector sees it. ExternalID is the platform's
// own ID of the agent (an agent ID, a runtime ARN, an engine resource name),
// stable across scans.
type Finding struct {
	Kind        string
	ExternalID  string
	DisplayName string
	// Scope is where the agent lives at the platform: a project, workspace,
	// region or location.
	Scope string
	// State is the agent's own state as the platform reports it (READY,
	// enabled, archived, ...).
	State string
	// OwnerHint is who created the agent, only when the agent listing itself
	// says so.
	OwnerHint         models.ShadowOwnerHint
	Attributes        map[string]any
	ProviderCreatedAt *time.Time
	// Signals are the evidence: the listing calls that returned the agent.
	Signals []Signal
}

// Signal is one piece of evidence: an evidence code from the catalog, the
// call that returned it and what was observed (flags and short labels only).
type Signal struct {
	Code string
	// Source names the call that produced it, e.g. "GET /v1/agents".
	Source string
	// Observed holds counts, flags and short labels only, e.g. {"status": "READY"}.
	Observed map[string]any
}

// AddSignal adds evidence, merging it with the same code from the same call.
func (f *Finding) AddSignal(s Signal) {
	for i := range f.Signals {
		if f.Signals[i].Code == s.Code && f.Signals[i].Source == s.Source {
			f.Signals[i].Observed = mergeObserved(f.Signals[i].Observed, s.Observed)
			return
		}
	}
	if s.Observed == nil {
		s.Observed = map[string]any{}
	}
	f.Signals = append(f.Signals, s)
}

// mergeObserved adds b to a: counts are summed, other values kept from a.
func mergeObserved(a, b map[string]any) map[string]any {
	if a == nil {
		a = map[string]any{}
	}
	for k, v := range b {
		old, ok := a[k]
		if !ok {
			a[k] = v
			continue
		}
		if x, ok1 := toInt64(old); ok1 {
			if y, ok2 := toInt64(v); ok2 {
				a[k] = x + y
			}
		}
	}
	return a
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case int32:
		return int64(n), true
	default:
		return 0, false
	}
}

// Findings indexes a scan's findings by kind and external ID, so evidence
// about the same agent from different calls lands on one finding.
type Findings struct {
	order []*Finding
	byKey map[string]*Finding
}

// NewFindings returns an empty index.
func NewFindings() *Findings {
	return &Findings{byKey: map[string]*Finding{}}
}

// Get returns the finding of (kind, externalID), creating it the first time.
func (fs *Findings) Get(kind, externalID string) *Finding {
	k := kind + "\x00" + externalID
	if f, ok := fs.byKey[k]; ok {
		return f
	}
	f := &Finding{Kind: kind, ExternalID: externalID, Attributes: map[string]any{}}
	fs.byKey[k] = f
	fs.order = append(fs.order, f)
	return f
}

// Lookup returns the finding of (kind, externalID) if it exists.
func (fs *Findings) Lookup(kind, externalID string) (*Finding, bool) {
	f, ok := fs.byKey[kind+"\x00"+externalID]
	return f, ok
}

// Remove drops a finding, e.g. an AgentCore runtime that turns out to host
// an MCP server rather than an agent.
func (fs *Findings) Remove(kind, externalID string) {
	k := kind + "\x00" + externalID
	f, ok := fs.byKey[k]
	if !ok {
		return
	}
	delete(fs.byKey, k)
	for i, o := range fs.order {
		if o == f {
			fs.order = append(fs.order[:i], fs.order[i+1:]...)
			break
		}
	}
}

// All returns the findings in the order they were first seen.
func (fs *Findings) All() []*Finding {
	out := make([]*Finding, len(fs.order))
	copy(out, fs.order)
	return out
}
