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
package store

import (
	"context"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

const testNamespace = "default"

func ocObject(kind, name, uid string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "openchoreo.dev/v1alpha1",
		"kind":       kind,
		"metadata":   map[string]any{"name": name, "namespace": testNamespace, "uid": uid},
	}}
}

func newFakeDynamic(objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
	gvr := func(resource string) k8sschema.GroupVersionResource {
		return k8sschema.GroupVersionResource{Group: "openchoreo.dev", Version: "v1alpha1", Resource: resource}
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[k8sschema.GroupVersionResource]string{
		gvr("components"):   "ComponentList",
		gvr("projects"):     "ProjectList",
		gvr("environments"): "EnvironmentList",
	}, objs...)
}

func strPtr(s string) *string { return &s }

func scopeOf(project, component, environment string) observer.ComponentSearchScope {
	return observer.ComponentSearchScope{
		Namespace:   testNamespace,
		Project:     strPtr(project),
		Component:   strPtr(component),
		Environment: strPtr(environment),
	}
}

func countGets(dyn *dynamicfake.FakeDynamicClient, resource string) *int {
	n := 0
	dyn.PrependReactor("get", resource, func(k8stesting.Action) (bool, runtime.Object, error) {
		n++
		return false, nil, nil
	})
	return &n
}

func TestResolverResolvesAllNames(t *testing.T) {
	dyn := newFakeDynamic(ocObject("Component", "a", "c-uid"), ocObject("Project", "p", "p-uid"), ocObject("Environment", "e", "e-uid"))
	r := NewResolver(dyn, DefaultResolverTTL, time.Now)

	got, ok, err := r.Scope(context.Background(), scopeOf("p", "a", "e"))
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	want := Scope{Namespace: testNamespace, ProjectUID: "p-uid", ComponentUID: "c-uid", EnvironmentUID: "e-uid"}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestResolverNilNamesLeaveFieldsEmpty(t *testing.T) {
	r := NewResolver(newFakeDynamic(), DefaultResolverTTL, time.Now)

	got, ok, err := r.Scope(context.Background(), observer.ComponentSearchScope{Namespace: testNamespace})
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if got != (Scope{Namespace: testNamespace}) {
		t.Fatalf("got %+v", got)
	}
}

func TestResolverUnknownName(t *testing.T) {
	dyn := newFakeDynamic(ocObject("Project", "p", "p-uid"), ocObject("Environment", "e", "e-uid"))
	r := NewResolver(dyn, DefaultResolverTTL, time.Now)

	_, ok, err := r.Scope(context.Background(), scopeOf("p", "missing", "e"))
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v, want ok=false err=nil", ok, err)
	}
}

func TestResolverSurfacesForbidden(t *testing.T) {
	dyn := newFakeDynamic(ocObject("Project", "p", "p-uid"), ocObject("Environment", "e", "e-uid"))
	dyn.PrependReactor("get", "components", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(k8sschema.GroupResource{Group: "openchoreo.dev", Resource: "components"}, "a", nil)
	})
	r := NewResolver(dyn, DefaultResolverTTL, time.Now)

	_, ok, err := r.Scope(context.Background(), scopeOf("p", "a", "e"))
	if err == nil || ok {
		t.Fatalf("ok=%v err=%v, want an error", ok, err)
	}
}

func TestResolverCachesAcrossScopes(t *testing.T) {
	dyn := newFakeDynamic(
		ocObject("Component", "a", "ca"), ocObject("Component", "b", "cb"),
		ocObject("Project", "p", "p-uid"), ocObject("Environment", "e", "e-uid"))
	projectGets, envGets := countGets(dyn, "projects"), countGets(dyn, "environments")
	r := NewResolver(dyn, DefaultResolverTTL, time.Now)

	for _, component := range []string{"a", "b"} {
		if _, ok, err := r.Scope(context.Background(), scopeOf("p", component, "e")); err != nil || !ok {
			t.Fatalf("component %s: ok=%v err=%v", component, ok, err)
		}
	}
	if *projectGets != 1 || *envGets != 1 {
		t.Fatalf("project gets=%d, environment gets=%d, want 1 each", *projectGets, *envGets)
	}
}

func TestResolverDoesNotCacheNotFound(t *testing.T) {
	dyn := newFakeDynamic(ocObject("Project", "p", "p-uid"), ocObject("Environment", "e", "e-uid"))
	r := NewResolver(dyn, DefaultResolverTTL, time.Now)
	if _, ok, _ := r.Scope(context.Background(), scopeOf("p", "a", "e")); ok {
		t.Fatal("component should not exist yet")
	}

	created := ocObject("Component", "a", "c-uid")
	if _, err := dyn.Resource(componentsGVR).Namespace(testNamespace).Create(context.Background(), created, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := r.Scope(context.Background(), scopeOf("p", "a", "e"))
	if err != nil || !ok || got.ComponentUID != "c-uid" {
		t.Fatalf("got %+v ok=%v err=%v", got, ok, err)
	}
}

func TestResolverCacheExpires(t *testing.T) {
	dyn := newFakeDynamic(ocObject("Component", "a", "c1"), ocObject("Project", "p", "p-uid"), ocObject("Environment", "e", "e-uid"))
	clock := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	r := NewResolver(dyn, DefaultResolverTTL, func() time.Time { return clock })

	first, _, err := r.Scope(context.Background(), scopeOf("p", "a", "e"))
	if err != nil || first.ComponentUID != "c1" {
		t.Fatalf("first=%+v err=%v", first, err)
	}

	components := dyn.Resource(componentsGVR).Namespace(testNamespace)
	if err := components.Delete(context.Background(), "a", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := components.Create(context.Background(), ocObject("Component", "a", "c2"), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	cached, _, _ := r.Scope(context.Background(), scopeOf("p", "a", "e"))
	if cached.ComponentUID != "c1" {
		t.Fatalf("within TTL got %s, want c1", cached.ComponentUID)
	}
	clock = clock.Add(11 * time.Second)
	refreshed, _, _ := r.Scope(context.Background(), scopeOf("p", "a", "e"))
	if refreshed.ComponentUID != "c2" {
		t.Fatalf("after TTL got %s, want c2", refreshed.ComponentUID)
	}
}
