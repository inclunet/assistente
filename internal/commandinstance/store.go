package commandinstance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProcessGenerationSchema é o schema aditivo usado pela migração v29 e pelo
// bootstrap composto. Open nunca o cria implicitamente.
const ProcessGenerationSchema = `CREATE TABLE IF NOT EXISTS command_process_generations (
	startup_id TEXT NOT NULL PRIMARY KEY CHECK(length(startup_id)=36 AND length(replace(startup_id,'-',''))=32 AND startup_id=lower(startup_id) AND startup_id NOT GLOB '*[^0-9a-f-]*' AND substr(startup_id,9,1)='-' AND substr(startup_id,14,1)='-' AND substr(startup_id,19,1)='-' AND substr(startup_id,24,1)='-' AND substr(startup_id,15,1)='7' AND substr(startup_id,20,1) IN ('8','9','a','b')),
	file_identity TEXT NOT NULL CHECK(length(file_identity) BETWEEN 1 AND 512 AND trim(file_identity)=file_identity AND instr(file_identity, char(0))=0),
	created_at DATETIME NOT NULL
)`

var (
	ErrSchema = errors.New("schema de gerações de processo ausente ou incompatível")
)

// Migrate cria somente o schema de ownership persistido. Open não chama esta
// função: no produto, a criação/carimbo acontece no bootstrap versionado.
func Migrate(ctx context.Context, db *gorm.DB) error {
	if ctx == nil || db == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Exec(ProcessGenerationSchema).Error
	})
}

type processGenerationRow struct {
	StartupID    string `gorm:"column:startup_id"`
	FileIdentity string `gorm:"column:file_identity"`
	CreatedAt    string `gorm:"column:created_at"`
}

type databaseListRow struct {
	Name string `gorm:"column:name"`
	File string `gorm:"column:file"`
}

type proofState struct {
	mu       sync.RWMutex
	valid    bool
	prefixes map[string]struct{}
	sqlRoot  *sql.DB
}

// RecoveryProof é deliberadamente opaca: só Lease.RecoveryProof pode criar
// uma prova que contém os prefixos de startups do mesmo arquivo físico.
type RecoveryProof struct{ state *proofState }

func (p RecoveryProof) Valid() bool {
	if p.state == nil {
		return false
	}
	p.state.mu.RLock()
	defer p.state.mu.RUnlock()
	return p.state.valid
}

func (p RecoveryProof) Includes(generation string) bool {
	if !validGeneration(generation) || p.state == nil {
		return false
	}
	prefix := generation[:strings.IndexByte(generation, ':')]
	p.state.mu.RLock()
	defer p.state.mu.RUnlock()
	if !p.state.valid {
		return false
	}
	_, ok := p.state.prefixes[prefix]
	return ok
}

func (p RecoveryProof) UsesDatabase(db *gorm.DB) bool {
	if p.state == nil || db == nil {
		return false
	}
	root, ok := databaseRoot(db)
	if !ok {
		return false
	}
	p.state.mu.RLock()
	defer p.state.mu.RUnlock()
	return p.state.valid && p.state.sqlRoot == root
}

type Lease struct {
	mu           sync.Mutex
	db           *gorm.DB
	lock         *Lock
	startupID    string
	fileIdentity string
	proof        *proofState
	sqlRoot      *sql.DB
	closed       bool
}

func Open(ctx context.Context, db *gorm.DB, startupID string) (*Lease, error) {
	if ctx == nil || db == nil || !canonicalUUIDv7(startupID) {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sqlRoot, ok := databaseRoot(db)
	if !ok {
		return nil, ErrInvalid
	}
	path, pathInfo, err := mainDatabasePath(ctx, db)
	if err != nil {
		return nil, err
	}
	if err := requireProcessGenerationSchema(ctx, db); err != nil {
		return nil, err
	}

	physicalLock, err := Acquire(ctx, path)
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = physicalLock.Close()
		}
	}()
	lockedInfo, err := physicalLock.file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspecionar arquivo bloqueado: %w", err)
	}
	currentInfo, err := os.Stat(path)
	if err != nil || !os.SameFile(pathInfo, currentInfo) || !os.SameFile(lockedInfo, currentInfo) {
		if err == nil {
			err = errors.New("arquivo principal mudou durante a aquisição do lock")
		}
		return nil, fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	fileIdentity := strings.TrimSpace(physicalLock.Identity())
	if fileIdentity == "" {
		return nil, fmt.Errorf("%w: identidade física vazia", ErrInvalid)
	}

	var prefixes []string
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := tx.Exec(
			`INSERT INTO command_process_generations (startup_id, file_identity, created_at) VALUES (?, ?, CURRENT_TIMESTAMP)`,
			startupID, fileIdentity,
		).Error; err != nil {
			return err
		}
		var rows []processGenerationRow
		if err := tx.Raw(
			`SELECT startup_id, file_identity, created_at FROM command_process_generations WHERE file_identity = ? AND startup_id <> ? ORDER BY startup_id`,
			fileIdentity, startupID,
		).Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if !canonicalUUIDv7(row.StartupID) || row.FileIdentity != fileIdentity {
				return fmt.Errorf("%w: registro de startup inválido", ErrSchema)
			}
			prefixes = append(prefixes, row.StartupID)
		}
		return ctx.Err()
	})
	if err != nil {
		return nil, err
	}

	state := &proofState{valid: true, prefixes: make(map[string]struct{}, len(prefixes)), sqlRoot: sqlRoot}
	for _, prefix := range prefixes {
		state.prefixes[prefix] = struct{}{}
	}
	keep = true
	return &Lease{db: db, lock: physicalLock, startupID: startupID, fileIdentity: fileIdentity, proof: state, sqlRoot: sqlRoot}, nil
}

func (l *Lease) UsesDatabase(db *gorm.DB) bool {
	if l == nil || db == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	root, ok := databaseRoot(db)
	return !l.closed && ok && l.sqlRoot == root
}

func (l *Lease) RecoveryProof() RecoveryProof {
	if l == nil {
		return RecoveryProof{}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return RecoveryProof{}
	}
	return RecoveryProof{state: l.proof}
}

func (l *Lease) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	if l.proof != nil {
		l.proof.mu.Lock()
		l.proof.valid = false
		l.proof.mu.Unlock()
	}
	lock := l.lock
	err := lock.Close()
	if err == nil {
		l.closed = true
	}
	l.mu.Unlock()
	return err
}

func mainDatabasePath(ctx context.Context, db *gorm.DB) (string, os.FileInfo, error) {
	var rows []databaseListRow
	if err := db.WithContext(ctx).Raw("PRAGMA database_list").Scan(&rows).Error; err != nil {
		return "", nil, fmt.Errorf("descobrir arquivo principal: %w", err)
	}
	for _, row := range rows {
		if row.Name != "main" {
			continue
		}
		path := row.File
		if path == "" || strings.TrimSpace(path) == "" {
			return "", nil, ErrUnsupported
		}
		info, err := os.Stat(path)
		if err != nil {
			return "", nil, fmt.Errorf("arquivo principal indisponível: %w", err)
		}
		if !info.Mode().IsRegular() {
			return "", nil, ErrUnsupported
		}
		return path, info, nil
	}
	return "", nil, ErrUnsupported
}

func requireProcessGenerationSchema(ctx context.Context, db *gorm.DB) error {
	var kind string
	if err := db.WithContext(ctx).Raw(`SELECT type FROM sqlite_master WHERE name = 'command_process_generations'`).Scan(&kind).Error; err != nil {
		return fmt.Errorf("verificar schema de gerações: %w", err)
	}
	if kind != "table" {
		return ErrSchema
	}
	var columns []struct {
		Name    string `gorm:"column:name"`
		Type    string `gorm:"column:type"`
		NotNull int    `gorm:"column:notnull"`
		PK      int    `gorm:"column:pk"`
	}
	if err := db.WithContext(ctx).Raw(`SELECT name, type, "notnull", pk FROM pragma_table_info('command_process_generations')`).Scan(&columns).Error; err != nil {
		return fmt.Errorf("ler schema de gerações: %w", err)
	}
	want := map[string]bool{"startup_id": false, "file_identity": false, "created_at": false}
	if len(columns) != len(want) {
		return ErrSchema
	}
	for _, column := range columns {
		switch column.Name {
		case "startup_id":
			want[column.Name] = column.Type == "TEXT" && column.NotNull == 1 && column.PK == 1
		case "file_identity":
			want[column.Name] = column.Type == "TEXT" && column.NotNull == 1 && column.PK == 0
		case "created_at":
			want[column.Name] = column.Type == "DATETIME" && column.NotNull == 1 && column.PK == 0
		}
	}
	for name, present := range want {
		if !present {
			return fmt.Errorf("%w: coluna %s ausente", ErrSchema, name)
		}
	}
	return nil
}

func canonicalUUIDv7(value string) bool {
	u, err := uuid.Parse(value)
	return err == nil && u.Version() == 7 && u.Variant() == uuid.RFC4122 && u.String() == value
}

func databaseRoot(db *gorm.DB) (*sql.DB, bool) {
	if db == nil || db.Statement == nil || db.Statement.ConnPool == nil {
		return nil, false
	}
	if _, transactional := db.Statement.ConnPool.(gorm.TxCommitter); transactional {
		return nil, false
	}
	root, err := db.DB()
	return root, err == nil && root != nil
}

func validGeneration(value string) bool {
	separator := strings.IndexByte(value, ':')
	if separator != 36 || strings.Count(value, ":") != 1 || !canonicalUUIDv7(value[:separator]) {
		return false
	}
	counter := value[separator+1:]
	if counter == "" || (len(counter) > 1 && counter[0] == '0') {
		return false
	}
	for _, char := range counter {
		if char < '0' || char > '9' {
			return false
		}
	}
	_, err := strconv.ParseUint(counter, 10, 64)
	return err == nil
}
