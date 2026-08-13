package wailsapi

import "context"

// Session é a fonte de contexto autenticado na borda Wails (AEP-0088 D2).
// O *App implementa esta interface reusando requireAuthenticatedContext /
// requireAdminContext.
type Session interface {
	AuthenticatedContext() (context.Context, error)
	AdminContext() (context.Context, error)
}

// WithUser obtém o contexto autenticado via Session e executa fn.
// Fail-closed: se não houver sessão, devolve o erro sem chamar fn.
func WithUser[T any](s Session, fn func(ctx context.Context) (T, error)) (T, error) {
	var zero T
	if s == nil {
		return zero, errNilSession
	}
	ctx, err := s.AuthenticatedContext()
	if err != nil {
		return zero, err
	}
	return fn(ctx)
}

// WithAdmin obtém o contexto autenticado com role admin e executa fn.
// Fail-closed: sem sessão ou sem admin, devolve o erro sem chamar fn.
func WithAdmin[T any](s Session, fn func(ctx context.Context) (T, error)) (T, error) {
	var zero T
	if s == nil {
		return zero, errNilSession
	}
	ctx, err := s.AdminContext()
	if err != nil {
		return zero, err
	}
	return fn(ctx)
}

// WithUser2 é WithUser para callbacks com dois valores de retorno além do error
// (ex.: CheckContextWindowThreshold → bool, float64).
func WithUser2[A, B any](s Session, fn func(ctx context.Context) (A, B, error)) (A, B, error) {
	var zeroA A
	var zeroB B
	if s == nil {
		return zeroA, zeroB, errNilSession
	}
	ctx, err := s.AuthenticatedContext()
	if err != nil {
		return zeroA, zeroB, err
	}
	return fn(ctx)
}
