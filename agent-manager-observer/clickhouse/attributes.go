package clickhouse

import "strconv"

type attrType int

const (
	attrNumber attrType = iota + 1
	attrBool
)

// typedAttributes lists attributes whose original type was a number or a boolean.
// ClickHouse stores every value as text, so these are converted back.
var typedAttributes = map[string]attrType{
	"http.status_code":           attrNumber,
	"gen_ai.embedding.dimension": attrNumber,
	"gen_ai.request.temperature": attrNumber,
	"gen_ai.request.top_p":       attrNumber,
	"gen_ai.request.max_tokens":  attrNumber,
	"gen_ai.usage.input_tokens":  attrNumber,
	"gen_ai.usage.output_tokens": attrNumber,
	"gen_ai.usage.total_tokens":  attrNumber,
	"llm.usage.total_tokens":     attrNumber,
	"llm.is_streaming":           attrBool,
}

// attributesToAny turns ClickHouse's text-only map into the mixed-value map the
// rest of the code expects. Values that cannot be converted stay text.
func attributesToAny(in map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		switch typedAttributes[k] {
		case attrNumber:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				out[k] = f
				continue
			}
		case attrBool:
			if b, err := strconv.ParseBool(v); err == nil {
				out[k] = b
				continue
			}
		}
		out[k] = v
	}
	return out
}
