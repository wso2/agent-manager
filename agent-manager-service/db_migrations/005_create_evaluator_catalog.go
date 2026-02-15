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

package dbmigrations

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gorm.io/gorm"
)

// EvaluatorDefinition represents an evaluator from the JSON seed file
// This structure matches the output of amp-evaluation's list_builtin_evaluators()
type EvaluatorDefinition struct {
	Name         string                      `json:"name"`
	Description  string                      `json:"description"`
	Tags         []string                    `json:"tags"`
	Version      string                      `json:"version"`
	ConfigSchema []map[string]interface{}    `json:"config_schema"`
	Metadata     EvaluatorDefinitionMetadata `json:"metadata"`
}

// EvaluatorDefinitionMetadata contains implementation details
type EvaluatorDefinitionMetadata struct {
	ClassName string `json:"class_name"`
	Module    string `json:"module"`
}

// Default path for builtin evaluators JSON file
const defaultBuiltinEvaluatorsPath = "data/builtin_evaluators.json"

// Create evaluator_catalog table and seed builtin evaluators from JSON file
var migration005 = migration{
	ID: 5,
	Migrate: func(db *gorm.DB) error {
		createEvaluatorCatalogTable := `
		CREATE TABLE IF NOT EXISTS evaluator_catalog (
			-- Identity
			id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			identifier      VARCHAR(255) NOT NULL,
			display_name    VARCHAR(255) NOT NULL,
			description     TEXT NOT NULL DEFAULT '',
			version         VARCHAR(50) NOT NULL DEFAULT '1.0',

			-- SDK binding
			provider        VARCHAR(100) NOT NULL,
			class_name      VARCHAR(255) NOT NULL,

			-- Classification
			tags            JSONB NOT NULL DEFAULT '[]',
			is_builtin      BOOLEAN NOT NULL DEFAULT false,

			-- Configuration schema (array of parameter definitions)
			config_schema   JSONB NOT NULL DEFAULT '[]',

			-- Ownership (NULL for builtins)
			org_id          UUID,
			created_by      VARCHAR(255),

			-- Timestamps
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`

		// Uniqueness constraints as partial indexes (PG 14+ compatible, replaces NULLS NOT DISTINCT)
		createUniqueIndexes := []string{
			// Unique (identifier, version) per org
			`CREATE UNIQUE INDEX IF NOT EXISTS uq_evaluator_id_ver_org ON evaluator_catalog (identifier, version, org_id) WHERE org_id IS NOT NULL`,
			// Unique (identifier, version) for global/builtin evaluators
			`CREATE UNIQUE INDEX IF NOT EXISTS uq_evaluator_id_ver_global ON evaluator_catalog (identifier, version) WHERE org_id IS NULL`,
		}

		createIndexes := []string{
			// Tag filtering (GIN for JSONB array containment queries)
			`CREATE INDEX IF NOT EXISTS idx_evaluator_tags ON evaluator_catalog USING GIN (tags)`,

			// List evaluators for an org (includes org-specific + builtins via OR query)
			`CREATE INDEX IF NOT EXISTS idx_evaluator_org ON evaluator_catalog (org_id)`,

			// Lookup by identifier
			`CREATE INDEX IF NOT EXISTS idx_evaluator_identifier ON evaluator_catalog (identifier)`,

			// Provider filtering
			`CREATE INDEX IF NOT EXISTS idx_evaluator_provider ON evaluator_catalog (provider)`,

			// Free-text search on display_name and description
			`CREATE INDEX IF NOT EXISTS idx_evaluator_search ON evaluator_catalog USING GIN (to_tsvector('english', display_name || ' ' || description))`,
		}

		createTrigger := `
		CREATE OR REPLACE FUNCTION update_evaluator_updated_at()
		RETURNS TRIGGER AS $$
		BEGIN
			NEW.updated_at = NOW();
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;

		DROP TRIGGER IF EXISTS trg_evaluator_updated_at ON evaluator_catalog;
		CREATE TRIGGER trg_evaluator_updated_at
			BEFORE UPDATE ON evaluator_catalog
			FOR EACH ROW
			EXECUTE FUNCTION update_evaluator_updated_at()
		`

		return db.Transaction(func(tx *gorm.DB) error {
			// Create table
			if err := runSQL(tx, createEvaluatorCatalogTable); err != nil {
				return err
			}

			// Create uniqueness constraints (partial indexes for PG 14+ compatibility)
			for _, idx := range createUniqueIndexes {
				if err := runSQL(tx, idx); err != nil {
					return err
				}
			}

			// Create indexes
			for _, idx := range createIndexes {
				if err := runSQL(tx, idx); err != nil {
					return err
				}
			}

			// Create trigger
			if err := runSQL(tx, createTrigger); err != nil {
				return err
			}

			// Load builtin evaluators from JSON file
			if err := loadBuiltinEvaluatorsFromJSON(tx); err != nil {
				// Log warning but don't fail migration - loading can be done later
				slog.Warn("Failed to load builtin evaluators from JSON file", "error", err)
				slog.Info("You can populate evaluators later by placing builtin_evaluators.json in the data directory")
			}

			return nil
		})
	},
}

// loadBuiltinEvaluatorsFromJSON reads evaluators from JSON file and inserts them
func loadBuiltinEvaluatorsFromJSON(tx *gorm.DB) error {
	// Check for custom path from environment variable
	evaluatorsPath := os.Getenv("BUILTIN_EVALUATORS_PATH")
	if evaluatorsPath == "" {
		evaluatorsPath = defaultBuiltinEvaluatorsPath
	}

	// Check if file exists
	if _, err := os.Stat(evaluatorsPath); os.IsNotExist(err) {
		slog.Info("Builtin evaluators file not found, skipping load", "path", evaluatorsPath)
		return nil
	}

	// Read the JSON file
	data, err := os.ReadFile(evaluatorsPath)
	if err != nil {
		return fmt.Errorf("failed to read builtin evaluators file: %w", err)
	}

	// Parse JSON
	var evaluators []EvaluatorDefinition
	if err := json.Unmarshal(data, &evaluators); err != nil {
		return fmt.Errorf("failed to parse builtin evaluators JSON: %w", err)
	}

	slog.Info("Loading builtin evaluators from JSON file", "path", evaluatorsPath, "count", len(evaluators))

	// Insert each evaluator
	for _, evalDef := range evaluators {
		if err := insertEvaluator(tx, evalDef); err != nil {
			return fmt.Errorf("failed to insert evaluator %s: %w", evalDef.Name, err)
		}
	}

	slog.Info("Finished loading builtin evaluators", "count", len(evaluators))
	return nil
}

// insertEvaluator inserts a single evaluator with upsert logic
func insertEvaluator(tx *gorm.DB, evalDef EvaluatorDefinition) error {
	// Marshal tags and config_schema to JSON
	tagsJSON, err := json.Marshal(evalDef.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}

	configSchemaJSON, err := json.Marshal(evalDef.ConfigSchema)
	if err != nil {
		return fmt.Errorf("failed to marshal config_schema: %w", err)
	}

	// Use the name directly as identifier (no transformation needed)
	identifier := evalDef.Name

	// Extract display name from identifier
	// If identifier contains "/", use the part after "/" for display
	displayNameBase := identifier
	if idx := strings.LastIndex(identifier, "/"); idx != -1 {
		displayNameBase = identifier[idx+1:]
	}
	// Replace underscores and hyphens with spaces, then title case
	displayName := cases.Title(language.English).String(
		strings.ReplaceAll(strings.ReplaceAll(displayNameBase, "_", " "), "-", " "),
	)

	// Use parameterized query for safety
	sql := `
		INSERT INTO evaluator_catalog (
			identifier, display_name, description, version, provider, class_name,
			tags, config_schema, is_builtin, org_id, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7::jsonb, $8::jsonb, true, NULL, 'system'
		)
		ON CONFLICT (identifier, version) WHERE org_id IS NULL
		DO UPDATE SET
			display_name = EXCLUDED.display_name,
			description = EXCLUDED.description,
			provider = EXCLUDED.provider,
			class_name = EXCLUDED.class_name,
			tags = EXCLUDED.tags,
			config_schema = EXCLUDED.config_schema,
			updated_at = NOW()
	`

	return tx.Exec(sql,
		identifier,
		displayName,
		evalDef.Description,
		evalDef.Version,
		evalDef.Metadata.Module, // module is the provider
		evalDef.Metadata.ClassName,
		string(tagsJSON),
		string(configSchemaJSON),
	).Error
}
