//go:build integration

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

package inhouse

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	secretmanagersvc "github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type refStore struct {
	refs    map[string]occlient.CreateSecretReferenceRequest
	deleted []string
}

func newTestClient(t *testing.T) (*Client, *refStore) {
	t.Helper()
	store := &refStore{refs: map[string]occlient.CreateSecretReferenceRequest{}}
	oc := &clientmocks.OpenChoreoClientMock{
		CreateSecretReferenceFunc: func(_ context.Context, _ string, req occlient.CreateSecretReferenceRequest) (*occlient.SecretReferenceInfo, error) {
			if _, ok := store.refs[req.Name]; ok {
				return nil, utils.ErrConflict
			}
			store.refs[req.Name] = req
			return &occlient.SecretReferenceInfo{Name: req.Name}, nil
		},
		UpdateSecretReferenceFunc: func(_ context.Context, _ string, name string, req occlient.CreateSecretReferenceRequest) (*occlient.SecretReferenceInfo, error) {
			store.refs[name] = req
			return &occlient.SecretReferenceInfo{Name: name}, nil
		},
		DeleteSecretReferenceFunc: func(_ context.Context, _ string, name string) error {
			store.deleted = append(store.deleted, name)
			delete(store.refs, name)
			return nil
		},
	}
	c, err := NewClient(&secretmanagersvc.InHouseConfig{
		DB:              db.GetDB(),
		EncryptionKey:   bytes.Repeat([]byte{3}, 32),
		Client:          oc,
		RefreshInterval: "1h",
	})
	if err != nil {
		t.Fatal(err)
	}
	return c, store
}

func TestInHouseSecretLifecycle(t *testing.T) {
	ctx := context.Background()
	c, refs := newTestClient(t)
	ou := "ou-" + uuid.NewString()
	loc := secretmanagersvc.SecretLocation{OrgName: ou, ProjectName: "p", AgentName: "a", EnvironmentName: "dev", EntityName: "agent"}
	meta := &secretmanagersvc.SecretMetadata{ManagedBy: secretmanagersvc.DefaultManagedBy}
	t.Cleanup(func() { db.GetDB().Exec("DELETE FROM secrets WHERE ou_id = ?", ou) })

	name, err := c.PushSecret(ctx, loc, map[string]string{"B": "2", "A": "1"}, meta)
	if err != nil {
		t.Fatal(err)
	}
	ref := refs.refs[name]
	if ref.KVPath != RemoteKey(ou, name) || len(ref.SecretKeys) != 2 || ref.SecretKeys[0] != "A" || ref.RefreshInterval != "1h" {
		t.Fatalf("unexpected SecretReference: %+v", ref)
	}

	// Stored encrypted, not in plaintext.
	var ciphertext []byte
	db.GetDB().Raw("SELECT ciphertext FROM secrets WHERE ou_id = ? AND name = ?", ou, name).Scan(&ciphertext)
	if len(ciphertext) == 0 || bytes.Contains(ciphertext, []byte(`"A":"1"`)) {
		t.Fatal("secret is not stored encrypted")
	}

	if _, err := c.PatchSecret(ctx, loc, map[string]string{"C": "3"}, []string{"B"}, meta); err != nil {
		t.Fatal(err)
	}
	data, err := c.GetSecretData(ctx, ou, name)
	if err != nil || len(data) != 2 || data["A"] != "1" || data["C"] != "3" {
		t.Fatalf("after patch: %v (%v)", data, err)
	}
	if got := refs.refs[name].SecretKeys; len(got) != 2 || got[0] != "A" || got[1] != "C" {
		t.Fatalf("SecretReference keys after patch: %v", got)
	}

	if v, err := c.Resolve(ctx, ou, name, "C"); err != nil || v != "3" {
		t.Fatalf("Resolve = %q (%v)", v, err)
	}
	if _, err := c.Resolve(ctx, ou, name, "B"); !errors.Is(err, secretmanagersvc.ErrSecretNotFound) {
		t.Fatalf("Resolve of a removed key: %v", err)
	}

	info, err := c.GetSecret(ctx, loc)
	if err != nil || info.Labels[secretmanagersvc.LabelKeyManagedBy] != secretmanagersvc.DefaultManagedBy {
		t.Fatalf("GetSecret = %+v (%v)", info, err)
	}

	// Another owner can neither overwrite nor delete it.
	other := &secretmanagersvc.SecretMetadata{ManagedBy: "someone-else"}
	if _, err := c.PushSecret(ctx, loc, map[string]string{"A": "x"}, other); !errors.Is(err, secretmanagersvc.ErrNotManaged) {
		t.Fatalf("push by another owner: %v", err)
	}
	if err := c.DeleteSecret(ctx, loc, other); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetSecret(ctx, loc); err != nil {
		t.Fatal("secret was deleted by another owner")
	}

	if err := c.DeleteSecret(ctx, loc, meta); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetSecret(ctx, loc); !errors.Is(err, secretmanagersvc.ErrSecretNotFound) {
		t.Fatalf("after delete: %v", err)
	}
	if len(refs.deleted) != 1 || refs.deleted[0] != name {
		t.Fatalf("SecretReference not deleted: %v", refs.deleted)
	}
	if err := c.DeleteSecret(ctx, loc, meta); err != nil {
		t.Fatalf("delete is not idempotent: %v", err)
	}
}

func TestInHousePatchMissingSecret(t *testing.T) {
	c, _ := newTestClient(t)
	loc := secretmanagersvc.SecretLocation{OrgName: "ou-" + uuid.NewString(), EntityName: "missing"}
	_, err := c.PatchSecret(context.Background(), loc, map[string]string{"A": "1"}, nil, &secretmanagersvc.SecretMetadata{ManagedBy: "m"})
	if !errors.Is(err, secretmanagersvc.ErrSecretNotFound) {
		t.Fatalf("PatchSecret on a missing secret: %v", err)
	}
}
