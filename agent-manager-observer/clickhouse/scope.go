package clickhouse

import (
	"strings"

	"github.com/wso2/agent-manager/agent-manager-observer/scope"
)

// tenantColumns names the four columns that hold the owner of a span.
type tenantColumns struct{ ns, project, component, env string }

// tenantTable is for otel_traces_tenant, which has real columns for the owner.
var tenantTable = tenantColumns{"Namespace", "ProjectUid", "ComponentUid", "EnvironmentUid"}

// exporterTable is for otel_traces, where the owner is inside the ResourceAttributes map.
var exporterTable = tenantColumns{
	"ResourceAttributes['openchoreo.dev/namespace']",
	"ResourceAttributes['openchoreo.dev/project-uid']",
	"ResourceAttributes['openchoreo.dev/component-uid']",
	"ResourceAttributes['openchoreo.dev/environment-uid']",
}

// scopeWhere builds "Namespace = ? AND ..." plus the values for the ? marks.
// Column names come from the constants above and never from the request.
func scopeWhere(c tenantColumns, s scope.UIDs) (string, []any) {
	var conds []string
	var args []any
	add := func(col, val string) {
		if val != "" {
			conds = append(conds, col+" = ?")
			args = append(args, val)
		}
	}
	add(c.ns, s.Namespace)
	add(c.project, s.Project)
	add(c.component, s.Component)
	add(c.env, s.Environment)
	return strings.Join(conds, " AND "), args
}
