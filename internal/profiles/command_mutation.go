package profiles

import (
	"assistente/internal/configdir"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	CommandMutationCreate    = "create"
	CommandMutationUpdate    = "update"
	CommandMutationDuplicate = "duplicate"
	CommandMutationDelete    = "delete"
	CommandMutationActivate  = "activate"
)

var (
	ErrInvalidCommandMutation        = errors.New("invalid profile command mutation")
	ErrStaleCommandMutation          = errors.New("stale profile command mutation")
	ErrConsumedCommandMutation       = errors.New("profile command mutation already committed")
	ErrCommandMutationOutcomeUnknown = errors.New("profile command mutation outcome unknown")
	ErrCommandMutationRolledBack     = errors.New("profile command mutation rolled back")
)

// MutationImpact é a consequência efetiva de uma mutação de profiles.
//
// O valor é derivado novamente do plano sob os locks do Manager; o caller não
// pode fornecer ou substituir qualquer parte dele. AffectedSlugs é copiado
// para cada callback, portanto uma callback não consegue alterar o impacto
// observado pela outra callback nem o estado interno da mutação.
type MutationImpact struct {
	Operation              string
	ResultSlug             string
	AffectedSlugs          []string
	DeletedSlug            string
	OriginalTargetIdentity string
}

// CommandMutation é uma mutação preparada contra um fingerprint de leitura.
// A preparação não reserva o perfil nem mantém locks; o CAS é repetido no
// CommitCoordinated, que é o único ponto da mutação preparada que materializa arquivos.
type CommandMutation struct {
	manager             *Manager
	operation           string
	slug                string
	profile             *Profile
	expectedFingerprint string
	stateMu             sync.Mutex
	committed           bool
}

type commandMutationPlan struct {
	resultSlug             string
	changes                []configdir.ProfileFileChange
	originalTargetIdentity string
}

type profileFingerprintFile struct {
	Filename string `json:"filename"`
	Path     string `json:"path"`
	Source   string `json:"source"`
	ModTime  int64  `json:"mtime_unix_nano"`
	Size     int64  `json:"size"`
	Data     []byte `json:"data"`
}

type profileFingerprintDocument struct {
	Version     int                      `json:"version"`
	Files       []profileFingerprintFile `json:"files"`
	ActiveSlug  string                   `json:"active_slug"`
	ActiveFound bool                     `json:"active_found"`
}

// ReadCommandTarget carrega o alvo e seu fingerprint na mesma seção crítica.
// A API é destinada a formulários: o caller não precisa combinar Get com um
// snapshot separado sujeito a uma alteração intermediária.
func (m *Manager) ReadCommandTarget(slug string) (*Profile, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.recoverLocked(); err != nil {
		return nil, "", err
	}
	profile, err := m.getLocked(slug)
	if err != nil {
		return nil, "", err
	}
	fingerprint, err := m.commandFingerprintLocked()
	if err != nil {
		return nil, "", err
	}
	return profile, fingerprint, nil
}

// ReadActiveCommandTarget resolve o profile ativo, seu slug e o fingerprint
// observável na mesma seção crítica. Isso evita que um caller combine
// GetActive/GetActiveSlug com uma leitura posterior e prepare uma mutação para
// um alvo diferente do profile que acabou de apresentar.
func (m *Manager) ReadActiveCommandTarget() (*Profile, string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.recoverLocked(); err != nil {
		return nil, "", "", err
	}
	profile, slug, err := m.resolveActiveLocked(false)
	if err != nil {
		return nil, "", "", err
	}
	fingerprint, err := m.commandFingerprintLocked()
	if err != nil {
		return nil, "", "", err
	}
	return profile, slug, fingerprint, nil
}

// CommandMutationSnapshot captura a versão observável dos perfis resolvidos e
// da resolução do ativo. O slug é usado pelo plano da operação, enquanto o
// fingerprint representa o conjunto inteiro para que ativação e criação
// ativa não aceitem uma leitura parcial.
func (m *Manager) CommandMutationSnapshot(slug string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.recoverLocked(); err != nil {
		return "", err
	}
	return m.commandFingerprintLocked()
}

// PrepareCommandMutation valida a operação e o fingerprint no momento da
// preparação. CommitCoordinated repete a validação; preparar não escreve configuração.
func (m *Manager) PrepareCommandMutation(operation, slug string, profile *Profile, expectedFingerprint string) (*CommandMutation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.recoverLocked(); err != nil {
		return nil, err
	}
	operation = strings.ToLower(strings.TrimSpace(operation))
	if strings.TrimSpace(expectedFingerprint) == "" {
		return nil, fmt.Errorf("%w: expected fingerprint is required", ErrInvalidCommandMutation)
	}
	if !validCommandMutationOperation(operation) {
		return nil, fmt.Errorf("%w: unsupported operation %q", ErrInvalidCommandMutation, operation)
	}
	ownedProfile, err := cloneProfile(profile)
	if err != nil {
		return nil, err
	}
	if _, err := m.buildCommandMutationPlanLocked(operation, slug, ownedProfile); err != nil {
		return nil, err
	}
	current, err := m.commandFingerprintLocked()
	if err != nil {
		return nil, err
	}
	if current != expectedFingerprint {
		return nil, fmt.Errorf("%w: expected %s, current %s", ErrStaleCommandMutation, expectedFingerprint, current)
	}
	return &CommandMutation{manager: m, operation: operation, slug: slug, profile: ownedProfile, expectedFingerprint: expectedFingerprint}, nil
}

// CommitCoordinated revalida o CAS sob os locks da mutação e do Manager,
// deriva o impacto do plano final e oferece duas barreiras para o coordenador.
// before ocorre depois do CAS final e antes de qualquer escrita; after ocorre
// somente depois de a gravação do plano terminar com sucesso. As callbacks
// são executadas sob os dois locks e não devem chamar métodos públicos do
// Manager.
func (m *CommandMutation) CommitCoordinated(before func(MutationImpact) error, after func(MutationImpact) error) (string, error) {
	if m == nil || m.manager == nil {
		return "", fmt.Errorf("%w: nil mutation", ErrInvalidCommandMutation)
	}
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	if m.committed {
		return "", ErrConsumedCommandMutation
	}
	m.manager.mu.Lock()
	defer m.manager.mu.Unlock()
	if err := m.manager.recoverLocked(); err != nil {
		return "", err
	}
	current, err := m.manager.commandFingerprintLocked()
	if err != nil {
		return "", err
	}
	if current != m.expectedFingerprint {
		return "", fmt.Errorf("%w: expected %s, current %s", ErrStaleCommandMutation, m.expectedFingerprint, current)
	}
	plan, err := m.manager.buildCommandMutationPlanLocked(m.operation, m.slug, m.profile)
	if err != nil {
		return "", err
	}
	// O plano faz novas leituras. Repetir a captura antes do journal fecha a
	// janela de uma escrita externa entre o primeiro CAS e a montagem do plano.
	latest, err := m.manager.commandFingerprintLocked()
	if err != nil {
		return "", err
	}
	if latest != current {
		return "", fmt.Errorf("%w: state changed while preparing commit", ErrStaleCommandMutation)
	}
	impact, err := m.manager.mutationImpactLocked(m.operation, plan)
	if err != nil {
		return "", err
	}
	if before != nil {
		if err := before(cloneMutationImpact(impact)); err != nil {
			return "", err
		}
	}
	// A journal is durable before the first file rename. From this point on a
	// failure is conservatively terminal for this token, even if the error
	// happened while writing the journal rather than while renaming a file.
	m.committed = true
	if err := m.manager.commitChangesLocked(plan.changes); err != nil {
		return "", errors.Join(ErrCommandMutationOutcomeUnknown, err)
	}
	if after != nil {
		if err := after(cloneMutationImpact(impact)); err != nil {
			if m.operation != CommandMutationDelete {
				return "", errors.Join(ErrCommandMutationOutcomeUnknown, err)
			}
			rollbackErr := m.manager.commitChangesLocked(inverseProfileChanges(plan.changes))
			if rollbackErr != nil {
				return "", errors.Join(ErrCommandMutationOutcomeUnknown, err, rollbackErr)
			}
			return "", errors.Join(ErrCommandMutationRolledBack, err)
		}
	}
	return plan.resultSlug, nil
}

func cloneMutationImpact(impact MutationImpact) MutationImpact {
	impact.AffectedSlugs = append([]string(nil), impact.AffectedSlugs...)
	return impact
}

func inverseProfileChanges(changes []configdir.ProfileFileChange) []configdir.ProfileFileChange {
	inverse := make([]configdir.ProfileFileChange, len(changes))
	for i, change := range changes {
		inverse[i] = configdir.ProfileFileChange{
			Path:         change.Path,
			Before:       append([]byte(nil), change.After...),
			BeforeExists: change.AfterExists,
			After:        append([]byte(nil), change.Before...),
			AfterExists:  change.BeforeExists,
		}
	}
	return inverse
}

func validCommandMutationOperation(operation string) bool {
	switch operation {
	case CommandMutationCreate, CommandMutationUpdate, CommandMutationDuplicate, CommandMutationDelete, CommandMutationActivate:
		return true
	default:
		return false
	}
}

func (m *Manager) commitLegacyLocked(operation, slug string, profile *Profile) (string, error) {
	plan, err := m.buildCommandMutationPlanLocked(operation, slug, profile)
	if err != nil {
		return "", err
	}
	if err := m.commitChangesLocked(plan.changes); err != nil {
		return "", errors.Join(ErrCommandMutationOutcomeUnknown, err)
	}
	return plan.resultSlug, nil
}

func (m *Manager) mutationImpactLocked(operation string, plan commandMutationPlan) (MutationImpact, error) {
	affected := make([]string, 0, len(plan.changes))
	seen := make(map[string]struct{}, len(plan.changes))
	for _, change := range plan.changes {
		filename := filepath.Base(filepath.Clean(change.Path))
		if filepath.Ext(filename) != ".json" {
			continue
		}
		slug := strings.TrimSuffix(filename, filepath.Ext(filename))
		if slug == "" {
			continue
		}
		if _, ok := seen[slug]; ok {
			continue
		}
		seen[slug] = struct{}{}
		affected = append(affected, slug)
	}
	sort.Strings(affected)
	impact := MutationImpact{
		Operation:              operation,
		ResultSlug:             plan.resultSlug,
		AffectedSlugs:          affected,
		OriginalTargetIdentity: plan.originalTargetIdentity,
	}
	if operation == CommandMutationDelete {
		impact.DeletedSlug = plan.resultSlug
	}
	return impact, nil
}

func (m *Manager) buildCommandMutationPlanLocked(operation, slug string, profile *Profile) (commandMutationPlan, error) {
	if !validCommandMutationOperation(operation) {
		return commandMutationPlan{}, fmt.Errorf("%w: unsupported operation %q", ErrInvalidCommandMutation, operation)
	}
	switch operation {
	case CommandMutationCreate:
		return m.planCreateLocked(slug, profile)
	case CommandMutationUpdate:
		return m.planUpdateLocked(slug, profile)
	case CommandMutationDuplicate:
		return m.planDuplicateLocked(slug)
	case CommandMutationDelete:
		return m.planDeleteLocked(slug)
	case CommandMutationActivate:
		return m.planActivateLocked(slug)
	default:
		panic("validated command mutation operation disappeared")
	}
}

func (m *Manager) planCreateLocked(slug string, profile *Profile) (commandMutationPlan, error) {
	if profile == nil {
		return commandMutationPlan{}, fmt.Errorf("%w: create requires profile", ErrInvalidCommandMutation)
	}
	if err := profile.Validate(); err != nil {
		return commandMutationPlan{}, err
	}
	derivedSlug := Slugify(profile.Name)
	if slug != "" && slug != derivedSlug {
		return commandMutationPlan{}, fmt.Errorf("%w: create slug %q differs from profile slug %q", ErrInvalidCommandMutation, slug, derivedSlug)
	}
	slug = derivedSlug
	filename := slug + ".json"
	if m.resolver.Exists(filename) {
		return commandMutationPlan{}, fmt.Errorf("profile already exists: %s", slug)
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return commandMutationPlan{}, err
	}
	home := m.resolver.GetHomeDir()
	if home == "" {
		return commandMutationPlan{}, fmt.Errorf("profile home directory unavailable")
	}
	changes := []configdir.ProfileFileChange{{Path: filepath.Join(home, filename), After: data, AfterExists: true}}
	if profile.Active {
		deactivate, err := m.deactivateVisibleProfilesLocked(slug)
		if err != nil {
			return commandMutationPlan{}, err
		}
		changes = append(changes, deactivate...)
	}
	return commandMutationPlan{resultSlug: slug, changes: changes}, nil
}

func (m *Manager) planUpdateLocked(slug string, profile *Profile) (commandMutationPlan, error) {
	if slug == "" || profile == nil {
		return commandMutationPlan{}, fmt.Errorf("%w: update requires slug and profile", ErrInvalidCommandMutation)
	}
	if err := profile.Validate(); err != nil {
		return commandMutationPlan{}, err
	}
	resolved, before, err := m.resolveProfileFileLocked(slug)
	if err != nil {
		return commandMutationPlan{}, err
	}
	original, err := m.getLocked(slug)
	if err != nil {
		return commandMutationPlan{}, err
	}
	originalIdentity, err := profileIdentity(original)
	if err != nil {
		return commandMutationPlan{}, err
	}
	after, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return commandMutationPlan{}, err
	}
	changes := []configdir.ProfileFileChange{{Path: resolved.Path, Before: before, BeforeExists: true, After: after, AfterExists: true}}
	if profile.Active {
		deactivate, err := m.deactivateVisibleProfilesLocked(slug)
		if err != nil {
			return commandMutationPlan{}, err
		}
		changes = append(changes, deactivate...)
	}
	return commandMutationPlan{resultSlug: slug, changes: changes, originalTargetIdentity: originalIdentity}, nil
}

func (m *Manager) planDuplicateLocked(slug string) (commandMutationPlan, error) {
	if slug == "" {
		return commandMutationPlan{}, fmt.Errorf("%w: duplicate requires source slug", ErrInvalidCommandMutation)
	}
	profile, err := m.getLocked(slug)
	if err != nil {
		return commandMutationPlan{}, err
	}
	originalIdentity, err := profileIdentity(profile)
	if err != nil {
		return commandMutationPlan{}, err
	}
	newProfile, err := cloneProfile(profile)
	if err != nil {
		return commandMutationPlan{}, err
	}
	newProfile.Name = m.nextCopyName(profile.Name)
	newProfile.Active = false
	newProfile.BuiltinVersion = ""
	newSlug := Slugify(newProfile.Name)
	data, err := json.MarshalIndent(newProfile, "", "  ")
	if err != nil {
		return commandMutationPlan{}, err
	}
	home := m.resolver.GetHomeDir()
	if home == "" {
		return commandMutationPlan{}, fmt.Errorf("profile home directory unavailable")
	}
	return commandMutationPlan{resultSlug: newSlug, originalTargetIdentity: originalIdentity, changes: []configdir.ProfileFileChange{{Path: filepath.Join(home, newSlug+".json"), After: data, AfterExists: true}}}, nil
}

func (m *Manager) planDeleteLocked(slug string) (commandMutationPlan, error) {
	if slug == "" {
		return commandMutationPlan{}, fmt.Errorf("%w: delete requires slug", ErrInvalidCommandMutation)
	}
	resolved, before, err := m.resolveProfileFileLocked(slug)
	if err != nil {
		return commandMutationPlan{}, err
	}
	original, err := m.getLocked(slug)
	if err != nil {
		return commandMutationPlan{}, err
	}
	originalIdentity, err := profileIdentity(original)
	if err != nil {
		return commandMutationPlan{}, err
	}
	_, activeSlug, err := m.resolveActiveLocked(false)
	if err != nil {
		return commandMutationPlan{}, err
	}
	if activeSlug == slug {
		return commandMutationPlan{}, fmt.Errorf("não é possível deletar o perfil ativo")
	}
	return commandMutationPlan{resultSlug: slug, originalTargetIdentity: originalIdentity, changes: []configdir.ProfileFileChange{{Path: resolved.Path, Before: before, BeforeExists: true}}}, nil
}

func (m *Manager) planActivateLocked(slug string) (commandMutationPlan, error) {
	if slug == "" {
		return commandMutationPlan{}, fmt.Errorf("%w: activate requires slug", ErrInvalidCommandMutation)
	}
	resolved, before, err := m.resolveProfileFileLocked(slug)
	if err != nil {
		return commandMutationPlan{}, err
	}
	profile, err := m.getLocked(slug)
	if err != nil {
		return commandMutationPlan{}, err
	}
	originalIdentity, err := profileIdentity(profile)
	if err != nil {
		return commandMutationPlan{}, err
	}
	profile.Active = true
	after, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return commandMutationPlan{}, err
	}
	changes := []configdir.ProfileFileChange{{Path: resolved.Path, Before: before, BeforeExists: true, After: after, AfterExists: true}}
	deactivate, err := m.deactivateVisibleProfilesLocked(slug)
	if err != nil {
		return commandMutationPlan{}, err
	}
	changes = append(changes, deactivate...)
	return commandMutationPlan{resultSlug: slug, originalTargetIdentity: originalIdentity, changes: changes}, nil
}

func (m *Manager) resolveProfileFileLocked(slug string) (*configdir.ResolvedFile, []byte, error) {
	if slug == "" {
		return nil, nil, fmt.Errorf("%w: empty profile slug", ErrInvalidCommandMutation)
	}
	resolved, err := m.resolver.Resolve(slug + ".json")
	if err != nil {
		return nil, nil, fmt.Errorf("profile not found: %s", slug)
	}
	data, err := os.ReadFile(resolved.Path)
	if err != nil {
		return nil, nil, err
	}
	return resolved, data, nil
}

func (m *Manager) deactivateVisibleProfilesLocked(keepSlug string) ([]configdir.ProfileFileChange, error) {
	files, err := m.resolver.List()
	if err != nil {
		return nil, err
	}
	changes := make([]configdir.ProfileFileChange, 0)
	for _, file := range files {
		if !strings.HasSuffix(file.Filename, ".json") {
			continue
		}
		slug := strings.TrimSuffix(file.Filename, ".json")
		if slug == keepSlug {
			continue
		}
		profile, err := m.getLocked(slug)
		if err != nil || !profile.Active {
			continue
		}
		before, err := os.ReadFile(file.Path)
		if err != nil {
			return nil, err
		}
		profile.Active = false
		after, err := json.MarshalIndent(profile, "", "  ")
		if err != nil {
			return nil, err
		}
		changes = append(changes, configdir.ProfileFileChange{Path: file.Path, Before: before, BeforeExists: true, After: after, AfterExists: true})
	}
	return changes, nil
}

func (m *Manager) deactivateChangesLocked(actives []activeCandidate, keepSlug string) ([]configdir.ProfileFileChange, error) {
	changes := make([]configdir.ProfileFileChange, 0, len(actives))
	for _, candidate := range actives {
		if candidate.slug == keepSlug {
			continue
		}
		before, err := os.ReadFile(candidate.path)
		if err != nil {
			return nil, err
		}
		candidate.profile.Active = false
		after, err := json.MarshalIndent(candidate.profile, "", "  ")
		if err != nil {
			return nil, err
		}
		changes = append(changes, configdir.ProfileFileChange{Path: candidate.path, Before: before, BeforeExists: true, After: after, AfterExists: true})
	}
	return changes, nil
}

func (m *Manager) commitChangesLocked(changes []configdir.ProfileFileChange) error {
	journalPath, err := m.transactionJournalPathLocked()
	if err != nil {
		return err
	}
	return configdir.CommitProfileTransaction(journalPath, m.resolver.GetSearchPaths(), changes)
}

func (m *Manager) recoverLocked() error {
	journalPath, err := m.transactionJournalPathLocked()
	if err != nil {
		return err
	}
	return configdir.RecoverProfileTransaction(journalPath, m.resolver.GetSearchPaths())
}

func (m *Manager) transactionJournalPathLocked() (string, error) {
	home := m.resolver.GetHomeDir()
	if home == "" {
		return "", fmt.Errorf("profile home directory unavailable")
	}
	return filepath.Join(home, ".profile-mutation.journal"), nil
}

func (m *Manager) commandFingerprintLocked() (string, error) {
	files, err := m.resolver.List()
	if err != nil {
		return "", err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Filename < files[j].Filename })
	document := profileFingerprintDocument{Version: 1, Files: make([]profileFingerprintFile, 0)}
	for _, file := range files {
		if !strings.HasSuffix(file.Filename, ".json") {
			continue
		}
		data, err := os.ReadFile(file.Path)
		if err != nil {
			return "", err
		}
		info, err := os.Stat(file.Path)
		if err != nil {
			return "", err
		}
		document.Files = append(document.Files, profileFingerprintFile{
			Filename: file.Filename,
			Path:     filepath.Clean(file.Path),
			Source:   string(file.Source),
			ModTime:  info.ModTime().UnixNano(),
			Size:     info.Size(),
			Data:     data,
		})
	}
	_, activeSlug, err := m.resolveActiveLocked(false)
	if err != nil {
		return "", err
	}
	document.ActiveSlug = activeSlug
	document.ActiveFound = activeSlug != ""
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func cloneProfile(profile *Profile) (*Profile, error) {
	if profile == nil {
		return nil, nil
	}
	data, err := json.Marshal(profile)
	if err != nil {
		return nil, err
	}
	var cloned Profile
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil, err
	}
	return &cloned, nil
}

func profileIdentity(profile *Profile) (string, error) {
	if profile == nil {
		return "", nil
	}
	data, err := json.Marshal(profile)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
