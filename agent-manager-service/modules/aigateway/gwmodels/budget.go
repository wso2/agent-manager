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

package gwmodels

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm/schema"
)

// BudgetOrigin says who created a budget.
type BudgetOrigin string

const (
	// BudgetOriginControlPlane budgets are created and managed in the control plane.
	BudgetOriginControlPlane BudgetOrigin = "control_plane"
	// BudgetOriginGateway budgets were created by a gateway through its own API and are
	// pushed by it; they are read-only in the control plane.
	BudgetOriginGateway BudgetOrigin = "gateway_api"
)

// API returns the origin as the REST API names it: controlPlane or gatewayApi.
func (o BudgetOrigin) API() string {
	if o == BudgetOriginGateway {
		return "gatewayApi"
	}
	return "controlPlane"
}

// Budget is a spend limit on LLM traffic: a mod_ai_gateway_budgets row, named by the
// budget name (unique in the organization). Configuration is the control-plane model, as
// LLMProviderConfig is for a provider: the spec of the Budget kind, which is built from it
// when it is sent out.
type Budget struct {
	ID        uuid.UUID    `gorm:"column:id;primaryKey"`
	OrgID     string       `gorm:"column:org_id"`
	Name      string       `gorm:"column:name"`
	Origin    BudgetOrigin `gorm:"column:origin"`
	GatewayID *uuid.UUID   `gorm:"column:gateway_id"` // set for gateway budgets only
	ProjectID *uuid.UUID   `gorm:"column:project_id"` // the owning project; nil: the organization
	// Configuration is the kind's spec; the column holds the whole Budget kind as received,
	// written and read by the budgetkind serializer.
	Configuration BudgetConfig `gorm:"column:configuration;serializer:budgetkind"`
	CreatedBy     string       `gorm:"column:created_by"`
	CreatedAt     time.Time    `gorm:"column:created_at"`
	UpdatedAt     time.Time    `gorm:"column:updated_at"`

	// The owners' handles, from the core's organization and project; the repository fills
	// them in after reading.
	OrgName     string `gorm:"-"`
	ProjectName string `gorm:"-"`
}

// BudgetConfig is a budget's settings, with defaults filled in: the
// spec of the Budget kind.
type BudgetConfig struct {
	DisplayName string            `json:"displayName,omitempty"`
	Description *string           `json:"description,omitempty"`
	Status      string            `json:"status"`
	Limit       BudgetLimit       `json:"limit"`
	Filters     []BudgetFilter    `json:"filters"`
	Exemptions  []BudgetExemption `json:"exemptions"`
	Warnings    *BudgetWarnings   `json:"warnings,omitempty"`
	// Labels are the kind's metadata.labels, kept with the spec: one-word labels to
	// organize and find budgets. Not in the spec's JSON; stored under metadata and never
	// sent to gateways.
	Labels []string `json:"-"`
}

// BudgetLimit is how much may be spent per period, what happens when it is used up, and
// what gets its own limit.
type BudgetLimit struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
	Period   string `json:"period"`
	Action   string `json:"action"`
	BlockAt  *int32 `json:"blockAt,omitempty"`
	// SplitBy gives each value of a type its own limit; nil: one shared limit for all
	// matching usage.
	SplitBy *BudgetSplit `json:"splitBy,omitempty"`
}

// BudgetSplit is what a budget gives a separate limit to: each LLM provider, project,
// agent, API key, or value of an attribute expression.
type BudgetSplit struct {
	Type       string  `json:"type"`
	Expression *string `json:"expression,omitempty"`
}

// BudgetFilter selects the usage a budget counts; filters combine with AND, and none
// counts all LLM usage.
type BudgetFilter struct {
	Type string  `json:"type"`
	ID   *string `json:"id,omitempty"`
	// Project is an agent filter's project (agent names are unique only per project).
	Project    *string `json:"project,omitempty"`
	Expression *string `json:"expression,omitempty"`
}

// BudgetExemption is traffic a budget neither counts nor blocks.
type BudgetExemption struct {
	Type       string  `json:"type"`
	ID         *string `json:"id,omitempty"`
	Project    *string `json:"project,omitempty"`
	Expression *string `json:"expression,omitempty"`
}

// BudgetWarnings is when, and to which notification channels, a budget warns.
type BudgetWarnings struct {
	Thresholds []int32 `json:"thresholds"`
	// Channels are notification channel names, as in the kind.
	Channels []string `json:"channels"`
}

// TableName maps Budget to its table.
func (Budget) TableName() string { return "mod_ai_gateway_budgets" }

// KindBudget is the Budget kind. Budgets are not artifacts: they are not deployed.
const KindBudget = "Budget"

// BudgetAPIVersion is the Budget kind's apiVersion.
const BudgetAPIVersion = "gateway.api-platform.wso2.com/v1"

// AnnotationBudgetProjectID names the project that owns a budget.
const AnnotationBudgetProjectID = "gateway.api-platform.wso2.com/project-id"

// BudgetDocument is a budget as stored, as a gateway stores it: the Budget kind as
// received.
type BudgetDocument struct {
	APIVersion string                 `json:"apiVersion"`
	Kind       string                 `json:"kind"`
	Metadata   BudgetDocumentMetadata `json:"metadata"`
	Spec       BudgetConfig           `json:"spec"`
}

// BudgetDocumentMetadata names a stored budget and its owning project.
type BudgetDocumentMetadata struct {
	Name        string            `json:"name"`
	Labels      []string          `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// NewBudgetDocument is the stored kind of a budget with that name, project ("" for an
// organization budget) and spec.
func NewBudgetDocument(name, project string, spec BudgetConfig) BudgetDocument {
	doc := BudgetDocument{APIVersion: BudgetAPIVersion, Kind: KindBudget, Metadata: BudgetDocumentMetadata{Name: name, Labels: spec.Labels}, Spec: spec}
	if project != "" {
		doc.Metadata.Annotations = map[string]string{AnnotationBudgetProjectID: project}
	}
	return doc
}

func init() {
	schema.RegisterSerializer("budgetkind", budgetKindSerializer{})
}

// budgetKindSerializer stores a Budget's Configuration as the whole Budget kind, named
// after the budget's Name and ProjectName, and reads the spec back. Metadata is read from
// the budget's columns, so the stored annotation never overrides the current project.
type budgetKindSerializer struct{}

func (budgetKindSerializer) Scan(ctx context.Context, field *schema.Field, dst reflect.Value, dbValue any) error {
	var raw []byte
	switch v := dbValue.(type) {
	case nil:
		return nil
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("budget configuration: unsupported type %T", dbValue)
	}
	var probe struct {
		Metadata struct {
			Labels []string `json:"labels"`
		} `json:"metadata"`
		Spec json.RawMessage `json:"spec"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return fmt.Errorf("budget configuration: %w", err)
	}
	var cfg BudgetConfig
	body := raw
	if len(probe.Spec) > 0 {
		body = probe.Spec
	}
	if err := json.Unmarshal(body, &cfg); err != nil {
		return fmt.Errorf("budget configuration: %w", err)
	}
	if len(probe.Metadata.Labels) > 0 {
		cfg.Labels = probe.Metadata.Labels
	}
	return field.Set(ctx, dst, cfg)
}

func (budgetKindSerializer) Value(_ context.Context, _ *schema.Field, dst reflect.Value, fieldValue any) (any, error) {
	cfg, _ := fieldValue.(BudgetConfig)
	name, project := "", ""
	for dst.Kind() == reflect.Pointer {
		dst = dst.Elem()
	}
	if dst.Kind() == reflect.Struct {
		name = dst.FieldByName("Name").String()
		project = dst.FieldByName("ProjectName").String()
	}
	b, err := json.Marshal(NewBudgetDocument(name, project, cfg))
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// BudgetKind is the Budget kind: what gateways receive and what pipelines
// keep in files. Agent Manager stores only its spec, as BudgetConfig.
type BudgetKind struct {
	APIVersion string             `json:"apiVersion" yaml:"apiVersion"`
	Kind       string             `json:"kind" yaml:"kind"`
	Metadata   BudgetKindMetadata `json:"metadata" yaml:"metadata"`
	Spec       BudgetConfig       `json:"spec" yaml:"spec"`
}

// BudgetKindMetadata names the budget and, by annotation, its organization.
type BudgetKindMetadata struct {
	Name        string            `json:"name" yaml:"name"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// BudgetGatewayPayload is a budget as a gateway gets it, in budget.created and
// budget.updated events and from GET /api/internal/v1/budgets. A gateway ignores one whose
// updatedAt is older than its stored copy.
type BudgetGatewayPayload struct {
	BudgetID  string     `json:"budgetId"`
	UpdatedAt time.Time  `json:"updatedAt"`
	Budget    BudgetKind `json:"budget"`
}

// BudgetDeletedPayload is the payload of budget.deleted.
type BudgetDeletedPayload struct {
	BudgetID string `json:"budgetId"`
}
