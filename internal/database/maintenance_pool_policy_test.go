package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"

	"assistente/internal/sqlite"
	"gorm.io/gorm"
	modernc "modernc.org/sqlite"
)

func TestCompactRestoresConnectionBusyTimeout(t *testing.T) {
	for _, mode := range []string{"full", "incremental", "skipped", "readonly-error"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "pool-policy.db")
			dsn := SQLiteDSN(path)
			gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			pool, err := gdb.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = pool.Close() })
			pool.SetMaxOpenConns(1)
			pool.SetMaxIdleConns(1)
			if mode == "incremental" {
				if err := gdb.Exec("PRAGMA auto_vacuum=INCREMENTAL").Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := gdb.Exec("CREATE TABLE evidence (value TEXT)").Error; err != nil {
				t.Fatal(err)
			}
			if err := gdb.Exec("INSERT INTO evidence VALUES ('preserved')").Error; err != nil {
				t.Fatal(err)
			}
			if mode == "readonly-error" {
				if err := pool.Close(); err != nil {
					t.Fatal(err)
				}
				gdb, err = gorm.Open(sqlite.Open(dsn+"&mode=ro"), &gorm.Config{})
				if err != nil {
					t.Fatal(err)
				}
				pool, err = gdb.DB()
				if err != nil {
					t.Fatal(err)
				}
				pool.SetMaxOpenConns(1)
				pool.SetMaxIdleConns(1)
			}
			previousDB, previousPath := db, dbPath
			db, dbPath = gdb, path
			t.Cleanup(func() { db, dbPath = previousDB, previousPath })
			// Valor não padrão prova restauração do estado da conexão, sem hardcode.
			const originalTimeout = 37
			if err := gdb.Exec("PRAGMA busy_timeout=37").Error; err != nil {
				t.Fatal(err)
			}
			force := mode == "full" || mode == "readonly-error"
			minFree := int64(0)
			if mode == "skipped" {
				minFree = 1 << 30
			}
			result, err := Compact(ctx, force, minFree)
			if mode == "readonly-error" {
				if err == nil {
					t.Fatal("read-only VACUUM should fail")
				}
			} else if err != nil || result.Mode != mode {
				t.Fatalf("compact mode=%s err=%v", result.Mode, err)
			}
			var timeout int
			if err := gdb.Raw("PRAGMA busy_timeout").Scan(&timeout).Error; err != nil {
				t.Fatal(err)
			}
			if timeout != originalTimeout {
				t.Fatalf("pooled busy_timeout=%d want %d", timeout, originalTimeout)
			}
			var value string
			if err := gdb.Raw("SELECT value FROM evidence").Scan(&value).Error; err != nil || value != "preserved" {
				t.Fatalf("data=%q err=%v", value, err)
			}
		})
	}
}

// O driver real executa o SQL; o invólucro só injeta falhas nas duas operações
// indicadas, sem hook ou dependência de teste no código produtivo.
type maintenanceSQLConn interface {
	driver.Conn
	driver.ExecerContext
	driver.QueryerContext
	driver.SessionResetter
	driver.Validator
}

type maintenanceFaultConnector struct {
	dsn          string
	armed        atomic.Bool
	opened       atomic.Int32
	closed       atomic.Int32
	restored     atomic.Int32
	cancel       context.CancelFunc
	operationErr error
	restoreErr   error
}

func (c *maintenanceFaultConnector) Driver() driver.Driver { return &modernc.Driver{} }
func (c *maintenanceFaultConnector) Connect(ctx context.Context) (driver.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := c.Driver().Open(c.dsn)
	if err != nil {
		return nil, err
	}
	c.opened.Add(1)
	return &maintenanceFaultConn{maintenanceSQLConn: conn.(maintenanceSQLConn), control: c}, nil
}

type maintenanceFaultConn struct {
	maintenanceSQLConn
	control *maintenanceFaultConnector
}

func (c *maintenanceFaultConn) Close() error {
	c.control.closed.Add(1)
	return c.maintenanceSQLConn.Close()
}
func (c *maintenanceFaultConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if c.control.armed.Load() {
		if query == "PRAGMA busy_timeout=37" {
			c.control.restored.Add(1)
			if c.control.restoreErr != nil {
				return nil, c.control.restoreErr
			}
		}
		if query == "VACUUM" && c.control.operationErr != nil {
			return nil, c.control.operationErr
		}
	}
	result, err := c.maintenanceSQLConn.ExecContext(ctx, query, args)
	if err == nil && c.control.armed.Load() && query == "PRAGMA busy_timeout=5000" && c.control.cancel != nil {
		c.control.cancel()
	}
	return result, err
}

func TestCompactCleanupCancellationAndFailure(t *testing.T) {
	for _, mode := range []string{"cancel", "restore-error", "operation-and-restore-error"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			path := filepath.Join(t.TempDir(), "cleanup.db")
			control := &maintenanceFaultConnector{dsn: SQLiteDSN(path)}
			pool := sql.OpenDB(control)
			t.Cleanup(func() { _ = pool.Close() })
			pool.SetMaxOpenConns(1)
			pool.SetMaxIdleConns(1)
			dialector := sqlite.Open(control.dsn).(*sqlite.Dialector)
			dialector.Conn = pool
			gdb, err := gorm.Open(dialector, &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			for _, statement := range []string{"CREATE TABLE evidence (value TEXT)", "INSERT INTO evidence VALUES ('preserved')", "PRAGMA busy_timeout=37"} {
				if err := gdb.Exec(statement).Error; err != nil {
					t.Fatal(err)
				}
			}
			previousDB, previousPath := db, dbPath
			db, dbPath = gdb, path
			t.Cleanup(func() { db, dbPath = previousDB, previousPath })
			if mode == "cancel" {
				control.cancel = cancel
			} else {
				control.restoreErr = errors.New("injected timeout restoration failure")
				if mode == "operation-and-restore-error" {
					control.operationErr = errors.New("injected vacuum failure")
				}
			}
			control.armed.Store(true)
			_, err = Compact(ctx, true, 0)
			if mode == "cancel" {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancel error=%v", err)
				}
			} else {
				if !errors.Is(err, control.restoreErr) {
					t.Fatalf("lost cleanup error: %v", err)
				}
				if control.operationErr != nil && !errors.Is(err, control.operationErr) {
					t.Fatalf("lost operation error: %v", err)
				}
			}
			if control.restored.Load() != 1 {
				t.Fatalf("cleanup calls=%d", control.restored.Load())
			}
			var timeout int
			if err := gdb.Raw("PRAGMA busy_timeout").Scan(&timeout).Error; err != nil {
				t.Fatal(err)
			}
			if mode == "cancel" {
				if timeout != 37 || control.opened.Load() != 1 || control.closed.Load() != 0 {
					t.Fatalf("cancel cleanup: timeout=%d opened=%d closed=%d", timeout, control.opened.Load(), control.closed.Load())
				}
			} else if timeout != int(sqliteBusyTimeout.Milliseconds()) || control.opened.Load() != 2 || control.closed.Load() != 1 {
				t.Fatalf("connection not replaced: timeout=%d opened=%d closed=%d", timeout, control.opened.Load(), control.closed.Load())
			}
			var value string
			if err := gdb.Raw("SELECT value FROM evidence").Scan(&value).Error; err != nil || value != "preserved" {
				t.Fatalf("data=%q err=%v", value, err)
			}
		})
	}
}
