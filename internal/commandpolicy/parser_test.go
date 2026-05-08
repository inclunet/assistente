package commandpolicy

import "testing"

func TestParse_SeparatesCompoundCommands(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		programs  []string
		operators []Operator
	}{
		{
			name:      "and operator",
			input:     "git status && git diff",
			programs:  []string{"git", "git"},
			operators: []Operator{OperatorNone, OperatorAnd},
		},
		{
			name:      "semicolon",
			input:     "git status; git diff",
			programs:  []string{"git", "git"},
			operators: []Operator{OperatorNone, OperatorSequence},
		},
		{
			name:      "newline",
			input:     "git status\ngit diff",
			programs:  []string{"git", "git"},
			operators: []Operator{OperatorNone, OperatorNewline},
		},
		{
			name:      "or operator",
			input:     "npm test || npm run lint",
			programs:  []string{"npm", "npm"},
			operators: []Operator{OperatorNone, OperatorOr},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			if len(got.Errors) > 0 {
				t.Fatalf("unexpected parse errors: %v", got.Errors)
			}
			if len(got.Commands) != len(tt.programs) {
				t.Fatalf("got %d commands, want %d: %#v", len(got.Commands), len(tt.programs), got.Commands)
			}
			for i := range tt.programs {
				if got.Commands[i].Program != tt.programs[i] {
					t.Errorf("command %d program = %q, want %q", i, got.Commands[i].Program, tt.programs[i])
				}
				if got.Commands[i].OperatorBefore != tt.operators[i] {
					t.Errorf("command %d operator = %q, want %q", i, got.Commands[i].OperatorBefore, tt.operators[i])
				}
			}
		})
	}
}

func TestParse_DetectsRedirections(t *testing.T) {
	tests := []struct {
		input   string
		feature Feature
	}{
		{"echo ok > out.txt", FeatureRedirectOutput},
		{"echo ok >> out.txt", FeatureRedirectAppend},
		{"cmd 2> err.log", FeatureRedirectError},
		{"cmd 2>> err.log", FeatureRedirectErrorAppend},
		{"sort < in.txt", FeatureRedirectInput},
		{"cat << EOF\nhello\nEOF", FeatureHeredoc},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Parse(tt.input)
			if !hasFeature(got.Features, tt.feature) {
				t.Fatalf("features = %v, want %s", got.Features, tt.feature)
			}
			if !got.RequiresConfirmation() {
				t.Fatalf("redirection should require confirmation")
			}
		})
	}
}

func TestParse_QuotedOperatorsRemainArguments(t *testing.T) {
	got := Parse(`echo "a > b" 'x >> y' "git status && git diff"`)
	if len(got.Errors) > 0 {
		t.Fatalf("unexpected parse errors: %v", got.Errors)
	}
	if len(got.Features) > 0 {
		t.Fatalf("quoted operators should not be features: %v", got.Features)
	}
	if len(got.Commands) != 1 {
		t.Fatalf("got %d commands, want 1", len(got.Commands))
	}
	wantArgs := []string{"a > b", "x >> y", "git status && git diff"}
	if len(got.Commands[0].Args) != len(wantArgs) {
		t.Fatalf("args = %#v, want %#v", got.Commands[0].Args, wantArgs)
	}
	for i, want := range wantArgs {
		if got.Commands[0].Args[i] != want {
			t.Errorf("arg %d = %q, want %q", i, got.Commands[0].Args[i], want)
		}
	}
}

func TestParse_DetectsConservativeFeatures(t *testing.T) {
	tests := []struct {
		input   string
		feature Feature
	}{
		{"echo ok | wc -c", FeaturePipe},
		{"sleep 1 &", FeatureBackground},
		{"echo $(pwd)", FeatureCommandSubstitution},
		{"echo `pwd`", FeatureBacktickSubstitution},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Parse(tt.input)
			if !hasFeature(got.Features, tt.feature) {
				t.Fatalf("features = %v, want %s", got.Features, tt.feature)
			}
			if !got.RequiresConfirmation() {
				t.Fatalf("conservative feature should require confirmation")
			}
		})
	}
}

func TestParse_DetectsSubstitutionInsideDoubleQuotes(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		feature Feature
	}{
		{
			name:    "command substitution in double quotes",
			input:   `echo "$(pwd)"`,
			feature: FeatureCommandSubstitution,
		},
		{
			name:    "backtick substitution in double quotes",
			input:   "echo \"`pwd`\"",
			feature: FeatureBacktickSubstitution,
		},
		{
			name:    "command substitution mixed with literal text",
			input:   `printf "user=%s home=%s\n" "$(whoami)" "$HOME"`,
			feature: FeatureCommandSubstitution,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			if !hasFeature(got.Features, tt.feature) {
				t.Fatalf("features = %v, want %s", got.Features, tt.feature)
			}
			if !got.RequiresConfirmation() {
				t.Fatalf("substitution inside double quotes should require confirmation: %#v", got)
			}
		})
	}
}

func TestParse_EscapedSubstitutionInDoubleQuotesIsLiteral(t *testing.T) {
	got := Parse(`echo "\$(pwd)"`)
	if hasFeature(got.Features, FeatureCommandSubstitution) {
		t.Fatalf("escaped $( should not trigger command substitution: %#v", got)
	}
	if hasFeature(got.Features, FeatureBacktickSubstitution) {
		t.Fatalf("no backticks were used: %#v", got)
	}
	if got.RequiresConfirmation() {
		t.Fatalf("escaped substitution should not require confirmation: %#v", got)
	}
}

func TestParse_SingleQuotesNeverTriggerSubstitution(t *testing.T) {
	got := Parse(`echo '$(pwd)' '` + "`pwd`" + `'`)
	if hasFeature(got.Features, FeatureCommandSubstitution) {
		t.Fatalf("single quotes should not expand $(): %#v", got)
	}
	if hasFeature(got.Features, FeatureBacktickSubstitution) {
		t.Fatalf("single quotes should not expand backticks: %#v", got)
	}
	if got.RequiresConfirmation() {
		t.Fatalf("literal single-quoted text should not require confirmation: %#v", got)
	}
}

func TestParse_AmbiguousSyntaxRequiresConfirmation(t *testing.T) {
	tests := []string{
		`echo "unterminated`,
		"git status &&",
		"echo ok >",
		"&& git status",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			got := Parse(input)
			if !got.RequiresConfirmation() {
				t.Fatalf("ambiguous syntax should require confirmation: %#v", got)
			}
			if len(got.Errors) == 0 && !hasFeature(got.Features, FeatureAmbiguousSyntax) {
				t.Fatalf("expected errors or ambiguous feature, got %#v", got)
			}
		})
	}
}

func hasFeature(features []Feature, want Feature) bool {
	for _, feature := range features {
		if feature == want {
			return true
		}
	}
	return false
}
