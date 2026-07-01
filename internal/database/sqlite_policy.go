package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"assistente/internal/logging"
)

const (
	sqliteBusyTimeout            = 100 * time.Millisecond
	sqliteMaintenanceBusyTimeout = 5 * time.Second
	sqliteMaxOpenConns           = 4
	sqliteMaxIdleConns           = 2
	sqliteBusyRetryMaxWait       = 4 * time.Second
)

var sqliteBusyRetryDelays = []time.Duration{
	25 * time.Millisecond,
	50 * time.Millisecond,
	100 * time.Millisecond,
	200 * time.Millisecond,
	375 * time.Millisecond,
}

// sqliteDSN configura pragmas por conexão. O busy_timeout nativo é curto para
// devolver SQLITE_BUSY rapidamente; WithSQLiteBusyRetry controla a espera total
// com backoff e logging centralizados.
func sqliteDSN(path string) string {
	normalized := filepath.ToSlash(path)
	if !strings.HasPrefix(normalized, "/") {
		normalized = "/" + normalized
	}
	u := url.URL{Scheme: "file", Path: normalized}
	q := url.Values{}
	q.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", sqliteBusyTimeout.Milliseconds()))
	u.RawQuery = q.Encode()
	return u.String()
}

// configureSQLitePool mantém algumas conexões para leitores em WAL sem abrir
// concorrência excessiva de writers num app desktop local.
func configureSQLitePool(sqlDB *sql.DB) {
	sqlDB.SetMaxOpenConns(sqliteMaxOpenConns)
	sqlDB.SetMaxIdleConns(sqliteMaxIdleConns)
}

func IsSQLiteBusyError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "sqlite_busy") ||
		strings.Contains(msg, "sqlite_locked") ||
		strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "database table is locked") ||
		strings.Contains(msg, "database is busy") ||
		strings.Contains(msg, "database schema is locked")
}

func WithSQLiteBusyRetry(ctx context.Context, operation string, fn func() error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	started := time.Now()
	var lastErr error
	for attempt := 0; attempt <= len(sqliteBusyRetryDelays); attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := fn()
		if !IsSQLiteBusyError(err) {
			return err
		}
		lastErr = err
		if attempt == len(sqliteBusyRetryDelays) || time.Since(started) >= sqliteBusyRetryMaxWait {
			logging.Warnf(ctx, "database.sqlite", "[Database] SQLite lock persistente em %s após %d tentativa(s): %v", operation, attempt+1, err)
			return err
		}
		delay := sqliteBusyRetryDelays[attempt]
		logging.Debugf(ctx, "database.sqlite", "[Database] SQLite lock transitório em %s; retry %d em %s", operation, attempt+1, delay)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return lastErr
}
