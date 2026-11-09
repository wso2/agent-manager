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

package client

import (
	"context"
	"crypto/sha3"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	workloadSecretSyncTimeout      = 30 * time.Second
	workloadSecretSyncPollInterval = time.Second
)

// SyncEditedWorkloadSecrets refreshes live ExternalSecrets that already use an
// edited key, without changing the pod template. Existing snapshots without
// the force-sync annotation continue to use their periodic refresh.
func (c *openChoreoClient) SyncEditedWorkloadSecrets(ctx context.Context, ouID, componentName, envName string, editedKeys []string) error {
	ctx, cancel := context.WithTimeout(ctx, workloadSecretSyncTimeout)
	defer cancel()

	namespace := c.NamespaceFor(ouID)
	binding, err := c.findReleaseBindingForEnv(ctx, namespace, componentName, envName)
	if err != nil {
		return err
	}
	if binding == nil {
		return nil
	}
	bindingName := binding.Metadata.Name
	tree, err := c.workloadSecretTree(ctx, namespace, bindingName)
	if err != nil {
		return err
	}
	objects := forceSyncExternalSecrets(tree)
	if len(objects) == 0 {
		return nil
	}
	targets := make(map[string]string)
	for uid, obj := range objects {
		if referencesEditedSecret(obj, editedKeys) {
			name, _, _ := unstructured.NestedString(obj, "metadata", "name")
			targets[uid] = name
		}
	}
	if len(targets) == 0 {
		return nil
	}

	revision := uuid.NewString()
	if err := c.SetReleaseBindingSecretSyncRevision(ctx, ouID, componentName, envName, revision); err != nil {
		return fmt.Errorf("request workload secret sync: %w", err)
	}

	ticker := time.NewTicker(workloadSecretSyncPollInterval)
	defer ticker.Stop()
	var pending []string
	for {
		tree, err := c.workloadSecretTree(ctx, namespace, bindingName)
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return workloadSecretSyncTimeoutError(revision, pending)
			}
			return err
		}
		objects := forceSyncExternalSecrets(tree)
		pending = pending[:0]
		for uid, name := range targets {
			obj := objects[uid]
			if !externalSecretSyncedRevision(obj, revision) {
				pending = append(pending, workloadSecretSyncObservation(name, obj))
			}
		}
		if len(pending) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return workloadSecretSyncTimeoutError(revision, pending)
			}
			return fmt.Errorf("wait for workload secret sync: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

// workloadSecretSyncTimeoutError reports the last observations without provider messages.
func workloadSecretSyncTimeoutError(revision string, pending []string) error {
	slices.Sort(pending)
	return fmt.Errorf("wait for workload secret sync revision %q, pending ExternalSecrets %v: %w", revision, pending, context.DeadlineExceeded)
}

// workloadSecretSyncObservation reports safe status metadata for a pending target.
// ESO condition messages can include provider paths and are omitted.
func workloadSecretSyncObservation(name string, obj map[string]any) string {
	if obj == nil {
		return fmt.Sprintf("%q missing from resource tree", name)
	}
	revision, _, _ := unstructured.NestedString(obj, "metadata", "annotations", forceSyncAnnotation)
	ack, _, _ := unstructured.NestedString(obj, "status", "syncedResourceVersion")
	ready, reason := externalSecretReadyStatus(obj)
	return fmt.Sprintf("%q (force-sync=%q, ack=%q, Ready=%q, reason=%q)", name, revision, ack, ready, reason)
}

// workloadSecretTree reads the live resources applied by the binding.
func (c *openChoreoClient) workloadSecretTree(ctx context.Context, namespace, bindingName string) (*gen.K8sResourceTreeResponse, error) {
	resp, err := c.ocClient.GetReleaseBindingK8sResourceTreeWithResponse(ctx, namespace, bindingName)
	if err != nil {
		return nil, fmt.Errorf("read workload secret resource tree: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		})
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("empty workload secret resource tree")
	}
	return resp.JSON200, nil
}

// forceSyncExternalSecrets indexes live ExternalSecrets that support the sync annotation.
func forceSyncExternalSecrets(tree *gen.K8sResourceTreeResponse) map[string]map[string]any {
	objects := make(map[string]map[string]any)
	for _, rendered := range tree.RenderedReleases {
		for _, node := range rendered.Nodes {
			if node.Kind != "ExternalSecret" || node.Group == nil || *node.Group != "external-secrets.io" {
				continue
			}
			_, found, err := unstructured.NestedString(node.Object, "metadata", "annotations", forceSyncAnnotation)
			if err == nil && found {
				objects[node.Uid] = node.Object
			}
		}
	}
	return objects
}

// referencesEditedSecret reports whether the ExternalSecret writes an edited
// key. Its secretKey is the workload env var or file name, which Agent Manager
// also uses as the key in the agent's SecretReference, so no SecretReference
// read is needed.
func referencesEditedSecret(obj map[string]any, editedKeys []string) bool {
	data, _, _ := unstructured.NestedSlice(obj, "spec", "data")
	for _, item := range data {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		key, _, _ := unstructured.NestedString(entry, "secretKey")
		if slices.Contains(editedKeys, key) {
			return true
		}
	}
	return false
}

// externalSecretSyncedRevision compares ESO's acknowledgement with the current
// metadata. ESO 1.3.2 and 2.0.1 use generation plus a SHA3-224 hash of annotations
// and labels. A Ready condition from the previous revision cannot pass this check.
func externalSecretSyncedRevision(obj map[string]any, revision string) bool {
	annotations, _, err := unstructured.NestedStringMap(obj, "metadata", "annotations")
	if err != nil || annotations[forceSyncAnnotation] != revision {
		return false
	}
	labels, _, err := unstructured.NestedStringMap(obj, "metadata", "labels")
	if err != nil {
		return false
	}
	generation, found, err := unstructured.NestedFieldNoCopy(obj, "metadata", "generation")
	if err != nil || !found {
		return false
	}
	// ObjectHash uses Go's %+v representation, including these field names.
	meta := struct {
		annotations map[string]string
		labels      map[string]string
	}{annotations: annotations, labels: labels}
	hash := sha3.Sum224([]byte(fmt.Sprintf("%+v", meta)))
	synced, _, _ := unstructured.NestedString(obj, "status", "syncedResourceVersion")
	if synced != fmt.Sprintf("%v-%x", generation, hash) {
		return false
	}
	ready, reason := externalSecretReadyStatus(obj)
	return ready == "True" && reason == "SecretSynced"
}

// externalSecretReadyStatus extracts the Ready status and machine-readable reason.
func externalSecretReadyStatus(obj map[string]any) (string, string) {
	conditions, _, _ := unstructured.NestedSlice(obj, "status", "conditions")
	for _, item := range conditions {
		condition, ok := item.(map[string]any)
		if ok && condition["type"] == "Ready" {
			status, _ := condition["status"].(string)
			reason, _ := condition["reason"].(string)
			return status, reason
		}
	}
	return "", ""
}
