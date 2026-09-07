package auth

import (
	"errors"
	"testing"
)

// TestIsUniqueConstraintError_DetectsAcrossDialects valida o vetor M5
// (re-review pós-correção do PR #94): a heurística precisa identificar
// violação de unique index nas mensagens dos drivers que o AEP-0052
// prevê em deployment futuro (SQLite hoje, Postgres/MySQL amanhã).
//
// Sem isso, criar usuário duplicado em qualquer dialect que não SQLite
// vira erro genérico e o caller perde a chance de mapear para
// `ErrUserExists` — UX silenciosamente quebrada na primeira migração.
func TestIsUniqueConstraintError_DetectsAcrossDialects(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "sqlite_modernc",
			err:  errors.New("constraint failed: UNIQUE constraint failed: users.username (2067)"),
			want: true,
		},
		{
			name: "sqlite_legacy_message",
			err:  errors.New("UNIQUE constraint failed: users.username"),
			want: true,
		},
		{
			name: "postgres_pq",
			err:  errors.New("pq: duplicate key value violates unique constraint \"users_username_key\""),
			want: true,
		},
		{
			name: "postgres_pgx",
			err:  errors.New("ERROR: duplicate key value violates unique constraint \"users_username_key\" (SQLSTATE 23505)"),
			want: true,
		},
		{
			name: "mysql",
			err:  errors.New("Error 1062: Duplicate entry 'alice' for key 'users.username'"),
			want: true,
		},
		{
			name: "unrelated_error",
			err:  errors.New("connection refused"),
			want: false,
		},
		{
			name: "nil_error",
			err:  nil,
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isUniqueConstraintError(tc.err); got != tc.want {
				t.Fatalf("isUniqueConstraintError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
