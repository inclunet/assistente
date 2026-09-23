package workspace

import (
	"math"
	"strconv"
	"sync"

	"github.com/google/uuid"
)

// cloneActiveSnapshotLocked clona um workspace para transporte e atribui uma
// sequência de publicação única deste Manager. O chamador deve possuir m.mu;
// snapshotMu protege apenas o contador para que a sequência continue segura
// também quando Active() estiver sob RLock.
//
// A sequência não é uma revisão semântica e não participa de CommandSnapshot,
// commandEpoch ou autorização. Os metadados permanecem somente no clone.
// Em caso de exaustão do contador, retorna nil (fail closed): o consumidor não
// deve aceitar um snapshot sem sequência nem resetar o epoch/contador.
func (m *Manager) cloneActiveSnapshotLocked(source *Workspace) *Workspace {
	if source == nil {
		return nil
	}

	cloned := cloneWorkspace(source)
	// Mesmo que um caller forneça um clone previamente carimbado, o estado de
	// transporte da nova publicação começa limpo e nunca contamina m.active.
	cloned.SnapshotEpoch = ""
	cloned.SnapshotSequence = ""

	m.snapshotMu.Lock()
	defer m.snapshotMu.Unlock()
	if m.snapshotEpoch == "" {
		m.snapshotEpoch = newSnapshotEpoch()
	}
	if m.snapshotSequence == math.MaxUint64 {
		return nil
	}
	m.snapshotSequence++
	sequence := m.snapshotSequence
	epoch := m.snapshotEpoch

	cloned.SnapshotEpoch = epoch
	cloned.SnapshotSequence = strconv.FormatUint(sequence, 10)
	return cloned
}

func newSnapshotEpoch() string {
	return uuid.New().String()
}

// snapshotState is intentionally separate from commandEpoch: publication
// ordering is a transport concern, not command semantics.
type snapshotState struct {
	snapshotMu       sync.Mutex
	snapshotEpoch    string
	snapshotSequence uint64
}
