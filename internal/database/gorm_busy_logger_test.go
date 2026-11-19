package database

import (
	"bytes"
	"context"
	"errors"
	"io"
	stdlog "log"
	"strings"
	"testing"
	"time"

	"assistente/internal/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newBusyQuietLogger(out io.Writer) busyQuietLogger {
	base := logger.New(
		stdlog.New(out, "", 0),
		logger.Config{
			SlowThreshold: time.Second,
			LogLevel:      logger.Warn,
			Colorful:      false,
		},
	)
	return busyQuietLogger{Interface: base}
}

// TestBusyQuietLogger_SuprimeBusy garante que o erro de SQLITE_BUSY (tratado por
// WithSQLiteBusyRetry) não é repassado ao logger do GORM, enquanto erros reais
// continuam sendo logados.
func TestBusyQuietLogger_SuprimeBusy(t *testing.T) {
	var buf bytes.Buffer
	l := newBusyQuietLogger(&buf)
	fc := func() (string, int64) { return "UPDATE `task_notes` SET x=1", 1 }

	// Busy transitório e rápido: nada deve ser logado.
	l.Trace(context.Background(), time.Now(), fc, errors.New("database is locked (5) (SQLITE_BUSY)"))
	if buf.Len() != 0 {
		t.Fatalf("erro de SQLITE_BUSY não deveria ser logado; log=%q", buf.String())
	}

	// Erro real: deve aparecer.
	l.Trace(context.Background(), time.Now(), fc, errors.New("no such table: foo"))
	if !strings.Contains(buf.String(), "no such table") {
		t.Fatalf("erro real deveria ser logado; log=%q", buf.String())
	}
}

// TestBusyQuietLogger_LogModeMantemWrapper garante que trocar o nível de log
// (db.Debug()/Session) preserva a supressão de busy.
func TestBusyQuietLogger_LogModeMantemWrapper(t *testing.T) {
	l := newBusyQuietLogger(io.Discard)
	if _, ok := l.LogMode(logger.Info).(busyQuietLogger); !ok {
		t.Fatal("LogMode deveria devolver busyQuietLogger para manter a supressão")
	}
}

func TestConfiguredLoggerPreservesErrorsWithoutLoggingExpectedAbsence(t *testing.T) {
	var output bytes.Buffer
	previousOutput, previousLevel := gormLogOutput, gormLogLevel
	gormLogOutput, gormLogLevel = &output, logger.Warn
	defer func() { gormLogOutput, gormLogLevel = previousOutput, previousLevel }()
	log := configuredGORMLogger()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: log})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			t.Error(err)
		}
	}()
	type pollingRow struct{ ID string }
	if err := db.AutoMigrate(&pollingRow{}); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	for i := 0; i < 1000; i++ {
		var row pollingRow
		if err := db.First(&row).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("ausência deixou de retornar ErrRecordNotFound: %v", err)
		}
	}
	if strings.Contains(output.String(), "record not found") {
		t.Fatalf("ausência esperada foi registrada como erro: %s", output.String())
	}
	output.Reset()
	// Expected absence must still be diagnosed if the SQL itself is slow.
	log.Trace(context.Background(), time.Now().Add(-time.Second), func() (string, int64) { return "SELECT id FROM polling_rows", 0 }, gorm.ErrRecordNotFound)
	if !strings.Contains(output.String(), "SLOW SQL") {
		t.Fatal("consulta lenta sem resultado foi omitida")
	}
	output.Reset()
	if err := db.Exec("SELECT id FROM missing_polling_table").Error; err == nil {
		t.Fatal("erro SQL real foi perdido")
	}
	if !strings.Contains(output.String(), "missing_polling_table") {
		t.Fatal("erro SQL real foi omitido do log")
	}
}
