// Package commandbootstrap prepara somente armazenamento e chaves. Readiness
// de execução, handlers, sessões e adapters continuam responsabilidade do App.
package commandbootstrap

import (
	"context"
	"errors"
	"sort"
	"strings"

	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/commandinstance"
	"assistente/internal/commandjobactivation"
	"assistente/internal/commandjobevents"
	"assistente/internal/commandledger"
	"assistente/internal/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var ErrStorage = errors.New("armazenamento de comandos indisponível ou incompatível")

const keySchema = `CREATE TABLE IF NOT EXISTS command_key_versions (
 id TEXT NOT NULL PRIMARY KEY CHECK(length(id)=36 AND length(replace(id,'-',''))=32 AND id=lower(id) AND id NOT GLOB '*[^0-9a-f-]*' AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND substr(id,15,1)='7' AND substr(id,20,1) IN ('8','9','a','b')),
 version TEXT NOT NULL UNIQUE,
 digest TEXT NOT NULL CHECK(length(digest) = 64 AND digest NOT GLOB '*[^0-9a-f]*'),
 active INTEGER NOT NULL CHECK(active IN (0,1))
)`

func migrate(ctx context.Context, db *gorm.DB) error {
	for _, step := range []func(context.Context, *gorm.DB) error{commandconfig.Migrate, commanddecision.Migrate, commandledger.Migrate, commandactivation.Migrate, commandautomation.Migrate, commandjobactivation.Migrate} {
		if err := step(ctx, db); err != nil {
			return err
		}
	}
	if err := db.WithContext(ctx).AutoMigrate(commandjobevents.Models()...); err != nil {
		return err
	}
	if err := db.Exec(keySchema).Error; err != nil {
		return err
	}
	if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS ux_command_key_active ON command_key_versions(active) WHERE active = 1").Error; err != nil {
		return err
	}
	return commandinstance.Migrate(ctx, db)
}

type schemaObject struct{ Type, Name, TblName, SQL string }

func objects(db *gorm.DB) ([]schemaObject, error) {
	var result []schemaObject
	err := db.Raw("SELECT type, name, tbl_name, sql FROM sqlite_master WHERE sql IS NOT NULL AND (name GLOB 'command_*' OR tbl_name GLOB 'command_*') ORDER BY name").Scan(&result).Error
	return result, err
}

// Migrate adota o schema experimental conhecido ou cria o novo; um schema
// desconhecido é recusado ANTES de AutoMigrate poder reescrever tabelas.
// A referência é construída em SQLite privado em memória usando os próprios
// repositories, sem copiar DDL e sem acessar arquivos ou banco global.
func Migrate(ctx context.Context, db *gorm.DB) error {
	if ctx == nil || db == nil {
		return ErrStorage
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	reference, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return ErrStorage
	}
	sqlDB, err := reference.DB()
	if err != nil {
		return ErrStorage
	}
	defer func() { _ = sqlDB.Close() }()
	sqlDB.SetMaxOpenConns(1)
	if err := migrate(ctx, reference.WithContext(ctx)); err != nil {
		return ErrStorage
	}
	expected, err := objects(reference)
	if err != nil {
		return ErrStorage
	}
	want := make(map[string]schemaObject, len(expected))
	for _, obj := range expected {
		want[obj.Name] = obj
	}
	check := func(tx *gorm.DB, complete bool) error {
		actual, err := objects(tx)
		if err != nil {
			return err
		}
		if complete && len(actual) != len(expected) {
			return ErrStorage
		}
		for _, obj := range actual {
			w, ok := want[obj.Name]
			if !ok || w.Type != obj.Type || w.TblName != obj.TblName || (normalizeDDL(w) != normalizeDDL(obj) && (complete || (normalizeDDL(legacyEnvelopeObject(w)) != normalizeDDL(obj) && normalizeDDL(legacyExternalContextObject(w)) != normalizeDDL(obj) && normalizeDDL(legacyConfigObject(w)) != normalizeDDL(obj) && normalizeDDL(legacyRulesObject(w)) != normalizeDDL(obj) && normalizeDDL(legacyImportObject(w)) != normalizeDDL(obj) && normalizeDDL(legacyEnvelopeObject(legacyConfigObject(w))) != normalizeDDL(obj)))) {
				return ErrStorage
			}
		}
		return nil
	}
	apply := func(tx *gorm.DB) error {
		if err := check(tx, false); err != nil {
			return err
		}
		if err := upgradeConfigObjects(tx, want); err != nil {
			return err
		}
		if err := upgradeEnvelopeObjects(tx, want); err != nil {
			return err
		}
		if err := upgradeDecisionObjects(tx, want); err != nil {
			return err
		}
		if err := migrate(ctx, tx); err != nil {
			return err
		}
		return check(tx, true)
	}
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := database.ApplyCommandStorageMigration(ctx, tx, apply); err != nil {
			return err
		}
		if err := database.ApplyCommandEnvelopeMigration(ctx, tx, apply); err != nil {
			return err
		}
		if err := database.ApplyCommandConfigMigration(ctx, tx, apply); err != nil {
			return err
		}
		if err := database.ApplyCommandActivationMigration(ctx, tx, apply); err != nil {
			return err
		}
		if err := database.ApplyCommandJobActivationMigration(ctx, tx, apply); err != nil {
			return err
		}
		if err := database.ApplyCommandImportMigration(ctx, tx, apply); err != nil {
			return err
		}
		if err := database.ApplyCommandInstanceMigration(ctx, tx, apply); err != nil {
			return err
		}
		return database.ApplyCommandDecisionExternalContextMigration(ctx, tx, apply)
	})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrStorage
	}
	// Mesmo com carimbo, drift posterior não publica readiness.
	if err := check(db.WithContext(ctx), true); err != nil {
		return ErrStorage
	}
	return nil
}

// GORM pode emitir constraints nomeadas em ordens diferentes. A ordem das
// definições de colunas/constraints de CREATE TABLE não muda o contrato destas
// tabelas (queries nomeiam colunas). Nunca ordenar expressões de índice/CHECK.
func normalizeDDL(obj schemaObject) string {
	sql := strings.TrimSpace(obj.SQL)
	if obj.Type != "table" {
		return sql
	}
	start, end := strings.IndexByte(sql, '('), strings.LastIndexByte(sql, ')')
	if start < 0 || end < start {
		return sql
	}
	body := sql[start+1 : end]
	parts := []string{}
	depth, from := 0, 0
	var quote byte
	for i := 0; i < len(body); i++ {
		c := body[i]
		if quote != 0 {
			if c == quote {
				if i+1 < len(body) && body[i+1] == quote {
					i++
				} else {
					quote = 0
				}
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
		case '[':
			quote = ']'
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(body[from:i]))
				from = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(body[from:]))
	sort.Strings(parts)
	return strings.TrimSpace(sql[:start]) + "(" + strings.Join(parts, ",") + ")" + strings.TrimSpace(sql[end+1:])
}
