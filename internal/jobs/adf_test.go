package jobs

import (
	"strings"
	"testing"
)

func TestADFMarkdown_TwoParagraphs(t *testing.T) {
	doc := map[string]any{
		"version": 1,
		"type":    "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{"type": "text", "text": "First paragraph."},
				},
			},
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{"type": "text", "text": "Second paragraph."},
				},
			},
		},
	}

	result, err := tplADFMarkdown(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "First paragraph.") || !strings.Contains(result, "Second paragraph.") {
		t.Errorf("expected both paragraphs, got:\n%s", result)
	}

	parts := strings.Split(result, "\n\n")
	if len(parts) < 2 {
		t.Errorf("expected paragraphs separated by blank line, got:\n%s", result)
	}
}

func TestADFMarkdown_TextWithLink(t *testing.T) {
	doc := map[string]any{
		"version": 1,
		"type":    "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{"type": "text", "text": "Visit "},
					map[string]any{
						"type": "text",
						"text": "Atlassian",
						"marks": []any{
							map[string]any{
								"type":  "link",
								"attrs": map[string]any{"href": "https://atlassian.com"},
							},
						},
					},
					map[string]any{"type": "text", "text": " for more."},
				},
			},
		},
	}

	result, err := tplADFMarkdown(doc)
	if err != nil {
		t.Fatal(err)
	}

	expect := "Visit [Atlassian](https://atlassian.com) for more."
	if result != expect {
		t.Errorf("got:\n%s\nwant:\n%s", result, expect)
	}
}

func TestADFMarkdown_BoldItalicCode(t *testing.T) {
	doc := map[string]any{
		"version": 1,
		"type":    "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{
						"type":  "text",
						"text":  "bold",
						"marks": []any{map[string]any{"type": "strong"}},
					},
					map[string]any{"type": "text", "text": " and "},
					map[string]any{
						"type":  "text",
						"text":  "italic",
						"marks": []any{map[string]any{"type": "em"}},
					},
					map[string]any{"type": "text", "text": " and "},
					map[string]any{
						"type":  "text",
						"text":  "code",
						"marks": []any{map[string]any{"type": "code"}},
					},
					map[string]any{"type": "text", "text": " and "},
					map[string]any{
						"type":  "text",
						"text":  "strike",
						"marks": []any{map[string]any{"type": "strike"}},
					},
				},
			},
		},
	}

	result, err := tplADFMarkdown(doc)
	if err != nil {
		t.Fatal(err)
	}

	expect := "**bold** and *italic* and `code` and ~~strike~~"
	if result != expect {
		t.Errorf("got:\n%s\nwant:\n%s", result, expect)
	}
}

func TestADFMarkdown_BulletList(t *testing.T) {
	doc := map[string]any{
		"version": 1,
		"type":    "doc",
		"content": []any{
			map[string]any{
				"type": "bulletList",
				"content": []any{
					map[string]any{
						"type": "listItem",
						"content": []any{
							map[string]any{
								"type":    "paragraph",
								"content": []any{map[string]any{"type": "text", "text": "Item A"}},
							},
						},
					},
					map[string]any{
						"type": "listItem",
						"content": []any{
							map[string]any{
								"type":    "paragraph",
								"content": []any{map[string]any{"type": "text", "text": "Item B"}},
							},
						},
					},
					map[string]any{
						"type": "listItem",
						"content": []any{
							map[string]any{
								"type":    "paragraph",
								"content": []any{map[string]any{"type": "text", "text": "Item C"}},
							},
						},
					},
				},
			},
		},
	}

	result, err := tplADFMarkdown(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "- Item A") {
		t.Errorf("expected '- Item A', got:\n%s", result)
	}
	if !strings.Contains(result, "- Item B") {
		t.Errorf("expected '- Item B', got:\n%s", result)
	}
	if !strings.Contains(result, "- Item C") {
		t.Errorf("expected '- Item C', got:\n%s", result)
	}
}

func TestADFMarkdown_OrderedList(t *testing.T) {
	doc := map[string]any{
		"version": 1,
		"type":    "doc",
		"content": []any{
			map[string]any{
				"type": "orderedList",
				"content": []any{
					map[string]any{
						"type": "listItem",
						"content": []any{
							map[string]any{
								"type":    "paragraph",
								"content": []any{map[string]any{"type": "text", "text": "First"}},
							},
						},
					},
					map[string]any{
						"type": "listItem",
						"content": []any{
							map[string]any{
								"type":    "paragraph",
								"content": []any{map[string]any{"type": "text", "text": "Second"}},
							},
						},
					},
				},
			},
		},
	}

	result, err := tplADFMarkdown(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "1. First") {
		t.Errorf("expected '1. First', got:\n%s", result)
	}
	if !strings.Contains(result, "2. Second") {
		t.Errorf("expected '2. Second', got:\n%s", result)
	}
}

func TestADFMarkdown_CodeBlock(t *testing.T) {
	doc := map[string]any{
		"version": 1,
		"type":    "doc",
		"content": []any{
			map[string]any{
				"type":  "codeBlock",
				"attrs": map[string]any{"language": "go"},
				"content": []any{
					map[string]any{"type": "text", "text": "fmt.Println(\"hello\")"},
				},
			},
		},
	}

	result, err := tplADFMarkdown(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "```go") {
		t.Errorf("expected code fence with 'go', got:\n%s", result)
	}
	if !strings.Contains(result, "fmt.Println") {
		t.Errorf("expected code content, got:\n%s", result)
	}
	if !strings.Contains(result, "```") {
		t.Errorf("expected closing fence, got:\n%s", result)
	}
}

func TestADFMarkdown_NilInput(t *testing.T) {
	result, err := tplADFMarkdown(nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != "" {
		t.Errorf("expected empty string for nil, got: %q", result)
	}
}

func TestADFMarkdown_EmptyStringInput(t *testing.T) {
	result, err := tplADFMarkdown("")
	if err != nil {
		t.Fatal(err)
	}
	if result != "" {
		t.Errorf("expected empty string for empty input, got: %q", result)
	}
}

func TestADFMarkdown_UnknownNodeFallback(t *testing.T) {
	doc := map[string]any{
		"version": 1,
		"type":    "doc",
		"content": []any{
			map[string]any{
				"type": "futureBlockType",
				"content": []any{
					map[string]any{
						"type":    "paragraph",
						"content": []any{map[string]any{"type": "text", "text": "Fallback text."}},
					},
				},
			},
		},
	}

	result, err := tplADFMarkdown(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "Fallback text.") {
		t.Errorf("expected fallback text extraction, got:\n%s", result)
	}
}

func TestADFMarkdown_HardBreak(t *testing.T) {
	doc := map[string]any{
		"version": 1,
		"type":    "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{"type": "text", "text": "Line one"},
					map[string]any{"type": "hardBreak"},
					map[string]any{"type": "text", "text": "Line two"},
				},
			},
		},
	}

	result, err := tplADFMarkdown(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "Line one\nLine two") {
		t.Errorf("expected hard break as newline, got:\n%s", result)
	}
}

func TestADFMarkdown_InlineCard(t *testing.T) {
	doc := map[string]any{
		"version": 1,
		"type":    "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{"type": "text", "text": "See "},
					map[string]any{
						"type":  "inlineCard",
						"attrs": map[string]any{"url": "https://jira.example.com/browse/FSD-123"},
					},
				},
			},
		},
	}

	result, err := tplADFMarkdown(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "https://jira.example.com/browse/FSD-123") {
		t.Errorf("expected inline card URL, got:\n%s", result)
	}
}

func TestADFMarkdown_Heading(t *testing.T) {
	doc := map[string]any{
		"version": 1,
		"type":    "doc",
		"content": []any{
			map[string]any{
				"type":  "heading",
				"attrs": map[string]any{"level": float64(2)},
				"content": []any{
					map[string]any{"type": "text", "text": "Section Title"},
				},
			},
		},
	}

	result, err := tplADFMarkdown(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "## Section Title") {
		t.Errorf("expected '## Section Title', got:\n%s", result)
	}
}

func TestADFMarkdown_Blockquote(t *testing.T) {
	doc := map[string]any{
		"version": 1,
		"type":    "doc",
		"content": []any{
			map[string]any{
				"type": "blockquote",
				"content": []any{
					map[string]any{
						"type":    "paragraph",
						"content": []any{map[string]any{"type": "text", "text": "Quoted text."}},
					},
				},
			},
		},
	}

	result, err := tplADFMarkdown(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "> Quoted text.") {
		t.Errorf("expected blockquote, got:\n%s", result)
	}
}

func TestADFMarkdown_Rule(t *testing.T) {
	doc := map[string]any{
		"version": 1,
		"type":    "doc",
		"content": []any{
			map[string]any{
				"type":    "paragraph",
				"content": []any{map[string]any{"type": "text", "text": "Above"}},
			},
			map[string]any{"type": "rule"},
			map[string]any{
				"type":    "paragraph",
				"content": []any{map[string]any{"type": "text", "text": "Below"}},
			},
		},
	}

	result, err := tplADFMarkdown(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "---") {
		t.Errorf("expected horizontal rule, got:\n%s", result)
	}
}

func TestADFMarkdown_Mention(t *testing.T) {
	doc := map[string]any{
		"version": 1,
		"type":    "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{"type": "text", "text": "Assigned to "},
					map[string]any{
						"type":  "mention",
						"attrs": map[string]any{"id": "abc123", "text": "@Alice"},
					},
				},
			},
		},
	}

	result, err := tplADFMarkdown(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "@Alice") {
		t.Errorf("expected mention text, got:\n%s", result)
	}
}

func TestADFMarkdown_Emoji(t *testing.T) {
	doc := map[string]any{
		"version": 1,
		"type":    "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{"type": "text", "text": "Great "},
					map[string]any{
						"type":  "emoji",
						"attrs": map[string]any{"shortName": ":thumbsup:", "text": "\U0001f44d"},
					},
				},
			},
		},
	}

	result, err := tplADFMarkdown(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "\U0001f44d") {
		t.Errorf("expected emoji text, got:\n%s", result)
	}
}

func TestADFMarkdown_Panel(t *testing.T) {
	doc := map[string]any{
		"version": 1,
		"type":    "doc",
		"content": []any{
			map[string]any{
				"type":  "panel",
				"attrs": map[string]any{"panelType": "info"},
				"content": []any{
					map[string]any{
						"type":    "paragraph",
						"content": []any{map[string]any{"type": "text", "text": "Panel content."}},
					},
				},
			},
		},
	}

	result, err := tplADFMarkdown(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "Panel content.") {
		t.Errorf("expected panel content, got:\n%s", result)
	}
	if !strings.Contains(result, "Info") {
		t.Errorf("expected panel type header, got:\n%s", result)
	}
}

func TestADFMarkdown_Table(t *testing.T) {
	doc := map[string]any{
		"version": 1,
		"type":    "doc",
		"content": []any{
			map[string]any{
				"type": "table",
				"content": []any{
					map[string]any{
						"type": "tableRow",
						"content": []any{
							map[string]any{
								"type": "tableHeader",
								"content": []any{
									map[string]any{
										"type":    "paragraph",
										"content": []any{map[string]any{"type": "text", "text": "Name"}},
									},
								},
							},
							map[string]any{
								"type": "tableHeader",
								"content": []any{
									map[string]any{
										"type":    "paragraph",
										"content": []any{map[string]any{"type": "text", "text": "Value"}},
									},
								},
							},
						},
					},
					map[string]any{
						"type": "tableRow",
						"content": []any{
							map[string]any{
								"type": "tableCell",
								"content": []any{
									map[string]any{
										"type":    "paragraph",
										"content": []any{map[string]any{"type": "text", "text": "Alpha"}},
									},
								},
							},
							map[string]any{
								"type": "tableCell",
								"content": []any{
									map[string]any{
										"type":    "paragraph",
										"content": []any{map[string]any{"type": "text", "text": "100"}},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	result, err := tplADFMarkdown(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "| Name | Value |") {
		t.Errorf("expected table header, got:\n%s", result)
	}
	if !strings.Contains(result, "| --- | --- |") {
		t.Errorf("expected separator, got:\n%s", result)
	}
	if !strings.Contains(result, "| Alpha | 100 |") {
		t.Errorf("expected table row, got:\n%s", result)
	}
}

func TestADFMarkdown_JSONStringInput(t *testing.T) {
	jsonStr := `{"version":1,"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"From JSON string."}]}]}`

	result, err := tplADFMarkdown(jsonStr)
	if err != nil {
		t.Fatal(err)
	}

	if result != "From JSON string." {
		t.Errorf("got:\n%s\nwant:\nFrom JSON string.", result)
	}
}

func TestADFMarkdown_Expand(t *testing.T) {
	doc := map[string]any{
		"version": 1,
		"type":    "doc",
		"content": []any{
			map[string]any{
				"type":  "expand",
				"attrs": map[string]any{"title": "Details"},
				"content": []any{
					map[string]any{
						"type":    "paragraph",
						"content": []any{map[string]any{"type": "text", "text": "Hidden content."}},
					},
				},
			},
		},
	}

	result, err := tplADFMarkdown(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "Details") {
		t.Errorf("expected expand title, got:\n%s", result)
	}
	if !strings.Contains(result, "Hidden content.") {
		t.Errorf("expected expand body, got:\n%s", result)
	}
}

func TestADFText_StripsFormatting(t *testing.T) {
	doc := map[string]any{
		"version": 1,
		"type":    "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{
						"type":  "text",
						"text":  "bold",
						"marks": []any{map[string]any{"type": "strong"}},
					},
					map[string]any{"type": "text", "text": " text"},
				},
			},
		},
	}

	result, err := tplADFText(doc)
	if err != nil {
		t.Fatal(err)
	}

	if result != "bold text" {
		t.Errorf("adf_text should strip formatting, got: %q", result)
	}
}

func TestADFMarkdown_ComplexDocument(t *testing.T) {
	doc := map[string]any{
		"version": 1,
		"type":    "doc",
		"content": []any{
			map[string]any{
				"type":  "heading",
				"attrs": map[string]any{"level": float64(1)},
				"content": []any{
					map[string]any{"type": "text", "text": "Release Notes"},
				},
			},
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{"type": "text", "text": "Version "},
					map[string]any{
						"type":  "text",
						"text":  "2.0",
						"marks": []any{map[string]any{"type": "strong"}},
					},
					map[string]any{"type": "text", "text": " is out!"},
				},
			},
			map[string]any{
				"type": "bulletList",
				"content": []any{
					map[string]any{
						"type": "listItem",
						"content": []any{
							map[string]any{
								"type":    "paragraph",
								"content": []any{map[string]any{"type": "text", "text": "Bug fixes"}},
							},
						},
					},
					map[string]any{
						"type": "listItem",
						"content": []any{
							map[string]any{
								"type":    "paragraph",
								"content": []any{map[string]any{"type": "text", "text": "Performance improvements"}},
							},
						},
					},
				},
			},
			map[string]any{
				"type":  "codeBlock",
				"attrs": map[string]any{"language": "bash"},
				"content": []any{
					map[string]any{"type": "text", "text": "npm install v2.0"},
				},
			},
		},
	}

	result, err := tplADFMarkdown(doc)
	if err != nil {
		t.Fatal(err)
	}

	checks := []string{
		"# Release Notes",
		"Version **2.0** is out!",
		"- Bug fixes",
		"- Performance improvements",
		"```bash",
		"npm install v2.0",
	}
	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("expected %q in result, got:\n%s", check, result)
		}
	}
}

func TestADFMarkdown_ResolveTemplate_Integration(t *testing.T) {
	ctx := &TemplateContext{
		Event: map[string]any{
			"fields": map[string]any{
				"description": map[string]any{
					"version": 1,
					"type":    "doc",
					"content": []any{
						map[string]any{
							"type": "paragraph",
							"content": []any{
								map[string]any{"type": "text", "text": "Template integration test."},
							},
						},
					},
				},
			},
		},
	}

	result, err := resolveTemplate("{{ adf_markdown .event.fields.description }}", ctx)
	if err != nil {
		t.Fatal(err)
	}

	if result != "Template integration test." {
		t.Errorf("got %q, want %q", result, "Template integration test.")
	}
}

func TestADFMarkdown_Date(t *testing.T) {
	doc := map[string]any{
		"version": 1,
		"type":    "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{"type": "text", "text": "Due: "},
					map[string]any{
						"type":  "date",
						"attrs": map[string]any{"timestamp": "1735689600000"},
					},
				},
			},
		},
	}

	result, err := tplADFMarkdown(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "1735689600000") {
		t.Errorf("expected date timestamp, got:\n%s", result)
	}
}

func TestADFMarkdown_Status(t *testing.T) {
	doc := map[string]any{
		"version": 1,
		"type":    "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{
						"type":  "status",
						"attrs": map[string]any{"text": "IN PROGRESS", "color": "blue"},
					},
				},
			},
		},
	}

	result, err := tplADFMarkdown(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "[IN PROGRESS]") {
		t.Errorf("expected status badge, got:\n%s", result)
	}
}
