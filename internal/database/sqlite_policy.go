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
	"gorm.io/gorm"
)

const (
	sqliteBusyTimeout            = 100 * time.Millisecond
	sqliteMaintenanceBusyTimeout = 5 * time.Second
	sqliteMaxOpenConns           = 4
	sqliteMaxIdleConns           = 2
	sqliteBusyRetryMaxWait       = 4 * time.Second
)

// sqliteMaintenanceGate coordena operações destrutivas longas com a
// compactação física. Um canal, em vez de sync.Mutex, permite que quem espera
// respeite o cancelamento do contexto.
var sqliteMaintenanceGate = make(chan struct{}, 1)
var conversationLifecycleGate = make(chan struct{}, 1)

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
	q.Add("_pragma", "foreign_keys(1)")
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

func acquireSQLiteMaintenance(ctx context.Context) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case sqliteMaintenanceGate <- struct{}{}:
		return func() { <-sqliteMaintenanceGate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// WithSQLiteMaintenance serializa operações de manutenção/importação com
// exclusões destrutivas e VACUUM sem expor o gate global aos callers.
func WithSQLiteMaintenance(ctx context.Context, fn func() error) error {
	release, err := acquireSQLiteMaintenance(ctx)
	if err != nil {
		return err
	}
	defer release()
	return fn()
}

// WithConversationLifecycle serializa exclusão e restauração do mesmo espaço
// de IDs até os efeitos pós-commit terminarem.
func WithConversationLifecycle(ctx context.Context, fn func() error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case conversationLifecycleGate <- struct{}{}:
		defer func() { <-conversationLifecycleGate }()
		return fn()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func withSQLiteImmediateTransaction(ctx context.Context, db *gorm.DB, operation string, fn func(*gorm.DB) error) error {
	return WithSQLiteImmediateTransaction(ctx, db, operation, fn)
}

// WithSQLiteImmediateTransaction serializa a aquisição do writer lock antes
// de qualquer leitura de validação. É exportado para repositories de outros
// pacotes que persistem dados ligados ao histórico.
func WithSQLiteImmediateTransaction(ctx context.Context, db *gorm.DB, operation string, fn func(*gorm.DB) error) error {
	return WithSQLiteBusyRetry(ctx, operation, func() error {
		return db.WithContext(ctx).Connection(func(tx *gorm.DB) error {
			// BEGIN/COMMIT são controlados explicitamente abaixo; impedir que
			// callbacks de escrita do GORM abram uma transação aninhada.
			tx = tx.Session(&gorm.Session{SkipDefaultTransaction: true})
			if err := tx.Exec("BEGIN IMMEDIATE").Error; err != nil {
				return err
			}
			committed := false
			defer func() {
				if !committed {
					rollbackCtx := context.Background()
					if ctx != nil {
						rollbackCtx = context.WithoutCancel(ctx)
					}
					if err := tx.WithContext(rollbackCtx).Exec("ROLLBACK").Error; err != nil {
						logging.Errorf(rollbackCtx, "database.sqlite", "falha no rollback de %s: %v", operation, err)
					}
				}
			}()

			if err := fn(tx); err != nil {
				return err
			}
			if err := tx.Exec("COMMIT").Error; err != nil {
				return err
			}
			committed = true
			return nil
		})
	})
}

// WithSQLiteImmediateTransactionOnce retries only acquisition of the SQLite
// writer. Once acquired, fn and COMMIT are each attempted exactly once. The
// acquisition deadline bounds pool and writer waits, but fn receives ctx.
// When db is already transactional, GORM's nested Transaction creates a
// savepoint and no acquisition retry is attempted.
func WithSQLiteImmediateTransactionOnce(ctx context.Context, acquisitionDeadline time.Time, db *gorm.DB, operation string, fn func(*gorm.DB) error) error {
	if ctx == nil || db == nil || fn == nil {
		return fmt.Errorf("%s: invalid transaction arguments", operation)
	}
	connPool := db.ConnPool
	if db.Statement != nil && db.Statement.ConnPool != nil {
		connPool = db.Statement.ConnPool
	}
	if _, ok := connPool.(gorm.TxCommitter); ok {
		return db.WithContext(ctx).Transaction(fn)
	}
	acquireCtx := ctx
	var cancel context.CancelFunc
	if !acquisitionDeadline.IsZero() {
		if parentDeadline, ok := ctx.Deadline(); !ok || acquisitionDeadline.Before(parentDeadline) {
			acquireCtx, cancel = context.WithDeadline(ctx, acquisitionDeadline)
			defer cancel()
		}
	}
	rootSQLDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("%s: database root unavailable: %w", operation, err)
	}
	if rootSQLDB == nil {
		return fmt.Errorf("%s: database root unavailable", operation)
	}
	return db.WithContext(acquireCtx).Connection(func(connectionDB *gorm.DB) error {
		connectionDB = connectionDB.Session(&gorm.Session{SkipDefaultTransaction: true})
		connPool := &sqliteImmediateConnPool{ConnPool: connectionDB.Statement.ConnPool, root: rootSQLDB}
		connectionDB.Statement.ConnPool = connPool
		if err := WithSQLiteBusyRetry(acquireCtx, operation+".begin", func() error {
			return connectionDB.Exec("BEGIN IMMEDIATE").Error
		}); err != nil {
			return err
		}
		committed := false
		defer func() {
			if !committed {
				rollbackCtx := context.WithoutCancel(ctx)
				if err := connectionDB.WithContext(rollbackCtx).Exec("ROLLBACK").Error; err != nil {
					logging.Errorf(rollbackCtx, "database.sqlite", "falha no rollback de %s: %v", operation, err)
				}
			}
		}()
		if err := acquireCtx.Err(); err != nil {
			return err
		}
		if err := fn(connectionDB.WithContext(ctx)); err != nil {
			return err
		}
		if err := connectionDB.WithContext(ctx).Exec("COMMIT").Error; err != nil {
			return err
		}
		committed = true
		return nil
	})
}

// Marca a conexão com BEGIN IMMEDIATE como transacional para que chamadas
// gorm.Transaction aninhadas usem SAVEPOINT em vez de tentar outro BEGIN.
type sqliteImmediateConnPool struct {
	gorm.ConnPool
	root *sql.DB
}

func (pool *sqliteImmediateConnPool) GetDBConn() (*sql.DB, error) { return pool.root, nil }

func (sqliteImmediateConnPool) Commit() error {
	return errors.New("commit controlado por WithSQLiteImmediateTransactionOnce")
}

func (sqliteImmediateConnPool) Rollback() error {
	return errors.New("rollback controlado por WithSQLiteImmediateTransactionOnce")
}
