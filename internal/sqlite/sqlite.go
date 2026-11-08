// Package sqlite centraliza o adaptador GORM e o formato de armazenamento usado
// pelos bancos existentes. A execução permanece pure Go, inclusive sem CGO.
package sqlite

import (
	"net/url"
	"strings"

	ormsqlite "gorm.io/driver/sqlite"
	"gorm.io/gorm"
	driver "modernc.org/sqlite"
	constants "modernc.org/sqlite/lib"
)

type Dialector struct{ ormsqlite.Dialector }

func Open(dsn string) gorm.Dialector {
	_, query, hasQuery := strings.Cut(dsn, "?")
	options, err := url.ParseQuery(query)
	if err == nil && !options.Has("_time_format") {
		separator := "?"
		if hasQuery {
			separator = "&"
		}
		// modernc usa time.Time.String por padrão; os bancos do aplicativo usam
		// o formato SQLite. Alterá-lo muda ordenação textual e cursores antigos.
		dsn += separator + "_time_format=sqlite"
	}
	return &Dialector{ormsqlite.Dialector{DriverName: "sqlite", DSN: dsn}}
}

// O tradutor oficial espera o JSON de erros do driver CGO. Preserva os três
// códigos que o adaptador anterior traduzia quando TranslateError era ligado.
func (Dialector) Translate(err error) error {
	if sqliteErr, ok := err.(*driver.Error); ok {
		switch sqliteErr.Code() {
		case constants.SQLITE_CONSTRAINT_UNIQUE, constants.SQLITE_CONSTRAINT_PRIMARYKEY:
			return gorm.ErrDuplicatedKey
		case constants.SQLITE_CONSTRAINT_FOREIGNKEY:
			return gorm.ErrForeignKeyViolated
		}
	}
	return err
}
