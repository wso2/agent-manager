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

package modhost

import (
	"strings"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

func mf(name string, requires, provides []string) modkit.Manifest {
	return modkit.Manifest{Name: name, Version: "1.0.0", CoreAPI: modkit.CoreAPIVersion, Requires: requires, Provides: provides}
}

var catalog = map[string]modkit.Manifest{
	"observability": mf("observability", nil, []string{"traces.query/v1"}),
	"evaluation":    mf("evaluation", []string{"traces.query/v1"}, []string{"eval.scores/v1"}),
	"reports":       mf("reports", []string{"eval.scores/v1"}, nil),
	"ai-gateway":    mf("ai-gateway", nil, []string{"llm.proxy/v1"}),
}

func TestPlanRequiresDependency(t *testing.T) {
	_, err := Plan("evaluation", catalog, map[string]string{}, false)
	if err == nil || !strings.Contains(err.Error(), "provided by observability") {
		t.Fatalf("want missing-dependency error, got %v", err)
	}
}

func TestPlanWithDepsOrdersProvidersFirst(t *testing.T) {
	steps, err := Plan("reports", catalog, map[string]string{}, true)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range steps {
		got = append(got, s.Action+":"+s.Module)
	}
	want := "install:observability install:evaluation install:reports"
	if strings.Join(got, " ") != want {
		t.Fatalf("got %v, want %s", got, want)
	}
}

func TestPlanEnablesDisabledProvider(t *testing.T) {
	steps, err := Plan("evaluation", catalog, map[string]string{"observability": StateDisabled}, true)
	if err != nil {
		t.Fatal(err)
	}
	if steps[0] != (Step{Module: "observability", Action: "enable"}) {
		t.Fatalf("got %v", steps)
	}
}

func TestPlanSatisfiedByActiveProvider(t *testing.T) {
	steps, err := Plan("evaluation", catalog, map[string]string{"observability": StateActive}, false)
	if err != nil || len(steps) != 1 {
		t.Fatalf("got %v, %v", steps, err)
	}
}

func TestPlanRejectsOtherCoreAPI(t *testing.T) {
	c := map[string]modkit.Manifest{"old": {Name: "old", Version: "0.1.0", CoreAPI: modkit.CoreAPIVersion + 1}}
	if _, err := Plan("old", c, nil, false); err == nil || !strings.Contains(err.Error(), "core API") {
		t.Fatalf("want core API error, got %v", err)
	}
}

func TestPlanNotCompiledIn(t *testing.T) {
	if _, err := Plan("nope", catalog, nil, false); err == nil || !strings.Contains(err.Error(), "not compiled") {
		t.Fatalf("got %v", err)
	}
}

func TestPlanAmbiguousProvider(t *testing.T) {
	c := map[string]modkit.Manifest{
		"a":        mf("a", nil, []string{"x/v1"}),
		"b":        mf("b", nil, []string{"x/v1"}),
		"consumer": mf("consumer", []string{"x/v1"}, nil),
	}
	if _, err := Plan("consumer", c, nil, true); err == nil || !strings.Contains(err.Error(), "several modules") {
		t.Fatalf("got %v", err)
	}
}

func TestPlanDetectsCycle(t *testing.T) {
	c := map[string]modkit.Manifest{
		"a": mf("a", []string{"b/v1"}, []string{"a/v1"}),
		"b": mf("b", []string{"a/v1"}, []string{"b/v1"}),
	}
	if _, err := Plan("a", c, nil, true); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("got %v", err)
	}
}

func TestDependents(t *testing.T) {
	states := map[string]string{"observability": StateActive, "evaluation": StateDisabled}
	if d := Dependents("observability", catalog, states, false); len(d) != 0 {
		t.Fatalf("disabled consumer must not block disable, got %v", d)
	}
	if d := Dependents("observability", catalog, states, true); len(d) != 1 || d[0] != "evaluation" {
		t.Fatalf("disabled consumer must block uninstall, got %v", d)
	}
}

func TestActivationOrder(t *testing.T) {
	got := ActivationOrder([]string{"reports", "evaluation", "observability"}, catalog)
	if strings.Join(got, ",") != "observability,evaluation,reports" {
		t.Fatalf("got %v", got)
	}
}
