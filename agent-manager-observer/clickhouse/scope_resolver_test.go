package clickhouse

import (
	"context"
	"errors"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

func TestStaticResolver(t *testing.T) {
	r := StaticResolver{
		Projects:   map[string]string{"default/default": "55f1"},
		Components: map[string]string{"default/it-support": "f412"},
	}
	ctx := context.Background()
	str := func(s string) *string { return &s }

	got, err := r.Resolve(ctx, observer.ComponentSearchScope{
		Namespace: "default", Project: str("default"), Component: str("it-support")})
	if err != nil || got.Project != "55f1" || got.Component != "f412" || got.Environment != "" {
		t.Fatalf("good names: got %+v, err %v", got, err)
	}

	// The dangerous case: a typo must be an error, never "no filter".
	_, err = r.Resolve(ctx, observer.ComponentSearchScope{Namespace: "default", Project: str("billng")})
	if !errors.Is(err, ErrUnknownScope) {
		t.Fatalf("typo in project name must fail, got %v", err)
	}

	_, err = r.Resolve(ctx, observer.ComponentSearchScope{Project: str("default")})
	if !errors.Is(err, ErrUnknownScope) {
		t.Fatalf("missing namespace must fail, got %v", err)
	}
}
