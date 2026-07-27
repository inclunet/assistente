package channels

import (
	"errors"
	"testing"

	"gorm.io/gorm"
)

func TestIsUniqueConstraintError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("something else"), false},
		{gorm.ErrDuplicatedKey, true},
		{errors.New("UNIQUE constraint failed: channel_contacts.channel_id"), true},
		{errors.New("duplicate key value violates unique constraint"), true},
		{errors.New("Error 1062: Duplicate entry"), true},
		// "UNIQUE" sozinho não basta — evita mascarar erros genéricos.
		{errors.New("UNIQUE token missing"), false},
	}
	for _, tc := range cases {
		if got := isUniqueConstraintError(tc.err); got != tc.want {
			t.Fatalf("isUniqueConstraintError(%v)=%v want %v", tc.err, got, tc.want)
		}
	}
}
