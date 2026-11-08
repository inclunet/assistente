package commandconfig

import (
	"reflect"
	"slices"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
)

type configRow interface {
	Layer | Binding | Generation | commandactivation.Rule | commandactivation.Claim | commandautomation.Grant
}

func equalRows[T configRow](a, b []T) bool {
	return slices.EqualFunc(a, b, equalRow[T])
}

func equalRow[T configRow](a, b T) bool {
	return reflect.DeepEqual(comparisonRow(a), comparisonRow(b))
}

// Datas persistidas representam instantes, não a Location interna do Go nem
// relógio monotônico. Normaliza somente cópias para comparação; nenhum campo
// é ignorado e os valores usados em SQL, no preview e no diff permanecem iguais.
func comparisonRow[T configRow](row T) T {
	switch value := any(row).(type) {
	case Layer:
		value.CreatedAt, value.UpdatedAt = value.CreatedAt.UTC(), value.UpdatedAt.UTC()
		return any(value).(T)
	case Generation:
		value.UpdatedAt = value.UpdatedAt.UTC()
		return any(value).(T)
	case commandautomation.Grant:
		value.GrantedAt = value.GrantedAt.UTC()
		value.RevokedAt = comparisonTime(value.RevokedAt)
		return any(value).(T)
	case commandactivation.Claim:
		value.ActivatedAt, value.UpdatedAt = value.ActivatedAt.UTC(), value.UpdatedAt.UTC()
		value.ExpiresAt = comparisonTime(value.ExpiresAt)
		value.SourceReplayDeadline = comparisonTime(value.SourceReplayDeadline)
		return any(value).(T)
	default:
		return row
	}
}

func comparisonTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	utc := value.UTC()
	return &utc
}

// MatchesSnapshot confere o estado anterior ao diff, usado pelas portas desktop
// e agent antes do commit. A validação das gerações continua no Store.
func (diff MutationDiff) MatchesSnapshot(snapshot Snapshot) bool {
	return diff.Scope.UserID == snapshot.Scope.UserID && sameWorkspace(diff.Scope.WorkspaceID, snapshot.Scope.WorkspaceID) &&
		equalRows(diff.BeforeLayers, snapshot.Layers) &&
		equalRows(diff.BeforeBindings, snapshot.Bindings) &&
		equalRows(diff.BeforeActivationRules, snapshot.ActivationRules) &&
		equalRows(diff.BeforeAutomationGrants, snapshot.AutomationGrants) &&
		equalRows(diff.BeforeActivationClaims, snapshot.ActivationClaims)
}
