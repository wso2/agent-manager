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
	"encoding/json"
	"errors"
	"fmt"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
)

// Secret env vars and file mounts are not handed to OpenChoreo as valueFrom
// references. OpenChoreo would resolve those through SecretReference resources,
// and secrets no longer have any. Instead each ReleaseBinding carries them in
// componentTypeEnvironmentConfigs as a list of direct secret store keys, from
// which the ComponentType renders one ExternalSecret against the in-house store.
//
// Inside this package nothing changes: bindings read through ocAPI show secret
// entries as workloadOverrides valueFrom references, exactly as before, and
// writes move them back out. Only the wire format differs.
const (
	bindingSecretEnvsField  = "secretEnvs"
	bindingSecretFilesField = "secretFiles"
)

// bindingSecretEnv is one secret env var as the ComponentType reads it.
type bindingSecretEnv struct {
	// Name is the container env var name.
	Name string `json:"name"`
	// Secret is the secret's name in the store.
	Secret string `json:"secret"`
	// Property is the key within that secret.
	Property string `json:"property"`
	// RemoteKey is the ExternalSecret remoteRef.key: "<ouID>/<secret>".
	RemoteKey string `json:"remoteKey"`
}

// bindingSecretFile is one secret file mount as the ComponentType reads it.
type bindingSecretFile struct {
	Key       string `json:"key"`
	MountPath string `json:"mountPath"`
	Secret    string `json:"secret"`
	Property  string `json:"property"`
	RemoteKey string `json:"remoteKey"`
}

type secretOuIDKey struct{}

// withSecretOuID records the organization whose secrets a binding write
// references. A write that carries secret entries fails without it, since the
// store key of a secret is scoped by organization.
func withSecretOuID(ctx context.Context, ouID string) context.Context {
	return context.WithValue(ctx, secretOuIDKey{}, ouID)
}

func secretOuIDFrom(ctx context.Context) string {
	ouID, _ := ctx.Value(secretOuIDKey{}).(string)
	return ouID
}

var errSecretOuIDMissing = errors.New("release binding write carries secret references but no organization")

// secretStoreKey is the in-house store key of a secret. It must match
// inhouse.RemoteKey, which this package cannot import.
func secretStoreKey(ouID, secretName string) string {
	return ouID + "/" + secretName
}

// ocAPI is the generated OpenChoreo client with ReleaseBinding secret entries
// translated on the way in and out. Every other call passes straight through.
type ocAPI struct {
	*gen.ClientWithResponses
}

func newOCAPI(c *gen.ClientWithResponses) *ocAPI {
	return &ocAPI{ClientWithResponses: c}
}

func (a *ocAPI) GetReleaseBindingWithResponse(ctx context.Context, namespaceName gen.NamespaceNameParam, releaseBindingName gen.ReleaseBindingNameParam, reqEditors ...gen.RequestEditorFn) (*gen.GetReleaseBindingResp, error) {
	resp, err := a.ClientWithResponses.GetReleaseBindingWithResponse(ctx, namespaceName, releaseBindingName, reqEditors...)
	if err == nil && resp != nil && resp.JSON200 != nil {
		decodeBindingSecrets(resp.JSON200.Spec)
	}
	return resp, err
}

func (a *ocAPI) ListReleaseBindingsWithResponse(ctx context.Context, namespaceName gen.NamespaceNameParam, params *gen.ListReleaseBindingsParams, reqEditors ...gen.RequestEditorFn) (*gen.ListReleaseBindingsResp, error) {
	resp, err := a.ClientWithResponses.ListReleaseBindingsWithResponse(ctx, namespaceName, params, reqEditors...)
	if err == nil && resp != nil && resp.JSON200 != nil {
		for i := range resp.JSON200.Items {
			decodeBindingSecrets(resp.JSON200.Items[i].Spec)
		}
	}
	return resp, err
}

func (a *ocAPI) CreateReleaseBindingWithResponse(ctx context.Context, namespaceName gen.NamespaceNameParam, body gen.CreateReleaseBindingJSONRequestBody, reqEditors ...gen.RequestEditorFn) (*gen.CreateReleaseBindingResp, error) {
	spec, err := encodeBindingSecrets(body.Spec, secretOuIDFrom(ctx))
	if err != nil {
		return nil, err
	}
	body.Spec = spec
	resp, err := a.ClientWithResponses.CreateReleaseBindingWithResponse(ctx, namespaceName, body, reqEditors...)
	if err == nil && resp != nil && resp.JSON201 != nil {
		decodeBindingSecrets(resp.JSON201.Spec)
	}
	return resp, err
}

func (a *ocAPI) UpdateReleaseBindingWithResponse(ctx context.Context, namespaceName gen.NamespaceNameParam, releaseBindingName gen.ReleaseBindingNameParam, body gen.UpdateReleaseBindingJSONRequestBody, reqEditors ...gen.RequestEditorFn) (*gen.UpdateReleaseBindingResp, error) {
	spec, err := encodeBindingSecrets(body.Spec, secretOuIDFrom(ctx))
	if err != nil {
		return nil, err
	}
	body.Spec = spec
	resp, err := a.ClientWithResponses.UpdateReleaseBindingWithResponse(ctx, namespaceName, releaseBindingName, body, reqEditors...)
	if err == nil && resp != nil && resp.JSON200 != nil {
		decodeBindingSecrets(resp.JSON200.Spec)
	}
	return resp, err
}

func secretRef(from *gen.EnvVarValueFrom) (name, key string, ok bool) {
	if from == nil || from.SecretKeyRef == nil || from.SecretKeyRef.Name == nil || from.SecretKeyRef.Key == nil {
		return "", "", false
	}
	return *from.SecretKeyRef.Name, *from.SecretKeyRef.Key, true
}

func secretValueFrom(name, key string) *gen.EnvVarValueFrom {
	return &gen.EnvVarValueFrom{SecretKeyRef: &struct {
		Key  *string `json:"key,omitempty"`
		Name *string `json:"name,omitempty"`
	}{Name: &name, Key: &key}}
}

// encodeBindingSecrets returns a copy of spec with every secret env var and
// file mount moved from workloadOverrides into componentTypeEnvironmentConfigs.
// The caller's spec is left as it was, so a retried write encodes it again.
// The overrides are the source of truth: an entry they no longer hold is gone
// from the configs too, and a binding without any secrets drops both fields.
func encodeBindingSecrets(spec *gen.ReleaseBindingSpec, ouID string) (*gen.ReleaseBindingSpec, error) {
	if spec == nil {
		return spec, nil // a binding without a spec has nothing to move
	}
	out, err := cloneBindingSpec(spec)
	if err != nil {
		return nil, err
	}
	var envs []bindingSecretEnv
	var files []bindingSecretFile
	if out.WorkloadOverrides != nil && out.WorkloadOverrides.Container != nil {
		container := out.WorkloadOverrides.Container
		if container.Env != nil {
			plain := make([]gen.EnvVar, 0, len(*container.Env))
			for _, e := range *container.Env {
				name, key, ok := secretRef(e.ValueFrom)
				if !ok {
					plain = append(plain, e)
					continue
				}
				envs = append(envs, bindingSecretEnv{Name: e.Key, Secret: name, Property: key})
			}
			container.Env = &plain
		}
		if container.Files != nil {
			plain := make([]gen.FileVar, 0, len(*container.Files))
			for _, f := range *container.Files {
				name, key, ok := secretRef(f.ValueFrom)
				if !ok {
					plain = append(plain, f)
					continue
				}
				files = append(files, bindingSecretFile{Key: f.Key, MountPath: f.MountPath, Secret: name, Property: key})
			}
			container.Files = &plain
		}
	}
	if (len(envs) > 0 || len(files) > 0) && ouID == "" {
		return nil, errSecretOuIDMissing
	}
	for i := range envs {
		envs[i].RemoteKey = secretStoreKey(ouID, envs[i].Secret)
	}
	for i := range files {
		files[i].RemoteKey = secretStoreKey(ouID, files[i].Secret)
	}

	configs := map[string]interface{}{}
	if out.ComponentTypeEnvironmentConfigs != nil {
		configs = *out.ComponentTypeEnvironmentConfigs
	}
	if err := setConfigList(configs, bindingSecretEnvsField, envs); err != nil {
		return nil, err
	}
	if err := setConfigList(configs, bindingSecretFilesField, files); err != nil {
		return nil, err
	}
	if len(configs) > 0 || out.ComponentTypeEnvironmentConfigs != nil {
		out.ComponentTypeEnvironmentConfigs = &configs
	}
	return out, nil
}

// setConfigList stores list under field as plain JSON values, or removes the
// field when list is empty. ComponentTypes that declare no secrets never see it.
func setConfigList[T any](configs map[string]interface{}, field string, list []T) error {
	if len(list) == 0 {
		delete(configs, field)
		return nil
	}
	raw, err := json.Marshal(list)
	if err != nil {
		return fmt.Errorf("encode %s: %w", field, err)
	}
	var generic []interface{}
	if err := json.Unmarshal(raw, &generic); err != nil {
		return fmt.Errorf("encode %s: %w", field, err)
	}
	configs[field] = generic
	return nil
}

// decodeBindingSecrets moves the configs' secret entries back into the
// workloadOverrides as valueFrom references, in place.
func decodeBindingSecrets(spec *gen.ReleaseBindingSpec) {
	if spec == nil || spec.ComponentTypeEnvironmentConfigs == nil {
		return
	}
	configs := *spec.ComponentTypeEnvironmentConfigs
	envs := readConfigList[bindingSecretEnv](configs, bindingSecretEnvsField)
	files := readConfigList[bindingSecretFile](configs, bindingSecretFilesField)
	delete(configs, bindingSecretEnvsField)
	delete(configs, bindingSecretFilesField)
	if len(envs) == 0 && len(files) == 0 {
		return
	}
	if spec.WorkloadOverrides == nil {
		spec.WorkloadOverrides = &gen.WorkloadOverrides{}
	}
	if spec.WorkloadOverrides.Container == nil {
		spec.WorkloadOverrides.Container = &gen.ContainerOverride{}
	}
	container := spec.WorkloadOverrides.Container
	if len(envs) > 0 {
		var env []gen.EnvVar
		if container.Env != nil {
			env = *container.Env
		}
		for _, e := range envs {
			env = append(env, gen.EnvVar{Key: e.Name, ValueFrom: secretValueFrom(e.Secret, e.Property)})
		}
		container.Env = &env
	}
	if len(files) > 0 {
		var fv []gen.FileVar
		if container.Files != nil {
			fv = *container.Files
		}
		for _, f := range files {
			fv = append(fv, gen.FileVar{Key: f.Key, MountPath: f.MountPath, ValueFrom: secretValueFrom(f.Secret, f.Property)})
		}
		container.Files = &fv
	}
}

func readConfigList[T any](configs map[string]interface{}, field string) []T {
	raw, ok := configs[field]
	if !ok || raw == nil {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var list []T
	if err := json.Unmarshal(data, &list); err != nil {
		return nil
	}
	return list
}

func cloneBindingSpec(spec *gen.ReleaseBindingSpec) (*gen.ReleaseBindingSpec, error) {
	data, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("copy release binding: %w", err)
	}
	var out gen.ReleaseBindingSpec
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("copy release binding: %w", err)
	}
	return &out, nil
}
