package opensearchreader

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/scope"
)

func TestListQuery(t *testing.T) {
	w := window{Start: time.Unix(1000, 0), End: time.Unix(2000, 0)}
	b, err := json.Marshal(listQuery(scope.UIDs{Namespace: "default", Component: "abc"}, w, true, 10))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		`"resource.openchoreo.dev/component-uid":"abc"`,
		`"resource.openchoreo.dev/namespace":"default"`,
		`"min_start_time":"desc"`,
		`"parentSpanId":""`,
		`"size":10`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("query is missing %s", want)
		}
	}
	if strings.Contains(s, "resource.openchoreo.dev/project-uid") {
		t.Error("a project filter appeared although no project was asked for")
	}
}

func TestDetailQueryHasNamespaceGuard(t *testing.T) {
	b, _ := json.Marshal(detailQuery("default", "t1", "s1"))
	if !strings.Contains(string(b), `"resource.openchoreo.dev/namespace":"default"`) {
		t.Error("namespace guard missing")
	}
}
