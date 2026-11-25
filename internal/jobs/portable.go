package jobs

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

// PortableJob is a definition for review. Runtime state, identity and
// grants are established in the destination account.
type PortableJob struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Tool        string            `json:"tool"`
	Triggers    []PortableTrigger `json:"triggers"`
	Inputs      map[string]any    `json:"inputs,omitempty"`
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
	job := &Job{ID: key, Name: d.Name, Description: d.Description, Tool: d.Tool, Inputs: d.Inputs}
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
