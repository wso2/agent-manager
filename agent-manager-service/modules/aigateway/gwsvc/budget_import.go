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
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

// CSV import of budgets (Interfaces 2.5): one budget per row, the same fields and checks
// as POST /budgets, saved all or none.

// MaxBudgetImportBytes is the largest CSV file an import takes.
const MaxBudgetImportBytes = 1 << 20

// maxBudgetImportRows is the most budgets one import creates: they are saved in one transaction.
const maxBudgetImportRows = 500

// CSV columns. Lists use ';'; attributes is a JSON array of CEL expressions and exemptions
// a JSON array of the payload's objects.
const (
	csvName              = "name"
	csvProjectID         = "projectId"
	csvDisplayName       = "displayName"
	csvDescription       = "description"
	csvAmount            = "amount"
	csvPeriod            = "period"
	csvAction            = "action"
	csvBlockAt           = "blockAt"
	csvSplitBy           = "splitBy"
	csvSplitByExpression = "splitByExpression"
	csvProvider          = "llmProvider"
	csvProject           = "project"
	csvAgent             = "agent"
	csvAPIKey            = "apiKey"
	csvAttributes        = "attributes"
	csvExemptions        = "exemptions"
	csvThresholds        = "thresholds"
	csvChannels          = "channels"
	csvLabels            = "labels"
)

// csvAnyValue in the project, agent or apiKey column is the id-less filter (any value).
const csvAnyValue = "*"

var (
	budgetImportColumns = []string{
		csvName, csvProjectID, csvDisplayName, csvDescription,
		csvAmount, csvPeriod, csvAction, csvBlockAt, csvSplitBy, csvSplitByExpression,
		csvProvider, csvProject, csvAgent, csvAPIKey,
		csvAttributes, csvExemptions, csvThresholds, csvChannels, csvLabels,
	}
	budgetImportRequired = []string{csvName, csvAmount, csvPeriod, csvAction}

	// budgetFilterColumns are the columns of the filters named by id, in the order they
	// become filters; attribute filters follow them.
	budgetFilterColumns = []struct{ typ, column string }{
		{budgetTypeLLMProvider, csvProvider},
		{budgetTypeProject, csvProject},
		{budgetTypeAgent, csvAgent},
		{budgetTypeAPIKey, csvAPIKey},
	}

	budgetFieldIndex = regexp.MustCompile(`^filters\[(\d+)\]`)
	utf8BOM          = []byte{0xEF, 0xBB, 0xBF}
)

// invalidCSV refuses a file that cannot be read as a whole.
func invalidCSV(format string, args ...any) error {
	return &BudgetError{Code: gwerr.ErrCodeInvalidCSV, Message: fmt.Sprintf(format, args...)}
}

// What an import does with a row naming an existing budget (onExisting).
const (
	OnExistingFail   = "fail"
	OnExistingSkip   = "skip"
	OnExistingUpdate = "update"
)

// Row actions.
const (
	importCreate = "create"
	importUpdate = "update"
	importSkip   = "skip"
)

// importRow is one CSV row on its way to becoming a budget.
type importRow struct {
	row    int32
	name   string
	req    budgetRequest
	budget *gwmodels.Budget // the budget the row creates, or the stored budget it updates
	action string
	errs   []gwspec.BudgetImportRowError
}

func (r *importRow) fail(column, code, message string) {
	e := gwspec.BudgetImportRowError{Code: code, Message: message}
	if column != "" {
		e.Column = &column
	}
	r.errs = append(r.errs, e)
}

func (s *budgetService) Import(ctx context.Context, ouID, createdBy string, file io.Reader, dryRun bool, onExisting string) (*gwspec.BudgetImportResult, error) {
	if ouID == "" {
		return nil, gwerr.ErrOrganizationNotFound
	}
	if onExisting == "" {
		onExisting = OnExistingFail
	}
	if onExisting != OnExistingFail && onExisting != OnExistingSkip && onExisting != OnExistingUpdate {
		return nil, invalidBudget("onExisting", "onExisting must be fail, skip or update.")
	}
	header, records, err := readBudgetCSV(file)
	if err != nil {
		return nil, err
	}

	rows := make([]*importRow, 0, len(records))
	firstRow := map[string]int32{}
	for i, record := range records {
		cells := make(map[string]string, len(header))
		for c, column := range header {
			cells[column] = strings.TrimSpace(record[c])
		}
		row := &importRow{row: int32(i + 2), name: cells[csvName]}
		rows = append(rows, row)
		row.req = budgetRequestFromCSV(cells, row)
		checkCSVColumns(cells, row)
		if row.name != "" {
			if first, seen := firstRow[row.name]; seen {
				row.fail(csvName, gwerr.ErrCodeBudgetAlreadyExists, fmt.Sprintf("Row %d already uses the name %s.", first, row.name))
			} else {
				firstRow[row.name] = row.row
			}
		}
		if len(row.errs) > 0 {
			continue
		}
		budget, err := s.prepareBudget(ctx, ouID, createdBy, row.req)
		var be *BudgetError
		switch {
		case errors.As(err, &be):
			row.fail(budgetCSVColumn(be.Field, row.req), be.Code, be.Message)
		case err != nil:
			return nil, err
		default:
			row.budget, row.action = budget, importCreate
		}
	}
	if err := s.resolveExisting(ctx, ouID, rows, onExisting); err != nil {
		return nil, err
	}

	result := importResult(rows, dryRun)
	if dryRun || result.Summary.Error > 0 {
		return result, nil
	}
	var creates, updates []*gwmodels.Budget
	var names []string
	for _, row := range rows {
		switch row.action {
		case importCreate:
			creates, names = append(creates, row.budget), append(names, row.name)
		case importUpdate:
			updates = append(updates, row.budget)
		}
	}
	if err := s.budgetRepo.CreateManyForControlPlane(ctx, creates, names, updates); err != nil {
		// Another request took a name after the check: nothing was saved.
		var taken *gwrepo.BudgetNameTakenError
		if !errors.As(err, &taken) {
			return nil, err
		}
		for _, row := range rows {
			if row.name == taken.Name {
				row.fail(csvName, gwerr.ErrCodeBudgetAlreadyExists, budgetNameTaken(row.name).Error())
			}
		}
		return importResult(rows, dryRun), nil
	}
	result.Applied = true
	for _, b := range creates {
		s.gateways.SendBudget(ctx, b, true)
	}
	for _, b := range updates {
		s.gateways.SendBudget(ctx, b, false)
		if s.warner != nil {
			// A lower limit or a new threshold may already be crossed.
			s.warner.CheckBudget(ctx, b)
		}
	}
	return result, nil
}

// readBudgetCSV reads the header and the rows, refusing a file that cannot be read as a
// whole: not CSV, unknown, repeated or missing required columns, no rows or too many.
func readBudgetCSV(file io.Reader) ([]string, [][]string, error) {
	br := bufio.NewReader(file)
	if bom, _ := br.Peek(len(utf8BOM)); bytes.Equal(bom, utf8BOM) {
		_, _ = br.Discard(len(utf8BOM))
	}
	r := csv.NewReader(br)
	header, err := r.Read()
	if errors.Is(err, io.EOF) {
		return nil, nil, invalidCSV("The file is empty: it needs a header row and at least one budget.")
	}
	if err != nil {
		return nil, nil, invalidCSV("The file is not valid CSV: %v.", err)
	}
	seen := map[string]bool{}
	for i, column := range header {
		column = strings.TrimSpace(column)
		header[i] = column
		if !containsString(budgetImportColumns, column) {
			return nil, nil, invalidCSV("Unknown column %q. Columns: %s.", column, strings.Join(budgetImportColumns, ", "))
		}
		if seen[column] {
			return nil, nil, invalidCSV("Column %s appears twice.", column)
		}
		seen[column] = true
	}
	for _, column := range budgetImportRequired {
		if !seen[column] {
			return nil, nil, invalidCSV("Column %s is required.", column)
		}
	}
	var records [][]string
	for {
		record, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, invalidCSV("The file is not valid CSV: %v.", err)
		}
		if len(records) == maxBudgetImportRows {
			return nil, nil, invalidCSV("The file has more than %d budgets.", maxBudgetImportRows)
		}
		records = append(records, record)
	}
	if len(records) == 0 {
		return nil, nil, invalidCSV("The file has no budgets: add a row under the header.")
	}
	return header, records, nil
}

// budgetRequestFromCSV maps a row onto the payload of POST /budgets. Cells that cannot be
// read (a number, JSON, an agent) fail their column on row.
func budgetRequestFromCSV(cells map[string]string, row *importRow) budgetRequest {
	optional := func(column string) *string {
		if v := cells[column]; v != "" {
			return &v
		}
		return nil
	}
	req := budgetRequest{
		Name: cells[csvName], ProjectId: optional(csvProjectID),
		DisplayName: optional(csvDisplayName), Description: optional(csvDescription),
		Limit: gwspec.BudgetLimit{Amount: cells[csvAmount], Period: cells[csvPeriod], Action: cells[csvAction]},
	}
	if v := cells[csvBlockAt]; v != "" {
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			row.fail(csvBlockAt, httpx.ErrCodeValidation, "blockAt must be a whole percentage, 1-100.")
		}
		blockAt := int32(n)
		req.Limit.BlockAt = &blockAt
	}
	if cells[csvSplitBy] != "" || cells[csvSplitByExpression] != "" {
		req.Limit.SplitBy = &gwspec.BudgetSplit{Type: cells[csvSplitBy], Expression: optional(csvSplitByExpression)}
	}
	req.Filters = []gwspec.BudgetFilter{}
	for _, c := range budgetFilterColumns {
		v := cells[c.column]
		if v == "" {
			continue
		}
		filter := gwspec.BudgetFilter{Type: c.typ, Id: &v}
		if v == csvAnyValue && c.typ != budgetTypeLLMProvider {
			// '*' is the id-less filter: any project, agent or API key.
			filter.Id = nil
		} else if c.typ == budgetTypeAgent {
			// An agent is <project>/<agent>: agent names are unique only per project.
			project, agent, ok := strings.Cut(v, "/")
			project, agent = strings.TrimSpace(project), strings.TrimSpace(agent)
			if !ok || project == "" || agent == "" || strings.Contains(agent, "/") {
				row.fail(csvAgent, httpx.ErrCodeValidation, "agent must be <project>/<agent>, e.g. online-shop/returns-bot.")
				continue
			}
			filter.Id, filter.Project = &agent, &project
		}
		req.Filters = append(req.Filters, filter)
	}
	if v := cells[csvAttributes]; v != "" {
		var expressions []string
		if err := decodeCSVJSON(v, &expressions); err != nil {
			row.fail(csvAttributes, httpx.ErrCodeValidation, "attributes must be a JSON array of CEL expressions (strings): "+err.Error())
		}
		for _, e := range expressions {
			expression := e
			req.Filters = append(req.Filters, gwspec.BudgetFilter{Type: budgetTypeAttribute, Expression: &expression})
		}
	}
	req.Exemptions = []gwspec.BudgetExemption{}
	if v := cells[csvExemptions]; v != "" {
		if err := decodeCSVJSON(v, &req.Exemptions); err != nil {
			row.fail(csvExemptions, httpx.ErrCodeValidation, "exemptions must be a JSON array of exemptions: "+err.Error())
		}
	}
	if cells[csvThresholds] != "" || cells[csvChannels] != "" {
		warnings := &gwspec.BudgetWarnings{Thresholds: []int32{}, Channels: csvList(cells[csvChannels])}
		for _, v := range csvList(cells[csvThresholds]) {
			n, err := strconv.ParseInt(v, 10, 32)
			if err != nil {
				row.fail(csvThresholds, httpx.ErrCodeValidation, "thresholds must be whole percentages separated by ';', e.g. 80;100.")
				break
			}
			warnings.Thresholds = append(warnings.Thresholds, int32(n))
		}
		req.Warnings = warnings
	}
	if v := cells[csvLabels]; v != "" {
		// One-word labels separated by ';', e.g. shop;finance.
		req.Labels = csvList(v)
	}
	return req
}

// csvList splits a ';' list, dropping empty items.
func csvList(v string) []string {
	out := []string{}
	for _, item := range strings.Split(v, ";") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// decodeCSVJSON reads a JSON cell strictly: unknown fields are errors, as in the payload.
func decodeCSVJSON(v string, out any) error {
	d := json.NewDecoder(strings.NewReader(v))
	d.DisallowUnknownFields()
	return d.Decode(out)
}

// budgetCSVColumn names the CSV column of a payload field, so a row's error points at
// the cell to fix; empty when it is about the whole row.
func budgetCSVColumn(field string, req budgetRequest) string {
	switch {
	case field == "name", field == "orgId", field == "projectId", field == "displayName", field == "description":
		return field
	case field == "metadata.annotations":
		return csvProjectID
	case field == "metadata.labels":
		return csvLabels
	case field == "limit.splitBy.expression":
		return csvSplitByExpression
	case strings.HasPrefix(field, "limit.splitBy"):
		return csvSplitBy
	case strings.HasPrefix(field, "limit."):
		return strings.TrimPrefix(field, "limit.")
	case strings.HasPrefix(field, "exemptions"):
		return csvExemptions
	case strings.HasPrefix(field, "warnings.channels"):
		return csvChannels
	case strings.HasPrefix(field, "warnings"):
		return csvThresholds
	}
	if m := budgetFieldIndex.FindStringSubmatch(field); m != nil {
		i, _ := strconv.Atoi(m[1])
		if i >= len(req.Filters) {
			// The project filter added for the owning project.
			return csvProjectID
		}
		typ := req.Filters[i].Type
		if typ == budgetTypeAttribute {
			return csvAttributes
		}
		for _, c := range budgetFilterColumns {
			if c.typ == typ {
				return c.column
			}
		}
	}
	return ""
}

// resolveExisting decides each valid row whose name is already taken (Interfaces 2.5): a
// name held by another kind always fails; an existing budget fails the row (fail, the
// default), is left as it is (skip), or has its editable fields replaced (update).
func (s *budgetService) resolveExisting(ctx context.Context, ouID string, rows []*importRow, onExisting string) error {
	names := []string{}
	for _, row := range rows {
		if row.budget != nil {
			names = append(names, row.name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	taken, err := s.budgetRepo.TakenNames(ctx, ouID, names)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.budget == nil || !containsString(taken, row.name) {
			continue
		}
		incoming := row.budget
		row.budget, row.action = nil, ""
		existing, err := s.budgetRepo.Get(ctx, ouID, row.name)
		if errors.Is(err, gwerr.ErrBudgetNotFound) {
			row.fail(csvName, gwerr.ErrCodeBudgetAlreadyExists,
				fmt.Sprintf("%s is already the name of another resource in the organization; choose another name.", row.name))
			continue
		}
		if err != nil {
			return err
		}
		switch onExisting {
		case OnExistingSkip:
			row.action = importSkip
		case OnExistingUpdate:
			if column, message := importUpdateRefusal(existing, incoming); message != "" {
				row.fail(column, importUpdateCode(existing), message)
				continue
			}
			updated := *existing
			updated.Configuration = updatedConfig(existing.Configuration, incoming.Configuration)
			row.budget, row.action = &updated, importUpdate
		default:
			row.fail(csvName, gwerr.ErrCodeBudgetAlreadyExists, fmt.Sprintf(
				"Budget %s already exists. Import with onExisting=update to update it, or onExisting=skip to leave it as it is.", row.name))
		}
	}
	return nil
}

// importUpdateRefusal returns why a row cannot update an existing budget, and the column
// to fix: a gateway's budget is changed only on the gateway, and what a budget covers is
// fixed after creation (BR-15).
func importUpdateRefusal(existing, incoming *gwmodels.Budget) (column, message string) {
	if existing.Origin != gwmodels.BudgetOriginControlPlane {
		return csvName, fmt.Sprintf("Budget %s was created on a gateway; change it there.", existing.Name)
	}
	fixed := func(what string) string {
		return fmt.Sprintf("Budget %s already exists with a different %s, which cannot change; create a new budget instead.", existing.Name, what)
	}
	was, now := existing.Configuration, incoming.Configuration
	switch {
	case !sameProject(existing.ProjectID, incoming.ProjectID):
		return csvProjectID, fixed("owning project")
	case was.Limit.Period != now.Limit.Period:
		return csvPeriod, fixed("period")
	case !reflect.DeepEqual(was.Limit.SplitBy, now.Limit.SplitBy):
		return csvSplitBy, fixed("splitBy")
	case !reflect.DeepEqual(was.Filters, now.Filters):
		return "", fixed("set of filters")
	}
	return "", ""
}

func importUpdateCode(existing *gwmodels.Budget) string {
	if existing.Origin != gwmodels.BudgetOriginControlPlane {
		return gwerr.ErrCodeBudgetReadOnly
	}
	return httpx.ErrCodeValidation
}

func sameProject(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// updatedConfig is a stored budget with a row's editable fields, as PUT replaces them; its
// status stays.
func updatedConfig(stored, incoming gwmodels.BudgetConfig) gwmodels.BudgetConfig {
	out := stored
	out.DisplayName, out.Description = incoming.DisplayName, incoming.Description
	out.Limit.Amount, out.Limit.Action, out.Limit.BlockAt = incoming.Limit.Amount, incoming.Limit.Action, incoming.Limit.BlockAt
	out.Exemptions, out.Warnings, out.Labels = incoming.Exemptions, incoming.Warnings, incoming.Labels
	return out
}

// importResult reports every row, in file order, with its errors or what it does.
func importResult(rows []*importRow, dryRun bool) *gwspec.BudgetImportResult {
	result := &gwspec.BudgetImportResult{DryRun: dryRun, Results: make([]gwspec.BudgetImportRowResult, 0, len(rows))}
	for _, row := range rows {
		r := gwspec.BudgetImportRowResult{Row: row.row}
		if row.name != "" {
			name := row.name
			r.Name = &name
		}
		if len(row.errs) > 0 {
			r.Errors = row.errs
			result.Summary.Error++
		} else {
			result.Summary.Valid++
			action := row.action
			r.Action = &action
			switch action {
			case importCreate:
				result.Summary.Created++
			case importUpdate:
				result.Summary.Updated++
			case importSkip:
				result.Summary.Skipped++
			}
		}
		result.Results = append(result.Results, r)
	}
	return result
}

// checkCSVColumns checks each column that can be judged on its own, and fails every one
// that is wrong, so a row lists all of its bad cells at once. Checks across columns and
// against existing projects, agents, providers and channels run afterwards, when every
// column is readable (prepareBudget), and report one problem at a time.
func checkCSVColumns(cells map[string]string, row *importRow) {
	oneOf := func(column, value string, allowed ...string) {
		if value != "" && !slices.Contains(allowed, value) {
			row.fail(column, httpx.ErrCodeValidation, fmt.Sprintf("%s must be %s; got %q.", column, orList(allowed), value))
		}
	}
	required := func(column string) bool {
		if cells[column] == "" {
			row.fail(column, httpx.ErrCodeValidation, column+" is required.")
			return false
		}
		return true
	}
	if required(csvName) && !budgetNamePattern.MatchString(cells[csvName]) {
		row.fail(csvName, httpx.ErrCodeValidation, "name must be 1-63 lowercase letters, digits or '-', starting and ending with a letter or digit.")
	}
	if required(csvAmount) {
		// The pattern goes first, so exponents are refused before they are parsed.
		valid := budgetAmountPattern.MatchString(cells[csvAmount])
		if valid {
			amount, ok := new(big.Rat).SetString(cells[csvAmount])
			valid = ok && amount.Sign() > 0
		}
		if !valid {
			row.fail(csvAmount, httpx.ErrCodeValidation, "amount must be a decimal above zero, with at most 12 whole digits, 2 decimals and no leading zeros, e.g. 100.00.")
		}
	}
	if required(csvPeriod) {
		oneOf(csvPeriod, cells[csvPeriod], "day", "week", "month")
	}
	if required(csvAction) {
		oneOf(csvAction, cells[csvAction], "block", "allow")
	}
	oneOf(csvSplitBy, cells[csvSplitBy], budgetFilterTypes...)
	if v := cells[csvBlockAt]; v != "" {
		if n, err := strconv.Atoi(v); err == nil && (n < 1 || n > 100) {
			row.fail(csvBlockAt, httpx.ErrCodeValidation, "blockAt must be a whole percentage, 1-100.")
		}
	}
	for _, v := range csvList(cells[csvThresholds]) {
		if n, err := strconv.Atoi(v); err == nil && (n < 1 || n > maxBudgetThreshold) {
			row.fail(csvThresholds, httpx.ErrCodeValidation, fmt.Sprintf("thresholds must be whole percentages from 1 to %d; got %d.", maxBudgetThreshold, n))
			break
		}
	}
}

// orList is "a, b or c".
func orList(values []string) string {
	if len(values) < 2 {
		return strings.Join(values, "")
	}
	return strings.Join(values[:len(values)-1], ", ") + " or " + values[len(values)-1]
}
