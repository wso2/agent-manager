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

package repositories

import (
	"context"
	"strings"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// A2ACatalogRepository reads the organisation's A2A agents, one entry per project and agent name.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/a2a_catalog_repository_mock.go . A2ACatalogRepository:A2ACatalogRepositoryMock
type A2ACatalogRepository interface {
	// List returns every environment row of the agents on the page, ordered by project, agent, environment name, and the number of matching agents.
	List(ctx context.Context, filter models.A2ACatalogFilter) ([]models.A2ACatalogRow, int, error)
	// ListForAgent returns the agent's rows in environment-name order, Matched false; no rows is not an error.
	ListForAgent(ctx context.Context, ouID, projectName, agentName string) ([]models.A2ACatalogRow, error)
}

type a2aCatalogRepository struct {
	db *gorm.DB
}

// NewA2ACatalogRepository creates an A2ACatalogRepository.
func NewA2ACatalogRepository(db *gorm.DB) A2ACatalogRepository {
	return &a2aCatalogRepository{db: db}
}

const a2aCatalogProxied = `EXISTS (SELECT 1 FROM a2a_proxies p
	WHERE p.ou_id = c.ou_id AND p.project_name = c.project_name
	AND p.agent_name = c.agent_name AND p.environment_uuid = c.environment_uuid)`

const a2aCatalogSearchMatch = `(c.agent_name ILIKE ? ESCAPE '\' OR c.card_search_text ILIKE ? ESCAPE '\')`

// sqlCondition is a SQL boolean expression with its bind arguments.
type sqlCondition struct {
	sql  string
	args []interface{}
}

func searchCondition(query string) sqlCondition {
	pattern := "%" + escapeLikePattern(query) + "%"
	return sqlCondition{sql: a2aCatalogSearchMatch, args: []interface{}{pattern, pattern}}
}

// rowCondition is what one card row must satisfy for its agent to be in the catalogue.
func rowCondition(filter models.A2ACatalogFilter) sqlCondition {
	conds := []sqlCondition{{sql: "c.ou_id = ?", args: []interface{}{filter.OUID}}}
	addEquals := func(column, value string) {
		if value != "" {
			conds = append(conds, sqlCondition{sql: column + " = ?", args: []interface{}{value}})
		}
	}
	addEquals("c.project_name", filter.Project)
	addEquals("c.environment_name", filter.Environment)
	addEquals("c.status", string(filter.Status))
	switch filter.Kind {
	case models.A2ACatalogKindPlatform, models.A2ACatalogKindExternal:
		addEquals("c.source", string(filter.Kind))
	case models.A2ACatalogKindProxied:
		conds = append(conds, sqlCondition{sql: a2aCatalogProxied})
	}
	if filter.Query != "" {
		conds = append(conds, searchCondition(filter.Query))
	}

	sqls := make([]string, 0, len(conds))
	var args []interface{}
	for _, cond := range conds {
		sqls = append(sqls, cond.sql)
		args = append(args, cond.args...)
	}
	return sqlCondition{sql: strings.Join(sqls, " AND "), args: args}
}

// matchedColumn is the per-row search flag; without a query no row is matched.
func matchedColumn(query string) sqlCondition {
	if query == "" {
		return sqlCondition{sql: "false"}
	}
	search := searchCondition(query)
	return sqlCondition{sql: "COALESCE(" + search.sql + ", false)", args: search.args}
}

func (r *a2aCatalogRepository) List(ctx context.Context, filter models.A2ACatalogFilter) ([]models.A2ACatalogRow, int, error) {
	where := rowCondition(filter)

	var total int
	countSQL := `SELECT COUNT(*) FROM (
		SELECT 1 FROM a2a_agent_cards c WHERE ` + where.sql + `
		GROUP BY c.project_name, c.agent_name
	) m`
	if err := r.db.WithContext(ctx).Raw(countSQL, where.args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	args := append([]interface{}{}, where.args...)
	page := "OFFSET ?"
	if filter.Limit > 0 {
		page = "LIMIT ? OFFSET ?"
		args = append(args, filter.Limit)
	}
	args = append(args, filter.Offset)
	matched := matchedColumn(filter.Query)
	args = append(args, matched.args...)
	args = append(args, filter.OUID)

	rowsSQL := `WITH keys AS (
		SELECT c.project_name, c.agent_name FROM a2a_agent_cards c WHERE ` + where.sql + `
		GROUP BY c.project_name, c.agent_name
		ORDER BY c.project_name, c.agent_name
		` + page + `
	)
	SELECT c.*, ` + a2aCatalogProxied + ` AS proxied, ` + matched.sql + ` AS matched
	FROM a2a_agent_cards c
	JOIN keys k ON k.project_name = c.project_name AND k.agent_name = c.agent_name
	WHERE c.ou_id = ?
	ORDER BY c.project_name, c.agent_name, c.environment_name`
	rows := []models.A2ACatalogRow{}
	if err := r.db.WithContext(ctx).Raw(rowsSQL, args...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *a2aCatalogRepository) ListForAgent(ctx context.Context, ouID, projectName, agentName string) ([]models.A2ACatalogRow, error) {
	rowsSQL := `SELECT c.*, ` + a2aCatalogProxied + ` AS proxied, false AS matched
	FROM a2a_agent_cards c
	WHERE c.ou_id = ? AND c.project_name = ? AND c.agent_name = ?
	ORDER BY c.environment_name`
	rows := []models.A2ACatalogRow{}
	if err := r.db.WithContext(ctx).Raw(rowsSQL, ouID, projectName, agentName).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
