package commandsecurity

import (
	"assistente/internal/database"
	"gorm.io/gorm"
)

// Admission checks the captured session. At commit, tx is the connection
// holding the writer lock; outside a transaction it is nil.
type Admission func(tx *gorm.DB, handoff func() error) error

func (admit Admission) Check() error {
	if admit == nil {
		return database.ErrUserScopeRequired
	}
	return admit(nil, func() error { return nil })
}
