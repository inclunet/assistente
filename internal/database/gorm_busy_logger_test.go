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
