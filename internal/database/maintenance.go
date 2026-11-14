package database

import (
	"assistente/internal/logging"
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"time"
)

// defaultVacuumMinFreeBytes é o limiar mínimo de espaço livre (freelist) usado
// quando o chamador passa um valor inválido (< 0). Um valor 0 é literal
// ("sempre compactar"). A política real vem do config.json (AEP-0074); este
// pacote não lê configuração diretamente.
const defaultVacuumMinFreeBytes int64 = 16 * 1024 * 1024 // 16 MiB

// Limita o tempo de posse do writer por passagem; a próxima retoma a freelist.
const incrementalVacuumBatchPages = 1024

// CompactionBusyRetry limita a frequência de tentativas enquanto há leitor no WAL.
const CompactionBusyRetry = 30 * time.Second

// Modos de auto_vacuum do SQLite (PRAGMA auto_vacuum).
const (
	autoVacuumNone        = 0
	autoVacuumFull        = 1
	autoVacuumIncremental = 2
)

// DatabaseStats resume o estado físico do arquivo SQLite.
type DatabaseStats struct {
	Path           string `json:"path"`
	FileSizeBytes  int64  `json:"fileSizeBytes"`  // arquivo principal
	WALSizeBytes   int64  `json:"walSizeBytes"`   // arquivo -wal (0 se ausente)
	TotalSizeBytes int64  `json:"totalSizeBytes"` // principal + -wal + -shm
	PageSize       int64  `json:"pageSize"`
	PageCount      int64  `json:"pageCount"`
	FreelistCount  int64  `json:"freelistCount"`
	FreeBytes      int64  `json:"freeBytes"`      // freelistCount * pageSize
	AutoVacuumMode string `json:"autoVacuumMode"` // none | full | incremental
}

// CompactionResult descreve o que a manutenção física fez.
type CompactionResult struct {
	checkpointBusy  bool
	more            bool
	Mode            string `json:"mode"` // noop | incremental | full | skipped
	WALCheckpointed bool   `json:"walCheckpointed"`
	FreeBytesBefore int64  `json:"freeBytesBefore"`
	TotalSizeBefore int64  `json:"totalSizeBefore"`
	TotalSizeAfter  int64  `json:"totalSizeAfter"`
	ReclaimedBytes  int64  `json:"reclaimedBytes"`
}

// HasMore indica que a freelist ou o checkpoint final exigem outra passagem.
func (r CompactionResult) HasMore() bool { return r.more }

// CheckpointBusy indica que um leitor adiou o truncamento do WAL.
func (r CompactionResult) CheckpointBusy() bool { return r.checkpointBusy }

// CoordinatorCompactor adapta a compactação global SQLite à porta do
// InstanceMaintenanceCoordinator. Não cria loop nem lê configurações; o
// coordenador fornece o limiar vigente.
type CoordinatorCompactor struct{}

func (CoordinatorCompactor) Compact(ctx context.Context, minFreeBytes int64) error {
	_, err := Compact(ctx, false, minFreeBytes)
	return err
}

// CompactBatch permite ao coordenador retomar a compactação incremental.
func (CoordinatorCompactor) CompactBatch(ctx context.Context, minFreeBytes int64) (bool, time.Duration, error) {
	result, err := Compact(ctx, false, minFreeBytes)
	var retryAfter time.Duration
	if result.CheckpointBusy() {
		retryAfter = CompactionBusyRetry
	}
	return result.HasMore(), retryAfter, err
}

func autoVacuumModeName(mode int64) string {
	switch mode {
	case autoVacuumFull:
		return "full"
	case autoVacuumIncremental:
		return "incremental"
	default:
		return "none"
	}
}

// fileSize retorna o tamanho de um arquivo, ou 0 se não existir. A ausência é
// esperada (ex.: os arquivos -wal/-shm podem não existir); outros erros
// (permissão, IO, path inválido) são anômalos e logados para não mascarar
// estatísticas incorretas na UI/telemetria.
func fileSize(path string) int64 {
	if path == "" {
		return 0
	}
	info, err := os.Stat(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			logging.Errorf(context.Background(), "database.maintenance", "[Database] fileSize(%s): %v", path, err)
		}
		return 0
	}
	return info.Size()
}

// DatabaseStatsSnapshot lê o estado físico atual do banco (somente leitura).
func DatabaseStatsSnapshot(ctx context.Context) (DatabaseStats, error) {
	stats := DatabaseStats{Path: dbPath}
	if db == nil {
		return stats, errors.New("banco de dados não inicializado")
	}

	readInt := func(pragma string) int64 {
		var v int64
		if err := db.WithContext(ctx).Raw("PRAGMA " + pragma).Scan(&v).Error; err != nil {
			logging.Infof(ctx, "database.maintenance", "[Database] manutenção: leitura de PRAGMA %s falhou: %v", pragma, err)
		}
		return v
	}

	stats.PageSize = readInt("page_size")
	stats.PageCount = readInt("page_count")
	stats.FreelistCount = readInt("freelist_count")
	stats.FreeBytes = stats.FreelistCount * stats.PageSize
	stats.AutoVacuumMode = autoVacuumModeName(readInt("auto_vacuum"))

	stats.FileSizeBytes = fileSize(dbPath)
	stats.WALSizeBytes = fileSize(dbPath + "-wal")
	stats.TotalSizeBytes = stats.FileSizeBytes + stats.WALSizeBytes + fileSize(dbPath+"-shm")

	return stats, nil
}

// SQLiteFreelistCount lê apenas a freelist, sem checkpoint ou alteração física.
func SQLiteFreelistCount(ctx context.Context) (int64, error) {
	if db == nil {
		return 0, errors.New("banco de dados não inicializado")
	}
	var pages int64
	if err := db.WithContext(ctx).Raw("PRAGMA freelist_count").Scan(&pages).Error; err != nil {
		return 0, err
	}
	return pages, nil
}

// Compact executa a manutenção física do arquivo SQLite (AEP-0074):
//
//   - checkpoint PASSIVE por lote incremental e TRUNCATE final sem espera;
//   - em bancos no modo incremental: PRAGMA incremental_vacuum limitado, salvo
//     quando force=true, que executa VACUUM completo;
//   - em bancos legados (auto_vacuum=none/full): VACUUM completo, gated por
//     limiar de páginas livres (a menos que force=true). O VACUUM também
//     converte o banco para o modo incremental dali em diante.
//
// minFreeBytes é o limiar de freelist (em bytes) para disparar o VACUUM completo
// em bancos legados; vem do config.json (AEP-0074). Valor 0 é literal (sempre
// compacta); valor negativo (inválido) usa o default.
//
// É best-effort: erros são retornados, mas o chamador (retenção/boot) deve
// apenas logá-los sem abortar. Toda a operação usa UMA conexão dedicada para
// garantir que o PRAGMA auto_vacuum e o VACUUM rodem na mesma conexão, e é
// serializada globalmente pelo gate compartilhado com exclusões em lote.
func Compact(ctx context.Context, force bool, minFreeBytes int64) (CompactionResult, error) {
	res := CompactionResult{Mode: "noop"}
	if db == nil {
		return res, errors.New("banco de dados não inicializado")
	}
	if minFreeBytes < 0 {
		minFreeBytes = defaultVacuumMinFreeBytes
	}

	releaseMaintenance, err := acquireSQLiteMaintenance(ctx)
	if err != nil {
		return res, err
	}
	defer releaseMaintenance()

	before, _ := DatabaseStatsSnapshot(ctx)
	res.FreeBytesBefore = before.FreeBytes
	res.TotalSizeBefore = before.TotalSizeBytes

	sqlDB, err := db.DB()
	if err != nil {
		return res, err
	}

	// Toda a manutenção física roda em UMA conexão dedicada para garantir que o
	// PRAGMA auto_vacuum e o VACUUM (conversão de modo) ocorram juntos. A conexão
	// é liberada ANTES do snapshot final: do contrário, com pool de 1 conexão,
	// a leitura de estatísticas ficaria bloqueada esperando a própria conexão.
	if err := func() (runErr error) {
		conn, err := sqlDB.Conn(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = conn.Close() }()

		var previousBusyTimeout int
		if err := conn.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&previousBusyTimeout); err != nil {
			return fmt.Errorf("read maintenance busy_timeout: %w", err)
		}
		defer func() {
			// A conexão volta ao pool também em erro/cancelamento. Não propagar
			// a espera longa da manutenção às próximas operações do aplicativo.
			if _, err := conn.ExecContext(context.WithoutCancel(ctx), fmt.Sprintf("PRAGMA busy_timeout=%d", previousBusyTimeout)); err != nil {
				// Uma conexão cuja política não foi restaurada não pode ser reusada.
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
				runErr = errors.Join(runErr, fmt.Errorf("restore maintenance busy_timeout: %w", err))
			}
		}()

		// busy_timeout é por conexão: o pragma de Init() roda em OUTRA conexão do
		// pool, então é preciso reaplicá-lo aqui para que checkpoint/VACUUM
		// aguardem o lock sob contenção em vez de falhar imediatamente com
		// SQLITE_BUSY (AEP-0074).
		if _, err := conn.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout=%d", sqliteMaintenanceBusyTimeout.Milliseconds())); err != nil {
			logging.Warnf(ctx, "database.maintenance", "[Database] manutenção: set busy_timeout falhou: %v", err)
		}

		checkpoint := func(mode string) (int, error) {
			var busy, pages, copied int
			err := conn.QueryRowContext(ctx, "PRAGMA wal_checkpoint("+mode+")").Scan(&busy, &pages, &copied)
			if err != nil {
				logging.Errorf(ctx, "database.maintenance", "[Database] manutenção: wal_checkpoint falhou: %v", err)
				return 0, err
			}
			if busy == 0 && pages == copied {
				res.WALCheckpointed = true
			}
			return busy, nil
		}
		if force || before.AutoVacuumMode != "incremental" {
			_, _ = checkpoint("TRUNCATE")
		}
		finishIncremental := func() error {
			if _, err := checkpoint("PASSIVE"); err != nil {
				return err
			}
			if res.more {
				return nil
			}
			// Tentativa final sem esperar leitores; busy é uma continuação, não sucesso.
			if _, err := conn.ExecContext(ctx, "PRAGMA busy_timeout=0"); err != nil {
				return err
			}
			busy, err := checkpoint("TRUNCATE")
			if err != nil {
				return err
			}
			res.checkpointBusy = busy != 0
			res.more = res.checkpointBusy
			return nil
		}

		runFullVacuum := func() error {
			if _, err := conn.ExecContext(ctx, "PRAGMA auto_vacuum=INCREMENTAL"); err != nil {
				logging.Errorf(ctx, "database.maintenance", "[Database] manutenção: set auto_vacuum=INCREMENTAL falhou: %v", err)
			}
			if _, err := conn.ExecContext(ctx, "VACUUM"); err != nil {
				return fmt.Errorf("vacuum: %w", err)
			}
			res.Mode = "full"
			return nil
		}

		switch before.AutoVacuumMode {
		case "incremental":
			if force {
				return runFullVacuum()
			}
			// Uma passagem não monopoliza o writer para toda a freelist.
			if before.FreelistCount == 0 {
				if err := finishIncremental(); err != nil {
					return err
				}
				return nil
			}
			if _, err := conn.ExecContext(ctx, fmt.Sprintf("PRAGMA incremental_vacuum(%d)", incrementalVacuumBatchPages)); err != nil {
				return fmt.Errorf("incremental_vacuum: %w", err)
			}
			var remaining int64
			if err := conn.QueryRowContext(ctx, "PRAGMA freelist_count").Scan(&remaining); err != nil {
				return fmt.Errorf("read remaining freelist: %w", err)
			}
			res.more = remaining > 0
			// Aplicar o WAL depois de cada lote recupera o espaço físico.
			if err := finishIncremental(); err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			res.Mode = "incremental"
		default:
			// Banco legado (none/full). VACUUM completo recupera o espaço e, com o
			// auto_vacuum definido para incremental nesta mesma conexão, converte o
			// banco para o modo incremental. Gated por limiar, salvo se forçado.
			if !force && before.FreeBytes < minFreeBytes {
				res.Mode = "skipped"
				return nil
			}
			return runFullVacuum()
		}
		return nil
	}(); err != nil {
		return res, err
	}

	after, _ := DatabaseStatsSnapshot(ctx)
	res.TotalSizeAfter = after.TotalSizeBytes
	res.ReclaimedBytes = res.TotalSizeBefore - res.TotalSizeAfter
	if res.ReclaimedBytes < 0 {
		res.ReclaimedBytes = 0
	}

	if res.Mode != "noop" && res.Mode != "skipped" {
		logging.Infof(ctx, "database.maintenance", "[Database] manutenção: modo=%s wal=%v liberado=%dB (antes=%dB depois=%dB)",
			res.Mode, res.WALCheckpointed, res.ReclaimedBytes, res.TotalSizeBefore, res.TotalSizeAfter)
	}
	return res, nil
}
