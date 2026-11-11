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

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"k8s.io/apimachinery/pkg/util/validation"

	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

type openBaoImportEntry struct {
	OrganizationID string `json:"organizationId"`
	SecretName     string `json:"secretName"`
}

func runOpenBaoImport(input io.Reader, cfg config.Config, auth occlient.AuthProvider) error {
	entries, err := readOpenBaoImportManifest(input)
	if err != nil {
		return err
	}
	source, err := wiring.ProvideOCClient(cfg, auth)
	if err != nil {
		return err
	}
	target, err := wiring.ProvideInHouseSecretImporter(cfg, db.GetDB())
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	imported, unchanged, err := importOpenBaoSecrets(ctx, entries, source.ReadLegacyManagedSecret, target.ImportLegacySecret)
	slog.Info("OpenBao secret import finished", "imported", imported, "unchanged", unchanged, "requested", len(entries))
	return err
}

func readOpenBaoImportManifest(input io.Reader) ([]openBaoImportEntry, error) {
	var manifest struct {
		Secrets []openBaoImportEntry `json:"secrets"`
	}
	decoder := json.NewDecoder(input)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("invalid OpenBao import manifest")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("OpenBao import requires a single JSON manifest")
	}
	if len(manifest.Secrets) == 0 {
		return nil, fmt.Errorf("OpenBao import manifest contains no secrets")
	}
	seen := make(map[string]struct{}, len(manifest.Secrets))
	for i, entry := range manifest.Secrets {
		if entry.OrganizationID == "" || entry.OrganizationID == "platform" ||
			len(validation.IsValidLabelValue(entry.OrganizationID)) > 0 ||
			len(validation.IsDNS1123Subdomain(entry.SecretName)) > 0 {
			return nil, fmt.Errorf("manifest entry %d has an invalid organization ID or secret name", i+1)
		}
		// Legacy secrets share one source namespace. A source name has one owner.
		if _, exists := seen[entry.SecretName]; exists {
			return nil, fmt.Errorf("manifest repeats source secret %q", entry.SecretName)
		}
		seen[entry.SecretName] = struct{}{}
	}
	return manifest.Secrets, nil
}

func importOpenBaoSecrets(
	ctx context.Context,
	entries []openBaoImportEntry,
	read func(context.Context, string, string) (map[string]string, error),
	write func(context.Context, string, string, map[string]string) (bool, error),
) (imported, unchanged int, err error) {
	for _, entry := range entries {
		if ctx.Err() != nil {
			return imported, unchanged, errors.Join(err, ctx.Err())
		}
		data, readErr := read(ctx, entry.OrganizationID, entry.SecretName)
		if readErr != nil {
			err = errors.Join(err, fmt.Errorf("read %s/%s: %w", entry.OrganizationID, entry.SecretName, readErr))
			continue
		}
		created, writeErr := write(ctx, entry.OrganizationID, entry.SecretName, data)
		if writeErr != nil {
			err = errors.Join(err, fmt.Errorf("import %s/%s: %w", entry.OrganizationID, entry.SecretName, writeErr))
		} else if created {
			imported++
		} else {
			unchanged++
		}
	}
	return imported, unchanged, err
}
