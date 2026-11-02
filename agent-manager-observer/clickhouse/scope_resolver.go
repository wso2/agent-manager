package clickhouse

import (
	"github.com/wso2/agent-manager/agent-manager-observer/scope"
)

// ErrUnknownScope means a name in the request has no matching id.
var ErrUnknownScope = scope.ErrUnknownScope

type ScopeUIDs = scope.UIDs
type ScopeResolver = scope.Resolver
type StaticResolver = scope.StaticResolver

func NewStaticResolverFromJSON(raw string) (StaticResolver, error) {
	return scope.NewStaticResolverFromJSON(raw)
}
