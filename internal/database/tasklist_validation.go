package database

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// TaskListValidationPolicy regras opcionais por tasklist (JSON em TaskList.ValidationPolicy).
// Campos vazios / listas vazias desativam a respectiva checagem.
type TaskListValidationPolicy struct {
	// TaskCodeRegex: se não vazio, todo task.code não vazio deve casar com esta expressão (Go regexp).
	TaskCodeRegex string `json:"task_code_regex,omitempty"`
	// AllowedNoteSources: se não vazio, source da nota externa deve ser um dos valores (match case-insensitive após trim).
	AllowedNoteSources []string `json:"allowed_note_sources,omitempty"`
	// NoteExternalIDRegex: se não vazio, external_id da nota deve casar.
	NoteExternalIDRegex string `json:"note_external_id_regex,omitempty"`
	// NoteExternalParentIDRegex: se não vazio e external_parent_id não vazio, o parent deve casar.
	NoteExternalParentIDRegex string `json:"note_external_parent_id_regex,omitempty"`
}

// ParseTaskListValidationPolicyJSON interpreta o campo validation_policy da tasklist.
func ParseTaskListValidationPolicyJSON(raw string) (*TaskListValidationPolicy, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return &TaskListValidationPolicy{}, nil
	}
	var p TaskListValidationPolicy
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return nil, fmt.Errorf("validation_policy: JSON inválido: %w", err)
	}
	return &p, nil
}

func loadTaskListValidationPolicy(taskListID string) (*TaskListValidationPolicy, error) {
	return loadTaskListValidationPolicyWithContext(context.Background(), taskListID)
}

func loadTaskListValidationPolicyWithContext(ctx context.Context, taskListID string) (*TaskListValidationPolicy, error) {
	var tl TaskList
	if err := ScopeByUser(ctx, db.WithContext(ctx).Select("validation_policy"), "user_id").First(&tl, "id = ?", taskListID).Error; err != nil {
		return nil, err
	}
	return ParseTaskListValidationPolicyJSON(tl.ValidationPolicy)
}

// ValidateTaskCodeAgainstPolicy aplica task_code_regex quando configurado. Code vazio não é validado.
func ValidateTaskCodeAgainstPolicy(code string, p *TaskListValidationPolicy) error {
	code = strings.TrimSpace(code)
	if code == "" || p == nil {
		return nil
	}
	pat := strings.TrimSpace(p.TaskCodeRegex)
	if pat == "" {
		return nil
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return fmt.Errorf("task_code_regex da tasklist é inválido: %w", err)
	}
	if !re.MatchString(code) {
		return fmt.Errorf("task code %q não corresponde ao padrão configurado nesta lista (task_code_regex)", code)
	}
	return nil
}

// ValidateTaskCodeForTaskList aplica task_code_regex quando configurado. Code vazio não é validado.
func ValidateTaskCodeForTaskList(taskListID string, code string) error {
	return ValidateTaskCodeForTaskListWithContext(context.Background(), taskListID, code)
}

func ValidateTaskCodeForTaskListWithContext(ctx context.Context, taskListID string, code string) error {
	p, err := loadTaskListValidationPolicyWithContext(ctx, taskListID)
	if err != nil {
		return err
	}
	return ValidateTaskCodeAgainstPolicy(code, p)
}

// ValidateExternalNoteAgainstPolicy aplica allowed_note_sources e regexes de nota quando configurados.
func ValidateExternalNoteAgainstPolicy(source, externalID, externalParentID string, p *TaskListValidationPolicy) error {
	if p == nil {
		return nil
	}
	src := strings.TrimSpace(source)
	extID := strings.TrimSpace(externalID)
	parent := strings.TrimSpace(externalParentID)

	if len(p.AllowedNoteSources) > 0 {
		lsrc := strings.ToLower(src)
		ok := false
		for _, a := range p.AllowedNoteSources {
			if strings.ToLower(strings.TrimSpace(a)) == lsrc {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("source %q não está em allowed_note_sources desta lista", src)
		}
	}

	if pat := strings.TrimSpace(p.NoteExternalIDRegex); pat != "" {
		re, err := regexp.Compile(pat)
		if err != nil {
			return fmt.Errorf("note_external_id_regex da tasklist é inválido: %w", err)
		}
		if !re.MatchString(extID) {
			return fmt.Errorf("external_id %q não corresponde ao padrão desta lista (note_external_id_regex)", extID)
		}
	}

	if pat := strings.TrimSpace(p.NoteExternalParentIDRegex); pat != "" && parent != "" {
		re, err := regexp.Compile(pat)
		if err != nil {
			return fmt.Errorf("note_external_parent_id_regex da tasklist é inválido: %w", err)
		}
		if !re.MatchString(parent) {
			return fmt.Errorf("external_parent_id %q não corresponde ao padrão desta lista (note_external_parent_id_regex)", parent)
		}
	}

	return nil
}

// ValidateExternalNoteForTaskList aplica allowed_note_sources e regexes de nota quando configurados.
func ValidateExternalNoteForTaskList(taskListID string, source, externalID, externalParentID string) error {
	return ValidateExternalNoteForTaskListWithContext(context.Background(), taskListID, source, externalID, externalParentID)
}

func ValidateExternalNoteForTaskListWithContext(ctx context.Context, taskListID string, source, externalID, externalParentID string) error {
	p, err := loadTaskListValidationPolicyWithContext(ctx, taskListID)
	if err != nil {
		return err
	}
	return ValidateExternalNoteAgainstPolicy(source, externalID, externalParentID, p)
}

// SetTaskListValidationPolicy persiste o JSON da política (string vazia = sem regras).
func SetTaskListValidationPolicy(taskListID string, policyJSON string) error {
	return SetTaskListValidationPolicyWithContext(context.Background(), taskListID, policyJSON)
}

func SetTaskListValidationPolicyWithContext(ctx context.Context, taskListID string, policyJSON string) error {
	s := strings.TrimSpace(policyJSON)
	if s != "" {
		if _, err := ParseTaskListValidationPolicyJSON(s); err != nil {
			return err
		}
	}
	return ScopeByUser(ctx, db.WithContext(ctx).Model(&TaskList{}), "user_id").Where("id = ?", taskListID).Update("validation_policy", s).Error
}
