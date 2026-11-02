package clickhouse

import (
	"testing"
)

func TestAttributesToAny(t *testing.T) {
	got := attributesToAny(map[string]string{
		"http.status_code":           "500",
		"gen_ai.system":              "openai",
		"gen_ai.embedding.dimension": "oops",
		"gen_ai.usage.input_tokens":  "420",
		"llm.is_streaming":           "true",
	})
	if got["http.status_code"] != float64(500) {
		t.Errorf("status code should be a number, got %v", got["http.status_code"])
	}
	if got["gen_ai.system"] != "openai" {
		t.Error("text values must stay text")
	}
	if got["gen_ai.embedding.dimension"] != "oops" {
		t.Error("a value that is not a number must stay text")
	}
	if got["gen_ai.usage.input_tokens"] != float64(420) {
		t.Errorf("input tokens should be a number, got %v", got["gen_ai.usage.input_tokens"])
	}
	if got["llm.is_streaming"] != true {
		t.Errorf("is_streaming should be a boolean, got %v", got["llm.is_streaming"])
	}
	if attributesToAny(nil) == nil {
		t.Error("a nil map should give an empty map, not nil")
	}
}
