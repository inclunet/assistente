package jobs

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// --- fixTemplateDots ---

func TestFixTemplateDots(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"already correct", "{{ .event.id }}", "{{ .event.id }}"},
		{"missing dot event", "{{ event.id }}", "{{ .event.id }}"},
		{"missing dot output", "{{ output.data }}", "{{ .output.data }}"},
		{"missing dot now", "{{ now }}", "{{ .now }}"},
		{"trim variant", "{{event.id}}", "{{.event.id}}"},
		{"with dash trim", "{{- event.id }}", "{{- .event.id }}"},
		{"already correct with dash", "{{- .event.id }}", "{{- .event.id }}"},
		{"no template", "hello world", "hello world"},
		{"mixed correct and wrong", "{{ .event.a }} {{ output.b }}", "{{ .event.a }} {{ .output.b }}"},
		{"pipe after event", "{{ event.items | json }}", "{{ .event.items | json }}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixTemplateDots(tt.input)
			if got != tt.expect {
				t.Errorf("fixTemplateDots(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

// --- fixArrayAccess ---

func TestFixArrayAccess(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			"simple array index",
			"{{ .event.items.0.name }}",
			"{{ (index .event.items 0).name }}",
		},
		{
			"no array index",
			"{{ .event.name }}",
			"{{ .event.name }}",
		},
		{
			"array index at end",
			"{{ .event.items.0 }}",
			"{{ (index .event.items 0) }}",
		},
		{
			"multi-digit index",
			"{{ .event.items.12.name }}",
			"{{ (index .event.items 12).name }}",
		},
		{
			"no template",
			"hello world 1.2.3",
			"hello world 1.2.3",
		},
		{
			"already using index function",
			"{{ (index .event.items 0).name }}",
			"{{ (index .event.items 0).name }}",
		},
		{
			"deep nesting with field after",
			"{{ .output.results.0.data }}",
			"{{ (index .output.results 0).data }}",
		},
		{
			"two numeric indices in same path (matrix)",
			"{{ .event.matrix.0.1 }}",
			"{{ (index (index .event.matrix 0) 1) }}",
		},
		{
			"array then field then array",
			"{{ .event.rows.0.cells.2.value }}",
			"{{ (index (index .event.rows 0).cells 2).value }}",
		},
		{
			"three numeric indices",
			"{{ .event.cube.0.1.2 }}",
			"{{ (index (index (index .event.cube 0) 1) 2) }}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixArrayAccess(tt.input)
			if got != tt.expect {
				t.Errorf("fixArrayAccess(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

// --- Combined preprocessing (dots + array) ---

func TestPreprocessingCombined(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			"missing dot AND array index",
			"{{ event.items.0.id }}",
			"{{ (index .event.items 0).id }}",
		},
		{
			"correct dot with array index",
			"{{ .event.items.0.id }}",
			"{{ (index .event.items 0).id }}",
		},
		{
			"no preprocessing needed",
			"{{ .event.name }}",
			"{{ .event.name }}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixTemplateDots(tt.input)
			got = fixArrayAccess(got)
			if got != tt.expect {
				t.Errorf("preprocess(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

// --- resolveTemplate end-to-end ---

func TestResolveTemplate_SimpleField(t *testing.T) {
	ctx := &TemplateContext{
		Event: map[string]any{
			"name": "order-created",
			"id":   "evt-001",
		},
	}

	result, err := resolveTemplate("{{ .event.name }}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "order-created" {
		t.Errorf("got %q, want %q", result, "order-created")
	}
}

func TestResolveTemplate_NestedField(t *testing.T) {
	ctx := &TemplateContext{
		Event: map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"email": "alice@example.com",
				},
			},
		},
	}

	result, err := resolveTemplate("{{ .event.data.user.email }}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "alice@example.com" {
		t.Errorf("got %q, want %q", result, "alice@example.com")
	}
}

func TestResolveTemplate_ArrayIndex(t *testing.T) {
	ctx := &TemplateContext{
		Event: map[string]any{
			"servers": []any{
				map[string]any{"id": "srv-aaa", "region": "us-east"},
				map[string]any{"id": "srv-bbb", "region": "eu-west"},
			},
		},
	}

	tests := []struct {
		name     string
		template string
		expect   string
	}{
		{
			"dot notation .servers.0.id",
			"{{ .event.servers.0.id }}",
			"srv-aaa",
		},
		{
			"dot notation .servers.0.region",
			"{{ .event.servers.0.region }}",
			"us-east",
		},
		{
			"dot notation .servers.1.id",
			"{{ .event.servers.1.id }}",
			"srv-bbb",
		},
		{
			"explicit index function",
			"{{ (index .event.servers 0).id }}",
			"srv-aaa",
		},
		{
			"missing dot with array index",
			"{{ event.servers.0.id }}",
			"srv-aaa",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveTemplate(tt.template, ctx)
			if err != nil {
				t.Fatalf("resolveTemplate(%q) error: %v", tt.template, err)
			}
			if result != tt.expect {
				t.Errorf("resolveTemplate(%q) = %q, want %q", tt.template, result, tt.expect)
			}
		})
	}
}

func TestResolveTemplate_OutputField(t *testing.T) {
	ctx := &TemplateContext{
		Output: map[string]any{
			"status": "ok",
			"items":  []any{"x", "y", "z"},
		},
	}

	result, err := resolveTemplate("{{ .output.status }}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("got %q, want %q", result, "ok")
	}
}

func TestResolveTemplate_NowField(t *testing.T) {
	now := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	ctx := &TemplateContext{Now: now}

	result, err := resolveTemplate("{{ date .now \"2006-01-02\" }}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "2025-06-15" {
		t.Errorf("got %q, want %q", result, "2025-06-15")
	}
}

func TestResolveTemplate_MissingDotAutofix(t *testing.T) {
	ctx := &TemplateContext{
		Event: map[string]any{"color": "blue"},
	}

	result, err := resolveTemplate("{{ event.color }}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "blue" {
		t.Errorf("got %q, want %q", result, "blue")
	}
}

func TestResolveTemplate_NoTemplate(t *testing.T) {
	ctx := &TemplateContext{}

	result, err := resolveTemplate("plain text value", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "plain text value" {
		t.Errorf("got %q, want %q", result, "plain text value")
	}
}

func TestResolveTemplate_PipeFunction(t *testing.T) {
	ctx := &TemplateContext{
		Event: map[string]any{
			"tags": []any{"go", "rust", "python"},
		},
	}

	result, err := resolveTemplate(`{{ join .event.tags ", " }}`, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "go, rust, python" {
		t.Errorf("got %q, want %q", result, "go, rust, python")
	}
}

// --- ResolveInputs ---

func TestResolveInputs_MixedStaticAndTemplate(t *testing.T) {
	ctx := &TemplateContext{
		Event: map[string]any{
			"resources": []any{
				map[string]any{"id": "res-42"},
			},
		},
	}

	inputs := map[string]any{
		"resourceId": "{{ .event.resources.0.id }}",
		"query":      "status = active",
		"limit":      "50",
	}

	resolved, err := ResolveInputs(inputs, ctx)
	if err != nil {
		t.Fatalf("ResolveInputs error: %v", err)
	}

	if resolved["resourceId"] != "res-42" {
		t.Errorf("resourceId = %q, want %q", resolved["resourceId"], "res-42")
	}
	if resolved["query"] != "status = active" {
		t.Errorf("query = %q, want %q", resolved["query"], "status = active")
	}
	if resolved["limit"] != "50" {
		t.Errorf("limit = %q, want %q", resolved["limit"], "50")
	}
}

func TestResolveInputs_MissingDotWithArray(t *testing.T) {
	ctx := &TemplateContext{
		Event: map[string]any{
			"nodes": []any{
				map[string]any{"ip": "10.0.0.1"},
			},
		},
	}

	inputs := map[string]any{
		"host": "{{ event.nodes.0.ip }}",
	}

	resolved, err := ResolveInputs(inputs, ctx)
	if err != nil {
		t.Fatalf("ResolveInputs error: %v", err)
	}

	if resolved["host"] != "10.0.0.1" {
		t.Errorf("host = %q, want %q", resolved["host"], "10.0.0.1")
	}
}

func TestResolveInputs_Empty(t *testing.T) {
	ctx := &TemplateContext{}
	resolved, err := ResolveInputs(nil, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved) != 0 {
		t.Errorf("expected empty map, got %v", resolved)
	}
}

func TestResolveInputs_NestedMap(t *testing.T) {
	ctx := &TemplateContext{
		Event: map[string]any{"token": "abc-xyz"},
	}

	inputs := map[string]any{
		"headers": map[string]any{
			"Authorization": "Bearer {{ .event.token }}",
		},
	}

	resolved, err := ResolveInputs(inputs, ctx)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	headers := resolved["headers"].(map[string]any)
	if headers["Authorization"] != "Bearer abc-xyz" {
		t.Errorf("Authorization = %q, want %q", headers["Authorization"], "Bearer abc-xyz")
	}
}

func TestResolveInputs_ArrayValues(t *testing.T) {
	ctx := &TemplateContext{
		Event: map[string]any{"prefix": "prod"},
	}

	inputs := map[string]any{
		"names": []any{"{{ .event.prefix }}-db", "{{ .event.prefix }}-cache"},
	}

	resolved, err := ResolveInputs(inputs, ctx)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	arr := resolved["names"].([]any)
	if arr[0] != "prod-db" {
		t.Errorf("names[0] = %q, want %q", arr[0], "prod-db")
	}
	if arr[1] != "prod-cache" {
		t.Errorf("names[1] = %q, want %q", arr[1], "prod-cache")
	}
}

// --- Matrix / nested arrays ---

func TestResolveTemplate_Matrix(t *testing.T) {
	ctx := &TemplateContext{
		Event: map[string]any{
			"matrix": []any{
				[]any{"a1", "a2", "a3"},
				[]any{"b1", "b2", "b3"},
			},
		},
	}

	tests := []struct {
		name     string
		template string
		expect   string
	}{
		{"row 0 col 1", "{{ .event.matrix.0.1 }}", "a2"},
		{"row 1 col 0", "{{ .event.matrix.1.0 }}", "b1"},
		{"row 1 col 2", "{{ .event.matrix.1.2 }}", "b3"},
		{"explicit index", "{{ (index (index .event.matrix 0) 2) }}", "a3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveTemplate(tt.template, ctx)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if result != tt.expect {
				t.Errorf("got %q, want %q", result, tt.expect)
			}
		})
	}
}

func TestResolveTemplate_ArrayFieldArray(t *testing.T) {
	ctx := &TemplateContext{
		Event: map[string]any{
			"tables": []any{
				map[string]any{
					"name": "users",
					"rows": []any{
						map[string]any{"id": "u-1", "email": "a@test.com"},
						map[string]any{"id": "u-2", "email": "b@test.com"},
					},
				},
				map[string]any{
					"name": "orders",
					"rows": []any{
						map[string]any{"id": "o-1", "total": 99.5},
					},
				},
			},
		},
	}

	tests := []struct {
		name     string
		template string
		expect   string
	}{
		{"first table name", "{{ .event.tables.0.name }}", "users"},
		{"first table second row email", "{{ .event.tables.0.rows.1.email }}", "b@test.com"},
		{"second table first row id", "{{ .event.tables.1.rows.0.id }}", "o-1"},
		{"missing dot with nested arrays", "{{ event.tables.0.rows.0.id }}", "u-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveTemplate(tt.template, ctx)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			str := fmt.Sprintf("%v", result)
			if str != tt.expect {
				t.Errorf("got %q, want %q", str, tt.expect)
			}
		})
	}
}

func TestResolveInputs_MatrixAccess(t *testing.T) {
	ctx := &TemplateContext{
		Event: map[string]any{
			"grid": []any{
				[]any{10, 20, 30},
				[]any{40, 50, 60},
			},
		},
	}

	inputs := map[string]any{
		"topRight":    "{{ .event.grid.0.2 }}",
		"bottomLeft":  "{{ .event.grid.1.0 }}",
		"center":      "{{ .event.grid.1.1 }}",
	}

	resolved, err := ResolveInputs(inputs, ctx)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if fmt.Sprintf("%v", resolved["topRight"]) != "30" {
		t.Errorf("topRight = %v, want 30", resolved["topRight"])
	}
	if fmt.Sprintf("%v", resolved["bottomLeft"]) != "40" {
		t.Errorf("bottomLeft = %v, want 40", resolved["bottomLeft"])
	}
	if fmt.Sprintf("%v", resolved["center"]) != "50" {
		t.Errorf("center = %v, want 50", resolved["center"])
	}
}

// --- CoerceInputs ---

func TestCoerceInputs(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"count": {"type": "integer"},
			"rate": {"type": "number"},
			"enabled": {"type": "boolean"},
			"tags": {"type": "array"},
			"config": {"type": "object"},
			"name": {"type": "string"},
			"unknown": {"type": "string"}
		}
	}`)

	inputs := map[string]any{
		"count":   "42",
		"rate":    "3.14",
		"enabled": "true",
		"tags":    `["a","b"]`,
		"config":  `{"key":"val"}`,
		"name":    "hello",
		"extra":   "not-in-schema",
	}

	result := CoerceInputs(inputs, schema)

	if v, ok := result["count"].(int64); !ok || v != 42 {
		t.Errorf("count: got %v (%T), want int64(42)", result["count"], result["count"])
	}
	if v, ok := result["rate"].(float64); !ok || v != 3.14 {
		t.Errorf("rate: got %v (%T), want float64(3.14)", result["rate"], result["rate"])
	}
	if v, ok := result["enabled"].(bool); !ok || v != true {
		t.Errorf("enabled: got %v (%T), want true", result["enabled"], result["enabled"])
	}
	if v, ok := result["tags"].([]any); !ok || len(v) != 2 {
		t.Errorf("tags: got %v (%T), want [a b]", result["tags"], result["tags"])
	}
	if v, ok := result["config"].(map[string]any); !ok || v["key"] != "val" {
		t.Errorf("config: got %v (%T), want {key:val}", result["config"], result["config"])
	}
	if result["name"] != "hello" {
		t.Errorf("name: got %q, want %q", result["name"], "hello")
	}
	if result["extra"] != "not-in-schema" {
		t.Errorf("extra: got %q, want %q", result["extra"], "not-in-schema")
	}
}

func TestCoerceInputs_InvalidValuesKeptAsString(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"count": {"type": "integer"},
			"rate": {"type": "number"},
			"enabled": {"type": "boolean"}
		}
	}`)

	inputs := map[string]any{
		"count":   "not-a-number",
		"rate":    "also-not",
		"enabled": "maybe",
	}

	result := CoerceInputs(inputs, schema)

	if result["count"] != "not-a-number" {
		t.Errorf("count should remain string, got %v (%T)", result["count"], result["count"])
	}
	if result["rate"] != "also-not" {
		t.Errorf("rate should remain string, got %v (%T)", result["rate"], result["rate"])
	}
	if result["enabled"] != "maybe" {
		t.Errorf("enabled should remain string, got %v (%T)", result["enabled"], result["enabled"])
	}
}

func TestCoerceInputs_NilSchema(t *testing.T) {
	inputs := map[string]any{"a": "1"}
	result := CoerceInputs(inputs, nil)
	if result["a"] != "1" {
		t.Errorf("expected unchanged, got %v", result["a"])
	}
}

func TestCoerceInputs_NonStringValues(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"count":{"type":"integer"}}}`)
	inputs := map[string]any{"count": 42}
	result := CoerceInputs(inputs, schema)
	if result["count"] != 42 {
		t.Errorf("expected 42 (int), got %v (%T)", result["count"], result["count"])
	}
}

// --- Realistic end-to-end scenario: upstream array response ---

func TestResolveTemplate_UpstreamArrayResponse(t *testing.T) {
	// Simulates: upstream job returns a JSON array from an API.
	// The executor parses it as output["content"] = []any{...}
	// which then becomes the event payload for the downstream job.
	eventPayload := map[string]any{
		"content": []any{
			map[string]any{
				"id":   "tenant-001",
				"name": "acme-corp",
				"url":  "https://acme.example.com",
			},
			map[string]any{
				"id":   "tenant-002",
				"name": "globex",
				"url":  "https://globex.example.com",
			},
		},
		"_meta_source": "api-gateway",
	}

	ctx := &TemplateContext{Event: eventPayload}

	tests := []struct {
		name     string
		template string
		expect   string
	}{
		{"first element id", "{{ .event.content.0.id }}", "tenant-001"},
		{"first element name", "{{ .event.content.0.name }}", "acme-corp"},
		{"second element id", "{{ .event.content.1.id }}", "tenant-002"},
		{"explicit index", "{{ (index .event.content 0).url }}", "https://acme.example.com"},
		{"missing dot + array", "{{ event.content.0.id }}", "tenant-001"},
		{"metadata field", "{{ .event._meta_source }}", "api-gateway"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveTemplate(tt.template, ctx)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if result != tt.expect {
				t.Errorf("got %q, want %q", result, tt.expect)
			}
		})
	}
}

// --- Full pipeline: ResolveInputs + CoerceInputs ---

func TestFullPipeline_ResolveAndCoerce(t *testing.T) {
	eventPayload := map[string]any{
		"accounts": []any{
			map[string]any{"id": "acc-777"},
		},
	}

	ctx := &TemplateContext{Event: eventPayload}

	inputs := map[string]any{
		"accountId": "{{ .event.accounts.0.id }}",
		"query":     "type = invoice",
		"limit":     "100",
		"format":    "json",
	}

	resolved, err := ResolveInputs(inputs, ctx)
	if err != nil {
		t.Fatalf("ResolveInputs error: %v", err)
	}

	if resolved["accountId"] != "acc-777" {
		t.Errorf("accountId = %q, want %q", resolved["accountId"], "acc-777")
	}

	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"accountId": {"type": "string"},
			"query": {"type": "string"},
			"limit": {"type": "integer"},
			"format": {"type": "string"}
		}
	}`)

	coerced := CoerceInputs(resolved, schema)

	if coerced["accountId"] != "acc-777" {
		t.Errorf("accountId = %q, want %q", coerced["accountId"], "acc-777")
	}
	if v, ok := coerced["limit"].(int64); !ok || v != 100 {
		t.Errorf("limit = %v (%T), want int64(100)", coerced["limit"], coerced["limit"])
	}
}

// --- Template functions ---

func TestTplJoin(t *testing.T) {
	result, err := tplJoin([]any{"go", "rust", "python"}, ", ")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if result != "go, rust, python" {
		t.Errorf("got %q, want %q", result, "go, rust, python")
	}
}

func TestTplPluck(t *testing.T) {
	items := []any{
		map[string]any{"label": "alpha", "score": 10},
		map[string]any{"label": "beta", "score": 20},
	}
	result, err := tplPluck(items, "label")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(result) != 2 || result[0] != "alpha" || result[1] != "beta" {
		t.Errorf("got %v, want [alpha beta]", result)
	}
}

func TestTplDefault(t *testing.T) {
	if v := tplDefault(50, nil); v != 50 {
		t.Errorf("nil case: got %v, want 50", v)
	}
	if v := tplDefault(50, ""); v != 50 {
		t.Errorf("empty string case: got %v, want 50", v)
	}
	if v := tplDefault(50, "hello"); v != "hello" {
		t.Errorf("non-zero case: got %v, want hello", v)
	}
	if v := tplDefault(50, 0); v != 50 {
		t.Errorf("zero int case: got %v, want 50", v)
	}
}

func TestTplDate(t *testing.T) {
	tm := time.Date(2025, 12, 31, 23, 59, 0, 0, time.UTC)
	result, err := tplDate(tm, "2006-01-02")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if result != "2025-12-31" {
		t.Errorf("got %q, want %q", result, "2025-12-31")
	}
}

func TestTplDate_FromString(t *testing.T) {
	result, err := tplDate("2025-08-20T10:00:00Z", "02/01/2006")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if result != "20/08/2025" {
		t.Errorf("got %q, want %q", result, "20/08/2025")
	}
}

func TestTplJSON(t *testing.T) {
	result, err := tplJSON(map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if result != `{"x":1}` {
		t.Errorf("got %q, want %q", result, `{"x":1}`)
	}
}

func TestTplAny(t *testing.T) {
	items := []any{
		map[string]any{"status": "open", "priority": "high"},
		map[string]any{"status": "closed", "priority": "low"},
	}

	found, err := tplAny(items, "priority", "high")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !found {
		t.Error("expected true, got false")
	}

	found, err = tplAny(items, "priority", "critical")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if found {
		t.Error("expected false, got true")
	}
}

func TestNavigatePath(t *testing.T) {
	data := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": "deep-value",
			},
		},
	}

	val, ok := navigatePath(data, "a.b.c")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if val != "deep-value" {
		t.Errorf("got %q, want %q", val, "deep-value")
	}

	_, ok = navigatePath(data, "a.b.missing")
	if ok {
		t.Error("expected ok=false for missing path")
	}
}
