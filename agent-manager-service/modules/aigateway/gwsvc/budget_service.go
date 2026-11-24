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

package gwsvc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/types"
	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/notifyv1"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwaudit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

// Budget field values and limits.
const (
	budgetAPIVersion      = "gateway.api-platform.wso2.com/v1"
	budgetStatusActive    = "active"
	budgetStatusInactive  = "inactive"
	budgetDefaultPageSize = 20
	budgetMaxPageSize     = 100
	budgetCurrencyUSD     = "USD"
	budgetActionBlock     = "block"
	budgetActionAllow     = "allow"
	budgetDefaultBlockAt  = 100

	budgetTypeLLMProvider = "llmProvider"
	budgetTypeProject     = "project"
	budgetTypeAgent       = "agent"
	budgetTypeAPIKey      = "apiKey"
	budgetTypeAttribute   = "attribute"

	maxBudgetDisplayName = 255
	maxBudgetDescription = 1023
	maxBudgetFilters     = 20
	maxBudgetExemptions  = 100
	maxBudgetThresholds  = 10
	maxBudgetThreshold   = 1000
	maxBudgetChannels    = 20
	// maxBudgetExpression caps an attribute's CEL expression; gateways run it on every request.
	maxBudgetExpression = 1024
)

// gatewayConsumerTypes are the consumer types (filter, splitBy and exemption types other
// than llmProvider) gateways can identify: token claims (attribute); the project and agent
// of an agent's hidden LLM proxy, from its annotations; and the API key a request
// authenticated with. A type not listed here is refused (UNSUPPORTED_CONSUMER).
var gatewayConsumerTypes = []string{budgetTypeAttribute, budgetTypeProject, budgetTypeAgent, budgetTypeAPIKey}

var (
	budgetNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	// The gateway's own rule (its management API), so every budget saved here is one a
	// gateway accepts: above zero, at most 12 whole digits and 2 decimals, no leading zeros.
	budgetAmountPattern = regexp.MustCompile(`^([1-9][0-9]{0,11}(\.[0-9]{1,2})?|0\.([1-9][0-9]?|0[1-9]))$`)
	budgetPeriods       = []string{"day", "week", "month"}
	// budgetFilterTypes are the types of a filter and of limit.splitBy.
	budgetFilterTypes = []string{budgetTypeLLMProvider, budgetTypeProject, budgetTypeAgent, budgetTypeAPIKey, budgetTypeAttribute}
)

// BudgetError is a budget request the service refuses: Code is the ErrorResponse code,
// Field the payload field it is about (empty when it is about the whole request).
type BudgetError struct {
	Code    string
	Message string
	Field   string
}

func (e *BudgetError) Error() string { return e.Message }

// Unwrap returns the class of the refusal, which sets the response status.
func (e *BudgetError) Unwrap() error {
	switch e.Code {
	case gwerr.ErrCodeBudgetAlreadyExists, gwerr.ErrCodeBudgetReadOnly:
		return gwerr.ErrBudgetConflict
	case gwerr.ErrCodeBudgetNotFound:
		return gwerr.ErrBudgetNotFound
	}
	return gwerr.ErrBudgetInvalid
}

func invalidBudget(field, format string, args ...any) error {
	return &BudgetError{Code: httpx.ErrCodeValidation, Message: fmt.Sprintf(format, args...), Field: field}
}

// budgetCELEnv is the environment gateways compile budget expressions in (the budget
// system policy): the policy engine's request, with Auth.
var budgetCELEnv = sync.OnceValues(func() (*cel.Env, error) {
	return cel.NewEnv(cel.Variable("request", cel.MapType(cel.StringType, cel.DynType)))
})

// checkBudgetExpression refuses an expression gateways would skip (INVALID_EXPRESSION): it
// must compile, and return bool as a condition (a filter or exemption) or a string or list
// as the value a split limit is given by (limit.splitBy). Claims are dynamic, so most types
// are known only when it runs.
func checkBudgetExpression(field, expression string, split bool) error {
	env, err := budgetCELEnv()
	if err != nil {
		return fmt.Errorf("failed to create the expression environment: %w", err)
	}
	invalid := func(format string, args ...any) error {
		return &BudgetError{Code: gwerr.ErrCodeInvalidExpression, Field: field, Message: fmt.Sprintf(format, args...)}
	}
	ast, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return invalid("%s does not compile: %s", field, issues.Err())
	}
	out := ast.OutputType()
	switch {
	case out == types.DynType:
	case split && out != types.StringType && out.Kind() != types.ListKind:
		return invalid("%s must return a string or a list of strings for limit.splitBy, not %s.", field, out)
	case !split && out != types.BoolType:
		return invalid("%s must return bool, not %s.", field, out)
	}
	if tooCostly(env, ast) {
		return invalid("%s is too costly to run on every request.", field)
	}
	return nil
}

// maxBudgetExpressionCost is the gateway's cost limit for one budget expression per request
// (system-policies/budget maxExpressionCost): above it the gateway stops the expression and
// the budget counts nothing.
const maxBudgetExpressionCost = 10000

// tooCostly runs the expression once on an empty request under the gateway's cost limit.
// Claims are known only when a request arrives, so a static estimate cannot bound most
// expressions; but one that exceeds the limit with no input (nested macros over large
// literal lists) always will, and the gateway would skip it. Other errors (a claim that
// is not there) are expected here and ignored.
func tooCostly(env *cel.Env, ast *cel.Ast) bool {
	prg, err := env.Program(ast, cel.CostLimit(maxBudgetExpressionCost))
	if err != nil {
		return false
	}
	_, _, err = prg.Eval(map[string]any{"request": map[string]any{}})
	return err != nil && strings.Contains(err.Error(), "cost limit exceeded")
}

func unsupportedConsumer(field, typ string) error {
	return &BudgetError{
		Code: gwerr.ErrCodeUnsupportedConsumer, Field: field,
		Message: fmt.Sprintf("Gateways cannot identify %s consumers yet; use an attribute (token claim) instead.", typ),
	}
}

func budgetReferenceNotFound(field, format string, args ...any) error {
	return &BudgetError{Code: gwerr.ErrCodeReferenceNotFound, Message: fmt.Sprintf(format, args...), Field: field}
}

// BudgetService manages budgets created in the control plane.
type BudgetService interface {
	// Create stores a new, active budget for the organization. createdBy is the caller.
	Create(ctx context.Context, ouID, createdBy string, kind gwspec.Budget) (*gwspec.Budget, error)
	// Import creates the budgets of a CSV file, one per row, all or none.
	// onExisting says what a row naming an existing budget does: fail (default), skip or
	// update.
	// The result lists each row's errors; applied says whether they were saved. A file that
	// cannot be read is a *BudgetError with code INVALID_CSV.
	Import(ctx context.Context, ouID, createdBy string, file io.Reader, dryRun bool, onExisting string) (*gwspec.BudgetImportResult, error)
	// Get returns the organization's budget named name.
	Get(ctx context.Context, ouID, name string) (*gwspec.Budget, error)
	// List returns a page of the organization's budgets matching every filter (type:id),
	// only those the project owns when projectID is set.
	List(ctx context.Context, ouID, projectID string, filters []string, labels []string, search, status string, limit, offset int32) (*gwspec.BudgetListResponse, error)
	// Update replaces a budget's editable fields. A gateway's budget changes only in the
	// fields gateways never get (displayName, description, warnings).
	Update(ctx context.Context, ouID, name string, kind gwspec.Budget) (*gwspec.Budget, error)
	// Usage returns a page of the budget's allowances with their spend this period,
	// highest first (GET /budgets/{budgetName}/budget-usage).
	Usage(ctx context.Context, ouID, name string, limit, offset int32) (*gwspec.BudgetUsageListResponse, error)
	// Delete deletes a control-plane budget.
	Delete(ctx context.Context, ouID, name string) error
}

type budgetService struct {
	budgetRepo   gwrepo.BudgetRepository
	artifactRepo gwrepo.ArtifactRepository
	core         modkit.CoreAPI
	gateways     BudgetGatewaySender
	warner       BudgetWarner
	usageRepo    gwrepo.BudgetUsageRepository
	channels     notifyv1.Notifier
	now          func() time.Time
}

// NewBudgetService creates a BudgetService. gateways sends budgets to the gateways that
// enforce them; warner checks an edited budget's thresholds against its spend; usageRepo
// holds the spend the gateways report.
func NewBudgetService(
	budgetRepo gwrepo.BudgetRepository,
	artifactRepo gwrepo.ArtifactRepository,
	core modkit.CoreAPI,
	gateways BudgetGatewaySender,
	warner BudgetWarner,
	usageRepo gwrepo.BudgetUsageRepository,
	channels notifyv1.Notifier,
) BudgetService {
	return &budgetService{
		budgetRepo: budgetRepo, artifactRepo: artifactRepo, core: core,
		gateways: gateways, warner: warner, usageRepo: usageRepo, channels: channels, now: time.Now,
	}
}

// BuildBudgetKind wraps a stored budget in the Budget kind, with the org-id annotation
// naming its organization (orgName, its handle). For a gateway it leaves out what gateways
// never get: displayName, description and warnings.
func BuildBudgetKind(name, orgName string, cfg gwmodels.BudgetConfig, forGateway bool) gwmodels.BudgetKind {
	if forGateway {
		cfg.DisplayName, cfg.Description, cfg.Warnings = "", nil, nil
	}
	metadata := gwmodels.BudgetKindMetadata{Name: name}
	if orgName != "" {
		metadata.Annotations = map[string]string{AnnotationOrgID: orgName}
	}
	return gwmodels.BudgetKind{APIVersion: budgetAPIVersion, Kind: gwmodels.KindBudget, Metadata: metadata, Spec: cfg}
}

// budgetRequest is a new budget's fields: from a Budget kind (Create, Update) or a CSV
// row (Import).
type budgetRequest struct {
	Name string
	// OrgId is the org-id annotation: nil when left out, filled in with the path's.
	OrgId       *string
	ProjectId   *string
	DisplayName *string
	Description *string
	Status      *string
	Limit       gwspec.BudgetLimit
	Filters     []gwspec.BudgetFilter
	Exemptions  []gwspec.BudgetExemption
	Warnings    *gwspec.BudgetWarnings
	Labels      []string
}

// budgetUpdateRequest is what an update may change; the other fields are fixed after creation.
type budgetUpdateRequest struct {
	DisplayName *string
	Description *string
	Status      *string
	Limit       budgetUpdateLimit
	Exemptions  []gwspec.BudgetExemption
	Warnings    *gwspec.BudgetWarnings
	// Labels replace the budget's labels; they can change on any budget.
	Labels []string
}

// budgetUpdateLimit is the editable part of the limit; currency, period and splitBy are fixed.
type budgetUpdateLimit struct {
	Amount  string
	Action  string
	BlockAt *int32
}

// budgetFromKind reads a Budget kind sent to the API: the org-id annotation
// names the organization, which must be the one in the path (left out, it is that one); the
// project-id annotation names the owning project.
func budgetFromKind(kind gwspec.Budget) (budgetRequest, error) {
	if kind.ApiVersion != budgetAPIVersion {
		return budgetRequest{}, invalidBudget("apiVersion", "apiVersion must be %s.", budgetAPIVersion)
	}
	if kind.Kind != gwmodels.KindBudget {
		return budgetRequest{}, invalidBudget("kind", "kind must be %s.", gwmodels.KindBudget)
	}
	req := budgetRequest{
		Name: kind.Metadata.Name, DisplayName: kind.Spec.DisplayName, Description: kind.Spec.Description,
		Status: kind.Spec.Status, Limit: kind.Spec.Limit,
		Filters: kind.Spec.Filters, Exemptions: kind.Spec.Exemptions, Warnings: kind.Spec.Warnings,
	}
	req.Labels = kind.Metadata.Labels
	if kind.Metadata.Annotations != nil {
		if org, ok := (*kind.Metadata.Annotations)[AnnotationOrgID]; ok {
			org = strings.TrimSpace(org)
			if org == "" {
				return budgetRequest{}, invalidBudget("metadata.annotations",
					"%s must name the organization; leave it out for the organization in the path.", AnnotationOrgID)
			}
			req.OrgId = &org
		}
		if project, ok := (*kind.Metadata.Annotations)[AnnotationProjectID]; ok {
			project = strings.TrimSpace(project)
			if project == "" {
				return budgetRequest{}, invalidBudget("metadata.annotations",
					"%s must name a project; leave it out for an organization budget.", AnnotationProjectID)
			}
			req.ProjectId = &project
		}
	}
	return req, nil
}

func (s *budgetService) Create(ctx context.Context, ouID, createdBy string, kind gwspec.Budget) (*gwspec.Budget, error) {
	req, err := budgetFromKind(kind)
	if err != nil {
		return nil, err
	}
	return s.create(ctx, ouID, createdBy, req)
}

func (s *budgetService) create(ctx context.Context, ouID, createdBy string, req budgetRequest) (*gwspec.Budget, error) {
	if ouID == "" {
		return nil, gwerr.ErrOrganizationNotFound
	}
	budget, err := s.prepareBudget(ctx, ouID, createdBy, req)
	if err != nil {
		return nil, err
	}
	if err := s.budgetRepo.CreateForControlPlane(ctx, budget, req.Name); err != nil {
		if errors.Is(err, gwerr.ErrBudgetAlreadyExists) {
			return nil, budgetNameTaken(req.Name)
		}
		return nil, err
	}
	s.gateways.SendBudget(ctx, budget, true)
	return s.response(ctx, ouID, budget)
}

// prepareBudget checks a new budget, its owner and every name it refers
// to, and returns it ready to store; Create and Import share it.
func (s *budgetService) prepareBudget(ctx context.Context, ouID, createdBy string, req budgetRequest) (*gwmodels.Budget, error) {
	req.Filters = withProjectFilter(req.Filters, req.ProjectId)
	cfg, err := normalizeBudget(req)
	if err != nil {
		return nil, err
	}
	orgName, err := s.orgHandle(ctx, ouID)
	if err != nil {
		return nil, err
	}
	if err := sameOrg(req.OrgId, orgName); err != nil {
		return nil, err
	}
	var projectID *uuid.UUID
	projectName := ""
	if req.ProjectId != nil {
		projectName = strings.TrimSpace(*req.ProjectId)
		if projectID, err = s.projectID(ctx, ouID, projectName, "metadata.annotations"); err != nil {
			return nil, err
		}
		if err := ownByProject(cfg, projectName); err != nil {
			return nil, err
		}
	}
	if err := s.checkReferences(ctx, ouID, cfg); err != nil {
		return nil, err
	}
	return &gwmodels.Budget{
		OrgID: ouID, Origin: gwmodels.BudgetOriginControlPlane, ProjectID: projectID, Configuration: *cfg, CreatedBy: createdBy,
		OrgName: orgName, ProjectName: projectName,
	}, nil
}

func budgetNameTaken(name string) error {
	return &BudgetError{
		Code: gwerr.ErrCodeBudgetAlreadyExists, Field: "name",
		Message: fmt.Sprintf("A budget or other resource named %s already exists.", name),
	}
}

// sameOrg checks a budget's org-id annotation, when sent, names the organization it is
// created or updated in. A budget belongs to one organization; left out, it is that one.
func sameOrg(sent *string, orgName string) error {
	if sent == nil || *sent == orgName {
		return nil
	}
	return invalidBudget("metadata.annotations", "%s must be %s, the organization in the path.", AnnotationOrgID, orgName)
}

// orgHandle returns the organization's handle.
func (s *budgetService) orgHandle(ctx context.Context, ouID string) (string, error) {
	org, err := LookupOrg(ctx, s.core, ouID)
	if err != nil {
		return "", fmt.Errorf("failed to get organization: %w", err)
	}
	return org.Handle, nil
}

// projectID returns the ID of the organization's project with that name.
func (s *budgetService) projectID(ctx context.Context, ouID, name, field string) (*uuid.UUID, error) {
	return budgetProjectID(ctx, s.core, ouID, name, field)
}

// budgetProjectID returns the ID of the organization's project with that name, or
// REFERENCE_NOT_FOUND on field when there is none.
func budgetProjectID(ctx context.Context, core modkit.CoreAPI, ouID, name, field string) (*uuid.UUID, error) {
	p, err := LookupProject(ctx, core, ouID, name)
	if err != nil {
		if errors.Is(err, gwerr.ErrProjectNotFound) {
			return nil, budgetReferenceNotFound(field, "Project '%s' was not found.", name)
		}
		return nil, fmt.Errorf("failed to get project %q: %w", name, err)
	}
	id, err := uuid.Parse(p.ID)
	if err != nil {
		return nil, fmt.Errorf("project %q has an invalid ID: %w", name, err)
	}
	return &id, nil
}

// withProjectFilter appends the owning project as the last filter when a project budget
// (the project-id annotation) lists no project filter: a project budget counts only its
// project's usage. The stored and returned kind lists it, so gateways get it too; it goes
// last so error indexes match what the client sent. A project filter the request lists is
// kept as sent; ownByProject checks it. No other filter is added: an agent filter carries
// its project.
func withProjectFilter(filters []gwspec.BudgetFilter, project *string) []gwspec.BudgetFilter {
	if project == nil || strings.TrimSpace(*project) == "" {
		return filters
	}
	for _, f := range filters {
		if f.Type == budgetTypeProject {
			return filters
		}
	}
	id := strings.TrimSpace(*project)
	return append(append([]gwspec.BudgetFilter{}, filters...), gwspec.BudgetFilter{Type: budgetTypeProject, Id: &id})
}

// ownByProject checks a budget owned by a project (the project-id annotation) is that
// project's own: its filters name that project, so it counts only that
// project's usage (an id-less project filter, any project, is refused); any agent it names
// is of that project; its limit is not split by
// project; and it exempts only agents of that project. Create adds the project filter when
// it is left out (withProjectFilter), as gateways do before pushing.
func ownByProject(cfg *gwmodels.BudgetConfig, project string) error {
	listed := false
	for i, f := range cfg.Filters {
		switch {
		case f.Type == budgetTypeProject && (f.ID == nil || *f.ID != project):
			return invalidBudget(fmt.Sprintf("filters[%d].id", i),
				"A budget of project %s counts only that project: its project filter must be that project.", project)
		case f.Type == budgetTypeProject:
			listed = true
		case f.Type == budgetTypeAgent && f.Project != nil && *f.Project != project:
			// An id-less agent filter (any agent) is allowed: the project filter keeps it to
			// this project's agents.
			return invalidBudget(fmt.Sprintf("filters[%d].project", i), "A budget of project %s can count only its agents.", project)
		}
	}
	if cfg.Limit.SplitBy != nil && cfg.Limit.SplitBy.Type == budgetTypeProject {
		return invalidBudget("limit.splitBy.type", "A budget of project %s cannot be split by project.", project)
	}
	for i, e := range cfg.Exemptions {
		field := fmt.Sprintf("exemptions[%d]", i)
		if e.Type == budgetTypeProject {
			return invalidBudget(field+".type", "A budget of project %s cannot exempt a project.", project)
		}
		if e.Type == budgetTypeAgent && (e.Project == nil || *e.Project != project) {
			return invalidBudget(field+".project", "A budget of project %s can exempt only its agents.", project)
		}
	}
	if !listed {
		return invalidBudget("filters",
			"A budget of project %s must list that project as a filter: {type: project, id: %s}.", project, project)
	}
	return nil
}

func budgetNotFound(name string) error {
	return &BudgetError{Code: gwerr.ErrCodeBudgetNotFound, Message: fmt.Sprintf("Budget '%s' was not found.", name)}
}

// getBudget loads a budget, answering BUDGET_NOT_FOUND when there is none.
func (s *budgetService) getBudget(ctx context.Context, ouID, name string) (*gwmodels.Budget, error) {
	if ouID == "" {
		return nil, gwerr.ErrOrganizationNotFound
	}
	b, err := s.budgetRepo.Get(ctx, ouID, name)
	if errors.Is(err, gwerr.ErrBudgetNotFound) {
		return nil, budgetNotFound(name)
	}
	return b, err
}

// writableBudget loads a budget the control plane may change: a gateway's are read-only.
func (s *budgetService) writableBudget(ctx context.Context, ouID, name string) (*gwmodels.Budget, error) {
	b, err := s.getBudget(ctx, ouID, name)
	if err != nil {
		return nil, err
	}
	if b.Origin != gwmodels.BudgetOriginControlPlane {
		return nil, &BudgetError{
			Code:    gwerr.ErrCodeBudgetReadOnly,
			Message: fmt.Sprintf("Budget %s was created on a gateway and can be changed only there.", name),
		}
	}
	return b, nil
}

func (s *budgetService) Get(ctx context.Context, ouID, name string) (*gwspec.Budget, error) {
	b, err := s.getBudget(ctx, ouID, name)
	if err != nil {
		return nil, err
	}
	return s.response(ctx, ouID, b)
}

func (s *budgetService) List(ctx context.Context, ouID, projectID string, filters []string, labels []string, search, status string, limit, offset int32) (*gwspec.BudgetListResponse, error) {
	if ouID == "" {
		return nil, gwerr.ErrOrganizationNotFound
	}
	if limit == 0 {
		limit = budgetDefaultPageSize
	}
	if limit < 1 || limit > budgetMaxPageSize {
		return nil, invalidBudget("limit", "limit must be from 1 to %d.", budgetMaxPageSize)
	}
	if offset < 0 {
		return nil, invalidBudget("offset", "offset must not be negative.")
	}
	matchers, err := parseBudgetFilters(filters)
	if err != nil {
		return nil, err
	}
	if status != "" && !containsString(budgetListStatuses, status) {
		return nil, invalidBudget("status", "status must be one of %s.", strings.Join(budgetListStatuses, ", "))
	}
	if len([]rune(search)) > maxBudgetSearch {
		return nil, invalidBudget("search", "search must be at most %d characters.", maxBudgetSearch)
	}
	search = strings.ToLower(strings.TrimSpace(search))
	// An organization has at most a few thousand budgets, so they are
	// filtered here rather than with database-specific JSON queries.
	all, err := s.budgetRepo.List(ctx, ouID)
	if err != nil {
		return nil, err
	}
	matched := make([]*gwmodels.Budget, 0, len(all))
	for i := range all {
		if projectID != "" && all[i].ProjectName != projectID {
			continue
		}
		if !budgetMatches(&all[i].Configuration, matchers) || !budgetHasLabels(&all[i].Configuration, labels) ||
			!budgetSearchMatches(&all[i], search) {
			continue
		}
		if status == budgetStatusActive || status == budgetStatusInactive {
			if all[i].Configuration.Status != status {
				continue
			}
		}
		matched = append(matched, &all[i])
	}
	// Ordered by the name people see, so pages are stable.
	sort.SliceStable(matched, func(a, b int) bool {
		x, y := strings.ToLower(matched[a].Configuration.DisplayName), strings.ToLower(matched[b].Configuration.DisplayName)
		if x != y {
			return x < y
		}
		return matched[a].Name < matched[b].Name
	})
	if status == budgetListStatusLimitReached {
		// Needs this period's spend: read for every match, then paged.
		resps, err := s.responses(ctx, ouID, matched)
		if err != nil {
			return nil, err
		}
		reached := make([]gwspec.Budget, 0, len(resps))
		for _, r := range resps {
			if budgetLimitReached(&r) {
				reached = append(reached, r)
			}
		}
		total := int32(len(reached))
		start, end := min(offset, total), min(offset+limit, total)
		return gwspec.NewBudgetListResponse(reached[start:end], total, limit, offset), nil
	}
	total := int32(len(matched))
	start, end := min(offset, total), min(offset+limit, total)
	page, err := s.responses(ctx, ouID, matched[start:end])
	if err != nil {
		return nil, err
	}
	return gwspec.NewBudgetListResponse(page, total, limit, offset), nil
}

// response is a budget as the API returns it, with its spend this period.
func (s *budgetService) response(ctx context.Context, ouID string, b *gwmodels.Budget) (*gwspec.Budget, error) {
	resps, err := s.responses(ctx, ouID, []*gwmodels.Budget{b})
	if err != nil {
		return nil, err
	}
	return &resps[0], nil
}

// responses are budgets as the API returns them, each with its spend this period
// (currentPeriod), read in one query.
func (s *budgetService) responses(ctx context.Context, ouID string, budgets []*gwmodels.Budget) ([]gwspec.Budget, error) {
	out := make([]gwspec.Budget, len(budgets))
	ids := make([]uuid.UUID, len(budgets))
	orgName := ""
	for i, b := range budgets {
		if b.OrgName == "" {
			if orgName == "" {
				var err error
				if orgName, err = s.orgHandle(ctx, ouID); err != nil {
					return nil, err
				}
			}
			b.OrgName = orgName
		}
		out[i] = *BudgetAsKind(b)
		ids[i] = b.ID
	}
	var rows []gwmodels.BudgetUsage
	if s.usageRepo != nil && len(ids) > 0 {
		var err error
		if rows, err = s.usageRepo.ForBudgets(ctx, ouID, ids); err != nil {
			return nil, err
		}
	}
	byBudget := map[uuid.UUID][]gwmodels.BudgetUsage{}
	for _, r := range rows {
		byBudget[r.BudgetID] = append(byBudget[r.BudgetID], r)
	}
	now := s.now()
	for i, b := range budgets {
		start := budgetPeriodStart(b.Configuration.Limit.Period, now)
		spent, unpriced := new(big.Rat), int64(0)
		for _, r := range byBudget[b.ID] {
			rowSpent, rowUnpriced := spendIn(r, start)
			spent.Add(spent, rowSpent)
			unpriced += rowUnpriced
		}
		current := gwspec.NewBudgetCurrentPeriod(start.Format(time.DateOnly), usdString(spent), int32(min(unpriced, math.MaxInt32)))
		out[i].Status.CurrentPeriod = current
	}
	return out, nil
}

// budgetPeriodStart is the start of the budget period that holds now: the UTC day, the
// week from Monday, or the month from the 1st.
func budgetPeriodStart(period string, now time.Time) time.Time {
	now = now.UTC()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	switch period {
	case "day":
		return day
	case "week":
		return day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
	default:
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
}

// spendIn is an allowance's spend in the period starting at start: its stored totals, or
// nothing when they belong to an earlier period.
func spendIn(r gwmodels.BudgetUsage, start time.Time) (*big.Rat, int64) {
	if !r.PeriodStart.UTC().Equal(start) {
		return new(big.Rat), 0
	}
	spent, ok := new(big.Rat).SetString(r.Spent)
	if !ok {
		spent = new(big.Rat)
	}
	return spent, r.UnpricedRequests
}

func (s *budgetService) Usage(ctx context.Context, ouID, name string, limit, offset int32) (*gwspec.BudgetUsageListResponse, error) {
	if limit == 0 {
		limit = budgetDefaultPageSize
	}
	if limit < 1 || limit > budgetMaxPageSize {
		return nil, invalidBudget("limit", "limit must be from 1 to %d.", budgetMaxPageSize)
	}
	if offset < 0 {
		return nil, invalidBudget("offset", "offset must not be negative.")
	}
	b, err := s.getBudget(ctx, ouID, name)
	if err != nil {
		return nil, err
	}
	rows, err := s.usageRepo.ForBudget(ctx, b.ID)
	if err != nil {
		return nil, err
	}
	start := budgetPeriodStart(b.Configuration.Limit.Period, s.now())
	type usage struct {
		row   gwmodels.BudgetUsage
		spent *big.Rat
		label string
	}
	all := make([]usage, 0, len(rows))
	for _, r := range rows {
		spent, unpriced := spendIn(r, start)
		r.UnpricedRequests = unpriced
		all = append(all, usage{row: r, spent: spent, label: allowanceLabel(r.Allowance)})
	}
	// Highest spend first; equal spend by allowance, so pages are stable.
	sort.SliceStable(all, func(i, j int) bool {
		if c := all[i].spent.Cmp(all[j].spent); c != 0 {
			return c > 0
		}
		return all[i].label < all[j].label
	})
	total := int32(len(all))
	from, to := min(offset, total), min(offset+limit, total)
	page := make([]gwspec.BudgetAllowanceUsage, 0, to-from)
	for _, u := range all[from:to] {
		allowance := u.row.Allowance
		if allowance == nil {
			allowance = map[string]string{}
		}
		page = append(page, *gwspec.NewBudgetAllowanceUsage(allowance, usdString(u.spent), u.row.UnpricedRequests))
	}
	return gwspec.NewBudgetUsageListResponse(start.Format(time.DateOnly), page, total, limit, offset), nil
}

func (s *budgetService) Update(ctx context.Context, ouID, name string, kind gwspec.Budget) (*gwspec.Budget, error) {
	req, err := budgetFromKind(kind)
	if err != nil {
		return nil, err
	}
	if req.Name != name {
		return nil, invalidBudget("metadata.name", "metadata.name must be %s, the budget being updated; a budget cannot be renamed.", name)
	}
	b, err := s.getBudget(ctx, ouID, name)
	if err != nil {
		return nil, err
	}
	orgName := b.OrgName
	if orgName == "" {
		if orgName, err = s.orgHandle(ctx, ouID); err != nil {
			return nil, err
		}
	}
	if err := sameOrg(req.OrgId, orgName); err != nil {
		return nil, err
	}
	if err := fixedFieldsUnchanged(b, req); err != nil {
		return nil, err
	}
	return s.update(ctx, ouID, b, budgetUpdateRequest{
		DisplayName: req.DisplayName, Description: req.Description, Status: req.Status,
		Limit:      budgetUpdateLimit{Amount: req.Limit.Amount, Action: req.Limit.Action, BlockAt: req.Limit.BlockAt},
		Exemptions: req.Exemptions, Warnings: req.Warnings, Labels: req.Labels,
	})
}

// fixedFieldsUnchanged checks an update sends the fields fixed after creation as stored:
// the owning project, currency, period, limit.splitBy and filters. A
// project budget may leave out its project filter, as on create.
func fixedFieldsUnchanged(b *gwmodels.Budget, req budgetRequest) error {
	fixed := func(field, what string) error {
		return invalidBudget(field, "%s cannot change after creation; create a new budget instead.", what)
	}
	project := ""
	if req.ProjectId != nil {
		project = *req.ProjectId
	}
	if project != b.ProjectName {
		return fixed("metadata.annotations", "The budget's project ("+AnnotationProjectID+")")
	}
	stored := b.Configuration
	if req.Limit.Currency != nil && *req.Limit.Currency != stored.Limit.Currency {
		return fixed("spec.limit.currency", "Currency")
	}
	if req.Limit.Period != stored.Limit.Period {
		return fixed("spec.limit.period", "Period")
	}
	splitBy, err := normalizeBudgetSplit(req.Limit.SplitBy)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(splitBy, stored.Limit.SplitBy) {
		return fixed("spec.limit.splitBy", "limit.splitBy")
	}
	filters, err := normalizeBudgetFilters(withProjectFilter(req.Filters, req.ProjectId))
	if err != nil {
		return err
	}
	if len(filters) != len(stored.Filters) || (len(filters) > 0 && !reflect.DeepEqual(filters, stored.Filters)) {
		return fixed("spec.filters", "Filters")
	}
	return nil
}

// update changes a budget's editable fields; Update and Import share it.
func (s *budgetService) update(ctx context.Context, ouID string, b *gwmodels.Budget, req budgetUpdateRequest) (*gwspec.Budget, error) {
	name := b.Name
	cfg := b.Configuration
	displayName, err := budgetDisplayName(req.DisplayName, name)
	if err != nil {
		return nil, err
	}
	if req.Description != nil && len([]rune(*req.Description)) > maxBudgetDescription {
		return nil, invalidBudget("description", "description must be at most %d characters.", maxBudgetDescription)
	}
	status := budgetStatusActive
	if req.Status != nil {
		if *req.Status != budgetStatusActive && *req.Status != budgetStatusInactive {
			return nil, invalidBudget("status", "status must be active or inactive.")
		}
		status = *req.Status
	}
	// Currency, period and splitBy are fixed: the new amount and action keep them.
	currency := cfg.Limit.Currency
	limit, err := normalizeBudgetLimit(gwspec.BudgetLimit{
		Amount: req.Limit.Amount, Currency: &currency, Period: cfg.Limit.Period, Action: req.Limit.Action, BlockAt: req.Limit.BlockAt,
	})
	if err != nil {
		return nil, err
	}
	limit.SplitBy = cfg.Limit.SplitBy
	exemptions, err := normalizeBudgetExemptions(req.Exemptions)
	if err != nil {
		return nil, err
	}
	warnings, err := normalizeBudgetWarnings(req.Warnings)
	if err != nil {
		return nil, err
	}
	labels, err := budgetLabels(req.Labels)
	if err != nil {
		return nil, err
	}
	if b.Origin != gwmodels.BudgetOriginControlPlane && !gatewayFieldsUnchanged(&cfg, status, limit, exemptions) {
		// A gateway's budget: the gateway owns what it enforces; only Agent Manager's
		// fields, which gateways never get, change here.
		return nil, &BudgetError{
			Code: gwerr.ErrCodeBudgetReadOnly,
			Message: fmt.Sprintf("Budget %s was created on a gateway: status, limit and exemptions can be changed only "+
				"there. displayName, description and warnings can be changed here.", name),
		}
	}
	cfg.DisplayName, cfg.Description, cfg.Status = displayName, req.Description, status
	cfg.Limit, cfg.Exemptions, cfg.Warnings, cfg.Labels = limit, exemptions, warnings, labels
	if b.ProjectName != "" {
		if err := ownByProject(&cfg, b.ProjectName); err != nil {
			return nil, err
		}
	}
	if err := s.checkEditableReferences(ctx, ouID, &cfg); err != nil {
		return nil, err
	}
	b.Configuration = cfg
	// A gateway's budget changes only in fields gateways never get: nothing to send.
	sendToGateways := b.Origin == gwmodels.BudgetOriginControlPlane
	if err := s.budgetRepo.Update(ctx, b); err != nil {
		if errors.Is(err, gwerr.ErrBudgetNotFound) {
			return nil, budgetNotFound(name)
		}
		return nil, err
	}
	if sendToGateways {
		s.gateways.SendBudget(ctx, b, false)
	}
	if s.warner != nil {
		// A lower limit or a new threshold may already be crossed.
		s.warner.CheckBudget(ctx, b)
	}
	return s.response(ctx, ouID, b)
}

// gatewayFieldsUnchanged reports whether an update leaves a budget's enforced fields
// (status, limit, exemptions) as stored. A stored block limit with no blockAt blocks at 100%.
func gatewayFieldsUnchanged(stored *gwmodels.BudgetConfig, status string, limit gwmodels.BudgetLimit, exemptions []gwmodels.BudgetExemption) bool {
	storedLimit := stored.Limit
	if storedLimit.Action == budgetActionBlock && storedLimit.BlockAt == nil {
		blockAt := int32(budgetDefaultBlockAt)
		storedLimit.BlockAt = &blockAt
	}
	storedStatus := stored.Status
	if storedStatus == "" {
		storedStatus = budgetStatusActive
	}
	return storedStatus == status && reflect.DeepEqual(storedLimit, limit) &&
		len(stored.Exemptions) == len(exemptions) && (len(exemptions) == 0 || reflect.DeepEqual(stored.Exemptions, exemptions))
}

func (s *budgetService) Delete(ctx context.Context, ouID, name string) error {
	b, err := s.writableBudget(ctx, ouID, name)
	if err != nil {
		return err
	}
	if err := s.budgetRepo.Delete(ctx, ouID, name); err != nil {
		if errors.Is(err, gwerr.ErrBudgetNotFound) {
			return budgetNotFound(name)
		}
		return err
	}
	s.gateways.SendBudgetDeleted(ctx, b)
	modkit.AuditResource(ctx, gwaudit.ResourceBudget, b.ID.String(), name)
	modkit.AuditDetail(ctx, "budgetName", name)
	return nil
}

// budgetListFilterTypes are the types a list filter can name.
var budgetListFilterTypes = []string{budgetTypeProject, budgetTypeAgent, budgetTypeAPIKey, budgetTypeLLMProvider}

// budgetListStatusLimitReached lists budgets with one shared limit whose spend this period
// has reached the point they act at (blockAt for Block, the amount for Allow).
const budgetListStatusLimitReached = "limitReached"

// maxBudgetSearch is the longest search text a budget list takes.
const maxBudgetSearch = 100

var budgetListStatuses = []string{budgetStatusActive, budgetStatusInactive, budgetListStatusLimitReached}

// budgetSearchMatches reports whether the budget's name, display name or one of its labels
// contains the search text (lowercase); an empty search matches every budget.
func budgetSearchMatches(b *gwmodels.Budget, search string) bool {
	if search == "" ||
		strings.Contains(strings.ToLower(b.Name), search) ||
		strings.Contains(strings.ToLower(b.Configuration.DisplayName), search) {
		return true
	}
	for _, l := range b.Configuration.Labels {
		if strings.Contains(strings.ToLower(l), search) {
			return true
		}
	}
	return false
}

// budgetHasLabels reports whether the budget has every label asked for (AND).
func budgetHasLabels(cfg *gwmodels.BudgetConfig, labels []string) bool {
	for _, want := range labels {
		if !slices.Contains(cfg.Labels, want) {
			return false
		}
	}
	return true
}

// budgetLabelPattern is one label word: letters, digits, '.', '_' or '-', starting and
// ending with a letter or digit; no spaces.
var budgetLabelPattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]{0,61}[a-zA-Z0-9])?$`)

// maxBudgetLabels is the most labels a budget may have.
const maxBudgetLabels = 10

// budgetLabels checks a budget's metadata.labels: at most 10 one-word labels, no repeats.
func budgetLabels(labels []string) ([]string, error) {
	if len(labels) > maxBudgetLabels {
		return nil, invalidBudget("metadata.labels", "A budget can have at most %d labels.", maxBudgetLabels)
	}
	seen := map[string]bool{}
	for _, l := range labels {
		if !budgetLabelPattern.MatchString(l) {
			return nil, invalidBudget("metadata.labels",
				"Label %q must be one word of 1-63 letters, digits, '.', '_' or '-', starting and ending with a letter or digit.", l)
		}
		if seen[l] {
			return nil, invalidBudget("metadata.labels", "Label %q is listed twice.", l)
		}
		seen[l] = true
	}
	if len(labels) == 0 {
		return nil, nil
	}
	return labels, nil
}

// budgetLimitReached reports whether a budget with one shared limit has spent up to the
// point it acts at this period. A split budget has a limit per value, so it never does.
func budgetLimitReached(b *gwspec.Budget) bool {
	if b.Spec.Limit.SplitBy != nil || b.Status == nil || b.Status.CurrentPeriod == nil {
		return false
	}
	limit, ok := new(big.Rat).SetString(b.Spec.Limit.Amount)
	spent, ok2 := new(big.Rat).SetString(b.Status.CurrentPeriod.Spent)
	if !ok || !ok2 || limit.Sign() <= 0 {
		return false
	}
	point := new(big.Rat).Set(limit)
	if b.Spec.Limit.Action == budgetActionBlock && b.Spec.Limit.BlockAt != nil {
		point.Mul(point, big.NewRat(int64(*b.Spec.Limit.BlockAt), 100))
	}
	return spent.Cmp(point) >= 0
}

type budgetListFilter struct{ typ, id string }

// parseBudgetFilters reads type:id list filters.
func parseBudgetFilters(filters []string) ([]budgetListFilter, error) {
	out := make([]budgetListFilter, 0, len(filters))
	for i, f := range filters {
		typ, id, ok := strings.Cut(f, ":")
		if !ok || id == "" || !containsString(budgetListFilterTypes, typ) {
			return nil, invalidBudget(fmt.Sprintf("filter[%d]", i),
				"filter must be type:id, with type project, agent, apiKey or llmProvider.")
		}
		out = append(out, budgetListFilter{typ: typ, id: id})
	}
	return out, nil
}

// budgetMatches reports whether the budget's filters or exemptions name every list
// filter's value.
func budgetMatches(c *gwmodels.BudgetConfig, filters []budgetListFilter) bool {
	for _, f := range filters {
		if !budgetNames(c, f) {
			return false
		}
	}
	return true
}

// budgetNames reports whether a filter or exemption of the budget names the list filter's
// value; a project is also named by an agent filter or exemption of that project. An
// id-less filter (any value) names no value.
func budgetNames(c *gwmodels.BudgetConfig, f budgetListFilter) bool {
	// agent:<project>/<agent>: the budgets that count that agent's usage, those naming
	// it, or naming its project and no agent.
	if project, agent, ok := strings.Cut(f.id, "/"); ok && f.typ == budgetTypeAgent {
		return budgetCountsAgent(c, project, agent)
	}
	is := func(p *string) bool { return p != nil && *p == f.id }
	for _, x := range c.Filters {
		if (x.Type == f.typ && is(x.ID)) || (f.typ == budgetTypeProject && is(x.Project)) {
			return true
		}
	}
	for _, x := range c.Exemptions {
		if (x.Type == f.typ && is(x.ID)) || (f.typ == budgetTypeProject && is(x.Project)) {
			return true
		}
	}
	return false
}

// budgetCountsAgent reports whether the budget names the agent, or names the agent's
// project without naming any agent. An id-less agent filter (any agent) counts the agent
// when the budget's project filter names its project, or when the budget names no project
// at all (it counts every agent); an id-less project filter names no project. Other
// filters (provider, API key, attributes) are not considered.
func budgetCountsAgent(c *gwmodels.BudgetConfig, project, agent string) bool {
	projectNamed, otherProject, anyAgent := false, false, false
	for _, x := range c.Filters {
		switch {
		case x.Type == budgetTypeAgent && x.ID == nil:
			anyAgent = true
		case x.Type == budgetTypeAgent:
			return x.Project != nil && *x.Project == project && *x.ID == agent
		case x.Type == budgetTypeProject && x.ID != nil:
			projectNamed = *x.ID == project
			otherProject = !projectNamed
		}
	}
	if anyAgent {
		return !otherProject
	}
	return projectNamed
}

// budgetDisplayName is the display name given, or the budget's name when none is.
func budgetDisplayName(in *string, name string) (string, error) {
	if in == nil || strings.TrimSpace(*in) == "" {
		return name, nil
	}
	displayName := strings.TrimSpace(*in)
	if len([]rune(displayName)) > maxBudgetDisplayName {
		return "", invalidBudget("displayName", "displayName must be at most %d characters.", maxBudgetDisplayName)
	}
	return displayName, nil
}

// normalizeBudget checks the payload and returns the budget's settings
// with defaults filled in.
func normalizeBudget(req budgetRequest) (*gwmodels.BudgetConfig, error) {
	if !budgetNamePattern.MatchString(req.Name) {
		return nil, invalidBudget("name", "name must be 1-63 lowercase letters, digits or '-', starting and ending with a letter or digit.")
	}
	displayName, err := budgetDisplayName(req.DisplayName, req.Name)
	if err != nil {
		return nil, err
	}
	if req.Description != nil && len([]rune(*req.Description)) > maxBudgetDescription {
		return nil, invalidBudget("description", "description must be at most %d characters.", maxBudgetDescription)
	}
	limit, err := normalizeBudgetLimit(req.Limit)
	if err != nil {
		return nil, err
	}
	if limit.SplitBy, err = normalizeBudgetSplit(req.Limit.SplitBy); err != nil {
		return nil, err
	}
	filters, err := normalizeBudgetFilters(req.Filters)
	if err != nil {
		return nil, err
	}
	if limit.SplitBy != nil && limit.SplitBy.Type != budgetTypeAttribute {
		for _, f := range filters {
			// As on gateways: one value cannot be split. A filter without an id (any value)
			// may be split by its own type: one limit for each.
			if f.Type == limit.SplitBy.Type && f.ID != nil && *f.ID != "" {
				return nil, invalidBudget("limit.splitBy.type",
					"limit.splitBy cannot be %s: a filter already narrows it to one %s.", f.Type, f.Type)
			}
		}
	}
	exemptions, err := normalizeBudgetExemptions(req.Exemptions)
	if err != nil {
		return nil, err
	}
	warnings, err := normalizeBudgetWarnings(req.Warnings)
	if err != nil {
		return nil, err
	}
	status := budgetStatusActive
	if req.Status != nil {
		if *req.Status != budgetStatusActive && *req.Status != budgetStatusInactive {
			return nil, invalidBudget("status", "status must be active or inactive.")
		}
		status = *req.Status
	}
	labels, err := budgetLabels(req.Labels)
	if err != nil {
		return nil, err
	}
	return &gwmodels.BudgetConfig{
		DisplayName: displayName, Description: req.Description, Status: status,
		Limit: limit, Filters: filters, Exemptions: exemptions, Warnings: warnings, Labels: labels,
	}, nil
}

func normalizeBudgetLimit(in gwspec.BudgetLimit) (gwmodels.BudgetLimit, error) {
	out := gwmodels.BudgetLimit{Amount: in.Amount, Currency: budgetCurrencyUSD, Period: in.Period, Action: in.Action}
	// The pattern goes first: it refuses exponents and long strings before they are parsed.
	if !budgetAmountPattern.MatchString(in.Amount) {
		return out, invalidBudget("limit.amount", "limit.amount must be a decimal above zero, with at most 12 whole digits, 2 decimals and no leading zeros, e.g. \"100.00\".")
	}
	if amount, ok := new(big.Rat).SetString(in.Amount); !ok || amount.Sign() <= 0 {
		return out, invalidBudget("limit.amount", "limit.amount must be a decimal above zero, with at most 12 whole digits, 2 decimals and no leading zeros, e.g. \"100.00\".")
	}
	if in.Currency != nil && *in.Currency != budgetCurrencyUSD {
		return out, invalidBudget("limit.currency", "limit.currency must be USD.")
	}
	if !containsString(budgetPeriods, in.Period) {
		return out, invalidBudget("limit.period", "limit.period must be day, week or month.")
	}
	switch in.Action {
	case budgetActionBlock:
		blockAt := int32(budgetDefaultBlockAt)
		if in.BlockAt != nil {
			if *in.BlockAt < 1 || *in.BlockAt > 100 {
				return out, invalidBudget("limit.blockAt", "limit.blockAt must be a whole percentage from 1 to 100.")
			}
			blockAt = *in.BlockAt
		}
		out.BlockAt = &blockAt
	case budgetActionAllow:
		if in.BlockAt != nil {
			return out, invalidBudget("limit.blockAt", "limit.blockAt applies only to the block action.")
		}
	default:
		return out, invalidBudget("limit.action", "limit.action must be block or allow.")
	}
	return out, nil
}

// normalizeBudgetSplit checks limit.splitBy; nil is one shared limit.
func normalizeBudgetSplit(in *gwspec.BudgetSplit) (*gwmodels.BudgetSplit, error) {
	if in == nil {
		return nil, nil //nolint:nilnil // no split: one shared limit
	}
	const field = "limit.splitBy"
	if !containsString(budgetFilterTypes, in.Type) {
		return nil, invalidBudget(field+".type", "limit.splitBy.type must be llmProvider, project, agent, apiKey or attribute.")
	}
	if in.Type != budgetTypeLLMProvider && !containsString(gatewayConsumerTypes, in.Type) {
		return nil, unsupportedConsumer(field+".type", in.Type)
	}
	if in.Type != budgetTypeAttribute {
		if in.Expression != nil {
			return nil, invalidBudget(field+".expression", "Only an attribute split takes an expression.")
		}
		return &gwmodels.BudgetSplit{Type: in.Type}, nil
	}
	if in.Expression == nil || strings.TrimSpace(*in.Expression) == "" {
		return nil, invalidBudget(field+".expression", "An attribute split needs an expression.")
	}
	if len([]rune(*in.Expression)) > maxBudgetExpression {
		return nil, invalidBudget(field+".expression", "expression must be at most %d characters.", maxBudgetExpression)
	}
	if err := checkBudgetExpression(field+".expression", *in.Expression, true); err != nil {
		return nil, err
	}
	return &gwmodels.BudgetSplit{Type: in.Type, Expression: in.Expression}, nil
}

// normalizeBudgetFilters checks the filters (combined with AND): each type except
// attribute at most once; llmProvider takes an id; project and apiKey take an id or none,
// agent an id and its project or neither; attribute a bool expression. A project, agent
// or apiKey filter without an id matches any value ("is set"): usage that carries one.
func normalizeBudgetFilters(in []gwspec.BudgetFilter) ([]gwmodels.BudgetFilter, error) {
	if len(in) > maxBudgetFilters {
		return nil, invalidBudget("filters", "filters takes at most %d entries.", maxBudgetFilters)
	}
	out := make([]gwmodels.BudgetFilter, 0, len(in))
	seen := map[string]bool{}
	empty := func(p *string) bool { return p == nil || strings.TrimSpace(*p) == "" }
	for i, f := range in {
		field := fmt.Sprintf("filters[%d]", i)
		if !containsString(budgetFilterTypes, f.Type) {
			return nil, invalidBudget(field+".type", "Filter type must be llmProvider, project, agent, apiKey or attribute.")
		}
		if f.Type != budgetTypeLLMProvider && !containsString(gatewayConsumerTypes, f.Type) {
			return nil, unsupportedConsumer(field+".type", f.Type)
		}
		if f.Type == budgetTypeAttribute {
			if empty(f.Expression) {
				return nil, invalidBudget(field+".expression", "An attribute filter needs an expression.")
			}
			if f.Id != nil {
				return nil, invalidBudget(field+".id", "An attribute filter takes an expression, not an id.")
			}
			if f.Project != nil {
				return nil, invalidBudget(field+".project", "Only an agent filter takes a project.")
			}
			if len([]rune(*f.Expression)) > maxBudgetExpression {
				return nil, invalidBudget(field+".expression", "expression must be at most %d characters.", maxBudgetExpression)
			}
			if err := checkBudgetExpression(field+".expression", *f.Expression, false); err != nil {
				return nil, err
			}
		} else {
			if seen[f.Type] {
				return nil, invalidBudget(field+".type", "Each filter type except attribute can be used once.")
			}
			seen[f.Type] = true
			if f.Expression != nil {
				return nil, invalidBudget(field+".expression", "Only an attribute filter takes an expression.")
			}
			if f.Id != nil && strings.TrimSpace(*f.Id) == "" {
				return nil, invalidBudget(field+".id", "A filter's id must not be empty; leave it out to match any %s.", f.Type)
			}
			if f.Type == budgetTypeLLMProvider && f.Id == nil {
				return nil, invalidBudget(field+".id", "An llmProvider filter needs an id.")
			}
			// An agent also names its project, as an agent exemption does: agent names are
			// unique only per project. Neither given is any agent.
			if f.Type == budgetTypeAgent && (f.Id == nil) != empty(f.Project) {
				return nil, invalidBudget(field+".project", "An agent filter takes both its project and id, or neither (any agent).")
			}
			if f.Type != budgetTypeAgent && f.Project != nil {
				return nil, invalidBudget(field+".project", "Only an agent filter takes a project.")
			}
		}
		project := f.Project
		if f.Type == budgetTypeAgent && f.Id == nil {
			project = nil
		}
		out = append(out, gwmodels.BudgetFilter{Type: f.Type, ID: f.Id, Project: project, Expression: f.Expression})
	}
	return out, nil
}

func normalizeBudgetExemptions(in []gwspec.BudgetExemption) ([]gwmodels.BudgetExemption, error) {
	if len(in) > maxBudgetExemptions {
		return nil, invalidBudget("exemptions", "exemptions takes at most %d entries.", maxBudgetExemptions)
	}
	out := make([]gwmodels.BudgetExemption, 0, len(in))
	empty := func(p *string) bool { return p == nil || *p == "" }
	for i, e := range in {
		field := fmt.Sprintf("exemptions[%d]", i)
		if containsString([]string{budgetTypeProject, budgetTypeAgent, budgetTypeAttribute}, e.Type) &&
			!containsString(gatewayConsumerTypes, e.Type) {
			return nil, unsupportedConsumer(field+".type", e.Type)
		}
		switch e.Type {
		case budgetTypeProject:
			if empty(e.Id) || e.Project != nil || e.Expression != nil {
				return nil, invalidBudget(field, "A project exemption takes only an id.")
			}
		case budgetTypeAgent:
			if empty(e.Id) || empty(e.Project) || e.Expression != nil {
				return nil, invalidBudget(field, "An agent exemption takes an id and its project.")
			}
		case budgetTypeAttribute:
			if empty(e.Expression) || e.Id != nil || e.Project != nil {
				return nil, invalidBudget(field, "An attribute exemption takes only an expression.")
			}
			if len([]rune(*e.Expression)) > maxBudgetExpression {
				return nil, invalidBudget(field+".expression", "expression must be at most %d characters.", maxBudgetExpression)
			}
			if err := checkBudgetExpression(field+".expression", *e.Expression, false); err != nil {
				return nil, err
			}
		default:
			return nil, invalidBudget(field+".type", "Exemption type must be project, agent or attribute.")
		}
		out = append(out, gwmodels.BudgetExemption{Type: e.Type, ID: e.Id, Project: e.Project, Expression: e.Expression})
	}
	return out, nil
}

func normalizeBudgetWarnings(in *gwspec.BudgetWarnings) (*gwmodels.BudgetWarnings, error) {
	if in == nil {
		return nil, nil //nolint:nilnil // warnings are optional
	}
	if len(in.Thresholds) < 1 || len(in.Thresholds) > maxBudgetThresholds {
		return nil, invalidBudget("warnings.thresholds", "warnings.thresholds takes 1-%d percentages.", maxBudgetThresholds)
	}
	seenThreshold := map[int32]bool{}
	for i, t := range in.Thresholds {
		if t < 1 || t > maxBudgetThreshold || seenThreshold[t] {
			return nil, invalidBudget(fmt.Sprintf("warnings.thresholds[%d]", i),
				"Thresholds must be unique whole percentages from 1 to %d.", maxBudgetThreshold)
		}
		seenThreshold[t] = true
	}
	if len(in.Channels) < 1 || len(in.Channels) > maxBudgetChannels {
		return nil, invalidBudget("warnings.channels", "warnings.channels takes 1-%d channels.", maxBudgetChannels)
	}
	// Channels are named, and stored by name as received (the kind as received).
	channels := make([]string, 0, len(in.Channels))
	seenChannel := map[string]bool{}
	for i, raw := range in.Channels {
		name := strings.TrimSpace(raw)
		if name == "" || seenChannel[name] {
			return nil, invalidBudget(fmt.Sprintf("warnings.channels[%d]", i), "Channel names must be non-empty and unique.")
		}
		seenChannel[name] = true
		channels = append(channels, name)
	}
	return &gwmodels.BudgetWarnings{Thresholds: in.Thresholds, Channels: channels}, nil
}

// checkReferences checks that the providers, projects, agents and channels the budget
// names exist in the organization. An id-less filter (any value) names none.
func (s *budgetService) checkReferences(ctx context.Context, ouID string, b *gwmodels.BudgetConfig) error {
	for i, f := range b.Filters {
		if f.ID == nil {
			continue
		}
		field := fmt.Sprintf("filters[%d].id", i)
		var err error
		switch f.Type {
		case budgetTypeLLMProvider:
			err = s.checkArtifact(ouID, gwmodels.KindLLMProvider, *f.ID, field, "LLM provider")
		case budgetTypeProject:
			err = s.checkProject(ctx, ouID, *f.ID, field)
		case budgetTypeAgent:
			err = s.checkAgent(ctx, ouID, *f.Project, *f.ID, field)
		}
		if err != nil {
			return err
		}
	}
	return s.checkEditableReferences(ctx, ouID, b)
}

// budgetProject is the project a budget counts, if any: its project filter's, else its
// agent filter's project.
func budgetProject(b *gwmodels.BudgetConfig) string {
	agentProject := ""
	for _, f := range b.Filters {
		switch {
		case f.Type == budgetTypeProject && f.ID != nil:
			return *f.ID
		case f.Type == budgetTypeAgent && f.Project != nil:
			agentProject = *f.Project
		}
	}
	return agentProject
}

// checkEditableReferences checks the references an update can change: the exempt
// projects and agents, and the warning channels.
func (s *budgetService) checkEditableReferences(ctx context.Context, ouID string, b *gwmodels.BudgetConfig) error {
	for i, e := range b.Exemptions {
		field := fmt.Sprintf("exemptions[%d].id", i)
		var err error
		switch e.Type {
		case budgetTypeProject:
			err = s.checkProject(ctx, ouID, *e.ID, field)
		case budgetTypeAgent:
			err = s.checkAgent(ctx, ouID, *e.Project, *e.ID, field)
		}
		if err != nil {
			return err
		}
	}
	return s.checkChannels(ctx, ouID, budgetProject(b), b.Warnings)
}

// checkChannels checks each warning channel, given by name, exists and is available to
// the budget: availability all or org, or projects when the budget's
// project (budgetProject) is one of them.
func (s *budgetService) checkChannels(ctx context.Context, ouID, project string, warnings *gwmodels.BudgetWarnings) error {
	if s.channels == nil || warnings == nil || len(warnings.Channels) == 0 {
		return nil
	}
	unusable, err := s.channels.Unusable(ctx, ouID, project, warnings.Channels)
	if err != nil {
		return fmt.Errorf("failed to check notification channels: %w", err)
	}
	if len(unusable) == 0 {
		return nil
	}
	u := unusable[0]
	field := fmt.Sprintf("warnings.channels[%d]", slices.Index(warnings.Channels, u.Name))
	if u.Missing {
		return &BudgetError{
			Code: gwerr.ErrCodeChannelNotAvailable, Field: field,
			Message: fmt.Sprintf("Notification channel '%s' was not found.", u.Name),
		}
	}
	return &BudgetError{
		Code: gwerr.ErrCodeChannelNotAvailable, Field: field,
		Message: fmt.Sprintf("Notification channel %s is not available to this budget.", u.Name),
	}
}

func (s *budgetService) checkArtifact(ouID, kind, handle, field, what string) error {
	exists, err := s.artifactRepo.Exists(kind, handle, ouID)
	if err != nil {
		return fmt.Errorf("failed to check %s %q: %w", what, handle, err)
	}
	if !exists {
		return budgetReferenceNotFound(field, "%s '%s' was not found.", what, handle)
	}
	return nil
}

func (s *budgetService) checkProject(ctx context.Context, ouID, name, field string) error {
	if _, err := LookupProject(ctx, s.core, ouID, name); err != nil {
		if errors.Is(err, gwerr.ErrProjectNotFound) {
			return budgetReferenceNotFound(field, "Project '%s' was not found.", name)
		}
		return fmt.Errorf("failed to get project %q: %w", name, err)
	}
	return nil
}

func (s *budgetService) checkAgent(ctx context.Context, ouID, project, name, field string) error {
	if _, err := LookupAgent(ctx, s.core, ouID, project, name); err != nil {
		if errors.Is(err, gwerr.ErrAgentNotFound) || errors.Is(err, gwerr.ErrProjectNotFound) {
			return budgetReferenceNotFound(field, "Agent '%s' was not found in project '%s'.", name, project)
		}
		return fmt.Errorf("failed to get agent %q: %w", name, err)
	}
	return nil
}

// BudgetResponse is a stored budget as the API returns it.
// BudgetAsKind is a stored budget as the API returns it: the Budget kind, with the org-id
// annotation, the project-id annotation for a project budget and the server's fields in
// status.
func BudgetAsKind(b *gwmodels.Budget) *gwspec.Budget {
	c := b.Configuration
	filters := make([]gwspec.BudgetFilter, 0, len(c.Filters))
	for _, x := range c.Filters {
		filters = append(filters, gwspec.BudgetFilter{Type: x.Type, Id: x.ID, Project: x.Project, Expression: x.Expression})
	}
	exemptions := make([]gwspec.BudgetExemption, 0, len(c.Exemptions))
	for _, x := range c.Exemptions {
		exemptions = append(exemptions, gwspec.BudgetExemption{Type: x.Type, Id: x.ID, Project: x.Project, Expression: x.Expression})
	}
	currency := c.Limit.Currency
	displayName, status := c.DisplayName, c.Status
	budgetSpec := gwspec.BudgetSpec{
		DisplayName: &displayName, Description: c.Description, Status: &status,
		Limit:   gwspec.BudgetLimit{Amount: c.Limit.Amount, Currency: &currency, Period: c.Limit.Period, Action: c.Limit.Action, BlockAt: c.Limit.BlockAt},
		Filters: filters, Exemptions: exemptions,
	}
	if c.Limit.SplitBy != nil {
		budgetSpec.Limit.SplitBy = &gwspec.BudgetSplit{Type: c.Limit.SplitBy.Type, Expression: c.Limit.SplitBy.Expression}
	}
	if c.Warnings != nil {
		budgetSpec.Warnings = &gwspec.BudgetWarnings{Thresholds: c.Warnings.Thresholds, Channels: c.Warnings.Channels}
	}
	metadata := gwspec.BudgetMetadata{Name: b.Name}
	metadata.Labels = c.Labels
	annotations := map[string]string{}
	if b.OrgName != "" {
		annotations[AnnotationOrgID] = b.OrgName
	}
	if b.ProjectName != "" {
		annotations[AnnotationProjectID] = b.ProjectName
	}
	if len(annotations) > 0 {
		metadata.Annotations = &annotations
	}
	serverStatus := gwspec.NewBudgetServerStatus(b.Origin.API(), b.CreatedAt, b.UpdatedAt)
	if b.CreatedBy != "" {
		serverStatus.CreatedBy = &b.CreatedBy
	}
	kind := gwspec.NewBudget(budgetAPIVersion, gwmodels.KindBudget, metadata, budgetSpec)
	kind.Status = serverStatus
	return kind
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
