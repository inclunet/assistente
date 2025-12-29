package agents

import (
	"testing"
)

func TestTemplateEngine_Execute(t *testing.T) {
	engine := NewTemplateEngine()

	tests := []struct {
		name     string
		template string
		params   map[string]interface{}
		want     string
		wantErr  bool
	}{
		{
			name:     "simple variable",
			template: "/users/{{.user_id}}",
			params:   map[string]interface{}{"user_id": "123"},
			want:     "/users/123",
		},
		{
			name:     "urlEncode function",
			template: "q={{.query | urlEncode}}",
			params:   map[string]interface{}{"query": "São Paulo"},
			want:     "q=S%C3%A3o+Paulo",
		},
		{
			name:     "default function with value",
			template: "page={{.page | default 1}}",
			params:   map[string]interface{}{"page": 5},
			want:     "page=5",
		},
		{
			name:     "default function without value",
			template: "page={{.page | default 1}}",
			params:   map[string]interface{}{},
			want:     "page=1",
		},
		{
			name:     "upper function",
			template: "{{.name | upper}}",
			params:   map[string]interface{}{"name": "john"},
			want:     "JOHN",
		},
		{
			name:     "lower function",
			template: "{{.name | lower}}",
			params:   map[string]interface{}{"name": "JOHN"},
			want:     "john",
		},
		{
			name:     "multiple variables",
			template: "/repos/{{.owner}}/{{.repo}}/issues",
			params:   map[string]interface{}{"owner": "wailsapp", "repo": "wails"},
			want:     "/repos/wailsapp/wails/issues",
		},
		{
			name:     "jsonEncode array",
			template: `{"tags": {{.tags | jsonEncode}}}`,
			params:   map[string]interface{}{"tags": []string{"go", "svelte"}},
			want:     `{"tags": ["go","svelte"]}`,
		},
		{
			name:     "empty template",
			template: "",
			params:   map[string]interface{}{},
			want:     "",
		},
		{
			name:     "access env var - empty map shows no value",
			template: "token={{index .env \"TEST_TOKEN\"}}",
			params:   map[string]interface{}{},
			want:     "token=",
		},
		{
			name:     "access request_id",
			template: "id={{.request_id}}",
			params:   map[string]interface{}{},
			want:     "", // Vai ter um ID, mas não vazio
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewTemplateContext(tt.params, map[string]string{}, "test", "Test Agent")
			got, err := engine.Execute(tt.template, ctx)
			
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			// Para o teste de request_id, apenas verifica que não está vazio
			if tt.name == "access request_id" {
				if got == "" || got == "id=" {
					t.Errorf("Execute() request_id should not be empty")
				}
				return
			}
			
			if got != tt.want {
				t.Errorf("Execute() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTemplateEngine_ValidateTemplate(t *testing.T) {
	engine := NewTemplateEngine()

	tests := []struct {
		name     string
		template string
		wantErr  bool
	}{
		{
			name:     "valid template",
			template: "/users/{{.id}}",
			wantErr:  false,
		},
		{
			name:     "valid template with function",
			template: "{{.name | upper}}",
			wantErr:  false,
		},
		{
			name:     "invalid template - unclosed",
			template: "/users/{{.id}",
			wantErr:  true,
		},
		{
			name:     "empty template",
			template: "",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := engine.ValidateTemplate(tt.template)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTemplate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTemplateEngine_ExtractVariables(t *testing.T) {
	engine := NewTemplateEngine()

	tests := []struct {
		name     string
		template string
		want     []string
	}{
		{
			name:     "single variable",
			template: "/users/{{.user_id}}",
			want:     []string{"user_id"},
		},
		{
			name:     "multiple variables",
			template: "/repos/{{.owner}}/{{.repo}}",
			want:     []string{"owner", "repo"},
		},
		{
			name:     "variable with function",
			template: "q={{.query | urlEncode}}",
			want:     []string{"query"},
		},
		{
			name:     "ignore env vars",
			template: "token={{.env.TOKEN}}",
			want:     []string{},
		},
		{
			name:     "ignore special vars",
			template: "id={{.request_id}} time={{.timestamp}}",
			want:     []string{},
		},
		{
			name:     "mixed vars",
			template: "/users/{{.id}}?token={{.env.TOKEN}}&t={{.timestamp}}",
			want:     []string{"id"},
		},
		{
			name:     "duplicate variables",
			template: "{{.id}} and {{.id}} again",
			want:     []string{"id"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := engine.ExtractVariables(tt.template)
			
			if len(got) != len(tt.want) {
				t.Errorf("ExtractVariables() = %v, want %v", got, tt.want)
				return
			}
			
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("ExtractVariables()[%d] = %v, want %v", i, v, tt.want[i])
				}
			}
		})
	}
}

func TestTemplateFuncs(t *testing.T) {
	// Test urlEncode
	if got := urlEncode("hello world"); got != "hello+world" {
		t.Errorf("urlEncode() = %v, want %v", got, "hello+world")
	}

	// Test jsonEncode
	if got := jsonEncode(map[string]string{"a": "b"}); got != `{"a":"b"}` {
		t.Errorf("jsonEncode() = %v, want %v", got, `{"a":"b"}`)
	}

	// Test base64Encode
	if got := base64Encode("hello"); got != "aGVsbG8=" {
		t.Errorf("base64Encode() = %v, want %v", got, "aGVsbG8=")
	}

	// Test base64Decode
	if got := base64Decode("aGVsbG8="); got != "hello" {
		t.Errorf("base64Decode() = %v, want %v", got, "hello")
	}

	// Test defaultValue
	if got := defaultValue("default", nil); got != "default" {
		t.Errorf("defaultValue(nil) = %v, want %v", got, "default")
	}
	if got := defaultValue("default", "value"); got != "value" {
		t.Errorf("defaultValue(value) = %v, want %v", got, "value")
	}

	// Test coalesce
	if got := coalesce(nil, "", "first", "second"); got != "first" {
		t.Errorf("coalesce() = %v, want %v", got, "first")
	}

	// Test isEmpty
	if !isEmpty(nil) {
		t.Error("isEmpty(nil) should be true")
	}
	if !isEmpty("") {
		t.Error("isEmpty('') should be true")
	}
	if isEmpty("hello") {
		t.Error("isEmpty('hello') should be false")
	}

	// Test ternary
	if got := ternary(true, "yes", "no"); got != "yes" {
		t.Errorf("ternary(true) = %v, want %v", got, "yes")
	}
	if got := ternary(false, "yes", "no"); got != "no" {
		t.Errorf("ternary(false) = %v, want %v", got, "no")
	}

	// Test math functions
	if got := add(1, 2); got != 3 {
		t.Errorf("add(1,2) = %v, want %v", got, 3)
	}
	if got := sub(5, 3); got != 2 {
		t.Errorf("sub(5,3) = %v, want %v", got, 2)
	}
	if got := mul(2, 3); got != 6 {
		t.Errorf("mul(2,3) = %v, want %v", got, 6)
	}
	if got := div(10, 2); got != 5 {
		t.Errorf("div(10,2) = %v, want %v", got, 5)
	}
	if got := div(10, 0); got != 0 {
		t.Errorf("div(10,0) = %v, want %v", got, 0)
	}
}

