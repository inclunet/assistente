package jobs

import (
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"
)

// Parse decodifica YAML em um Job e valida campos obrigatorios.
func Parse(data []byte) (*Job, error) {
	var job Job
	if err := yaml.Unmarshal(data, &job); err != nil {
		return nil, fmt.Errorf("yaml parse error: %w", err)
	}

	if err := Validate(&job); err != nil {
		return nil, err
	}

	return &job, nil
}

// Validate verifica regras de negocio do Job.
func Validate(job *Job) error {
	if job.ID == "" {
		return fmt.Errorf("job validation: 'id' is required")
	}

	if strings.ContainsAny(job.ID, " /\\") {
		return fmt.Errorf("job validation: 'id' must not contain spaces or path separators: %q", job.ID)
	}

	if job.Tool == "" {
		return fmt.Errorf("job validation [%s]: 'tool' is required", job.ID)
	}

	if len(job.Triggers) == 0 {
		return fmt.Errorf("job validation [%s]: at least one trigger is required", job.ID)
	}

	for i, t := range job.Triggers {
		if err := validateTrigger(job.ID, i, &t); err != nil {
			return err
		}
	}

	if err := validateErrorPolicy(job.ID, &job.ErrorPolicy); err != nil {
		return err
	}

	return nil
}

func validateTrigger(jobID string, idx int, t *Trigger) error {
	prefix := fmt.Sprintf("job validation [%s] trigger[%d]", jobID, idx)

	switch t.Type {
	case TriggerCron:
		if t.Expression == "" {
			return fmt.Errorf("%s: cron trigger requires 'expression'", prefix)
		}
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		if _, err := parser.Parse(t.Expression); err != nil {
			return fmt.Errorf("%s: invalid cron expression %q: %w", prefix, t.Expression, err)
		}

	case TriggerInterval:
		if t.Every == "" {
			return fmt.Errorf("%s: interval trigger requires 'every'", prefix)
		}
		if _, err := parseInterval(t.Every); err != nil {
			return fmt.Errorf("%s: invalid interval %q: %w", prefix, t.Every, err)
		}

	case TriggerEvent:
		if t.Listen == "" {
			return fmt.Errorf("%s: event trigger requires 'listen'", prefix)
		}

	case TriggerHotkey:
		if t.Keys == "" {
			return fmt.Errorf("%s: hotkey trigger requires 'keys'", prefix)
		}

	case TriggerManual:
		// nenhuma validacao extra

	case TriggerWebhook:
		if t.Path == "" {
			return fmt.Errorf("%s: webhook trigger requires 'path'", prefix)
		}

	default:
		return fmt.Errorf("%s: unknown trigger type %q", prefix, t.Type)
	}

	return nil
}

func validateErrorPolicy(jobID string, ep *ErrorPolicy) error {
	if ep.Strategy == "" {
		return nil
	}

	switch ep.Strategy {
	case ErrorRetry, ErrorStop, ErrorSkip:
		// ok
	default:
		return fmt.Errorf("job validation [%s]: unknown error strategy %q", jobID, ep.Strategy)
	}

	if ep.Strategy == ErrorRetry && ep.MaxRetries <= 0 {
		return fmt.Errorf("job validation [%s]: retry strategy requires max_retries > 0", jobID)
	}

	if ep.Backoff != "" {
		switch ep.Backoff {
		case BackoffLinear, BackoffExponential, BackoffFixed:
			// ok
		default:
			return fmt.Errorf("job validation [%s]: unknown backoff type %q", jobID, ep.Backoff)
		}
	}

	return nil
}

// parseInterval converte strings como "2h", "30m", "1h30m" em time.Duration.
func parseInterval(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty interval")
	}

	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}

	if d <= 0 {
		return 0, fmt.Errorf("interval must be positive, got %s", d)
	}

	return d, nil
}
