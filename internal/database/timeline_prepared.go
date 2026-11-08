package database

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"gorm.io/gorm"
)

// TimelineStatements prepara somente as seis formas fixas da abertura: fonte
// limitada, contagem de filhos e página inicial/final, com/sem pai. Guarda planos, nunca resultados
// ou parâmetros. Os handles pertencem ao mesmo sql.DB e são liberados por Close
// desse banco; uma nova instância de banco exige um novo leitor.
type TimelineStatements struct {
	mu         sync.Mutex
	db         *gorm.DB
	statements map[string]*sql.Stmt
}

func NewTimelineStatements(db *gorm.DB) *TimelineStatements {
	return &TimelineStatements{db: db}
}

// Prepare acontece antes da transação: preparar pelo pool dentro dela poderia
// bloquear com MaxOpenConns=1 ou acessar outra conexão de um banco em memória.
// Falha fecha os handles parciais e não envenena as próximas requisições.
func (p *TimelineStatements) Prepare(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.statements != nil {
		return nil
	}
	db, err := p.db.DB()
	if err != nil {
		return err
	}
	queries := make([]string, 0, 6)
	var parent string
	repo := NewMessageRepository(p.db)
	for _, scope := range []*string{nil, &parent} {
		for _, order := range []string{timelineOrderAscending, timelineOrderDescending} {
			queries = append(queries, timelineIdentitySQL(repo.timelinePageSQL(scope, "", order), order))
		}
	}
	queries = append(queries, timelineBoundedSourceSQL, canonicalChildCountSQL)
	statements := make(map[string]*sql.Stmt, len(queries))
	for _, query := range queries {
		stmt, err := db.PrepareContext(ctx, query)
		if err != nil {
			for _, previous := range statements {
				_ = previous.Close()
			}
			return err
		}
		statements[query] = stmt
	}
	p.statements = statements
	return nil
}

// Bind, após Prepare bem-sucedido, mantém as leituras na transação autorizada.
// As demais consultas
// seguem diretamente para essa mesma transação, sem cache dinâmico de SQL.
func (p *TimelineStatements) Bind(tx *gorm.DB) error {
	transaction, ok := tx.Statement.ConnPool.(*sql.Tx)
	if !ok {
		return fmt.Errorf("timeline: transação SQL esperada, recebido %T", tx.Statement.ConnPool)
	}
	tx.Statement.ConnPool = &timelineStatementPool{Tx: transaction, statements: p.statements}
	return nil
}

type timelineStatementPool struct {
	*sql.Tx
	statements map[string]*sql.Stmt
}

func (p timelineStatementPool) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	if stmt := p.statements[query]; stmt != nil {
		return p.Tx.StmtContext(ctx, stmt).QueryContext(ctx, args...)
	}
	return p.Tx.QueryContext(ctx, query, args...)
}
