package commandsecurity

import "assistente/internal/database"

type Admission func(func() error) error

func (admit Admission) Check() error {
	if admit == nil {
		return database.ErrUserScopeRequired
	}
	return admit(func() error { return nil })
}
