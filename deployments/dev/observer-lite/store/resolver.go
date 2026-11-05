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
	"fmt"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

const DefaultResolverTTL = 10 * time.Second

func openChoreoResource(resource string) k8sschema.GroupVersionResource {
	return k8sschema.GroupVersionResource{Group: "openchoreo.dev", Version: "v1alpha1", Resource: resource}
}

var (
	componentsGVR   = openChoreoResource("components")
	projectsGVR     = openChoreoResource("projects")
	environmentsGVR = openChoreoResource("environments")
)

type cacheKey struct {
	resource  string
	namespace string
	name      string
}

type cacheEntry struct {
	uid     string
	expires time.Time
}

// Resolver maps OpenChoreo resource names to UIDs. The cache is required: a
// filtered trace walk asks for the scope once per examined trace.
type Resolver struct {
	dyn dynamic.Interface
	ttl time.Duration
	now func() time.Time

	mu    sync.Mutex
	cache map[cacheKey]cacheEntry
}

func NewResolver(dyn dynamic.Interface, ttl time.Duration, now func() time.Time) *Resolver {
	return &Resolver{dyn: dyn, ttl: ttl, now: now, cache: map[cacheKey]cacheEntry{}}
}

// Scope returns ok=false with a nil error only when a named resource does not exist.
func (r *Resolver) Scope(ctx context.Context, s observer.ComponentSearchScope) (Scope, bool, error) {
	out := Scope{Namespace: s.Namespace}
	lookups := []struct {
		gvr  k8sschema.GroupVersionResource
		name *string
		uid  *string
	}{
		{projectsGVR, s.Project, &out.ProjectUID},
		{componentsGVR, s.Component, &out.ComponentUID},
		{environmentsGVR, s.Environment, &out.EnvironmentUID},
	}
	for _, l := range lookups {
		if l.name == nil {
			continue
		}
		uid, err := r.uid(ctx, l.gvr, s.Namespace, *l.name)
		if apierrors.IsNotFound(err) {
			return Scope{}, false, nil
		}
		if err != nil {
			return Scope{}, false, err
		}
		*l.uid = uid
	}
	return out, true, nil
}

func (r *Resolver) uid(ctx context.Context, gvr k8sschema.GroupVersionResource, namespace, name string) (string, error) {
	key := cacheKey{resource: gvr.Resource, namespace: namespace, name: name}
	r.mu.Lock()
	entry, hit := r.cache[key]
	r.mu.Unlock()
	if hit && r.now().Before(entry.expires) {
		return entry.uid, nil
	}

	obj, err := r.dyn.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", err
		}
		return "", fmt.Errorf("get %s %s/%s: %w", gvr.Resource, namespace, name, err)
	}
	uid := string(obj.GetUID())

	r.mu.Lock()
	r.cache[key] = cacheEntry{uid: uid, expires: r.now().Add(r.ttl)}
	r.mu.Unlock()
	return uid, nil
}
