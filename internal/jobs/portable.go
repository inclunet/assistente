package jobs

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// PortableJob is a definition for review. Runtime state, identity and
// grants are established in the destination account.
type PortableJob struct {
	MaxRunsPerHour int               `json:"maxRunsPerHour,omitempty"`
	RequiredInputs []string          `json:"requiredInputs,omitempty"`
	Name           string            `json:"name"`
	Description    string            `json:"description,omitempty"`
	Tool           string            `json:"tool"`
	Triggers       []PortableTrigger `json:"triggers"`
	Inputs         map[string]any    `json:"inputs,omitempty"`
}

type PortableTrigger struct {
	Type       TriggerType `json:"type"`
	Every      string      `json:"every,omitempty"`
	Expression string      `json:"expression,omitempty"`
}

var portableJobTool = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

func ReadPortableJob(key string, data []byte) (*Job, error) {
	if len(data) > 64<<10 || !utf8.Valid(data) {
		return nil, errors.New("invalid portable job encoding or size")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var d PortableJob
	if err := decoder.Decode(&d); err != nil {
		return nil, err
	}
	if decoder.Decode(new(any)) != io.EOF || strings.TrimSpace(d.Name) == "" || !portableJobTool.MatchString(d.Tool) || len(d.Triggers) > 32 {
		return nil, errors.New("invalid portable job")
	}
	seen := map[string]bool{}
	for _, field := range d.RequiredInputs {
		_, supplied := d.Inputs[field]
		if supplied || !portableJobTool.MatchString(field) || seen[field] {
			return nil, errors.New("invalid required job input")
		}
		seen[field] = true
	}
	if d.MaxRunsPerHour < 0 {
		return nil, errors.New("invalid portable rate limit")
	}
	job := &Job{MaxRunsPerHour: d.MaxRunsPerHour, RequiredInputs: d.RequiredInputs, ID: key, Name: d.Name, Description: d.Description, Tool: d.Tool, Inputs: d.Inputs}
	for _, t := range d.Triggers {
		switch t.Type {
		case TriggerManual, TriggerCron, TriggerInterval:
		default:
			return nil, errors.New("unsupported portable trigger")
		}
		if t.Expression != "" && t.Type != TriggerCron || t.Every != "" && t.Type != TriggerInterval {
			return nil, errors.New("unexpected portable trigger field")
		}
		job.Triggers = append(job.Triggers, Trigger{Type: t.Type, Every: t.Every, Expression: t.Expression})
	}
	if err := Validate(job); err != nil {
		return nil, err
	}
	return job, nil
}

// ExportPortableJob carries input names so the destination can configure its own values.
func ExportPortableJob(job *Job) ([]byte, error) {
	if job.Pipeline != "" || job.DryRun.Enabled || job.Events != (EventsConfig{}) || len(job.Output.Map) > 0 || (job.ErrorPolicy.Strategy != "" && job.ErrorPolicy.Strategy != ErrorStop) || job.ErrorPolicy.OnExhausted == OnExhaustedNotify {
		return nil, errors.New("PLUGIN_BUILDER_JOB_UNAVAILABLE")
	}
	definition := PortableJob{MaxRunsPerHour: job.MaxRunsPerHour, Name: job.Name, Description: job.Description, Tool: job.Tool}
	seen := map[string]bool{}
	for _, field := range job.RequiredInputs {
		seen[field] = true
	}
	for field := range job.Inputs {
		seen[field] = true
	}
	for field := range seen {
		definition.RequiredInputs = append(definition.RequiredInputs, field)
	}
	slices.Sort(definition.RequiredInputs)
	for _, t := range job.Triggers {
		if t.When != "" || t.Listen != "" || t.Keys != "" || t.Path != "" {
			return nil, errors.New("unsupported portable trigger")
		}
		definition.Triggers = append(definition.Triggers, PortableTrigger{Type: t.Type, Every: t.Every, Expression: t.Expression})
	}
	raw, err := json.MarshalIndent(definition, "", "  ")
	if err != nil {
		return nil, err
	}
	_, err = ReadPortableJob(job.ID, raw)
	return raw, err
}
