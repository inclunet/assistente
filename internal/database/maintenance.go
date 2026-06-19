package database

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
)

// defaultVacuumMinFreeBytes é o limiar mínimo de espaço livre (freelist) usado
// quando o chamador passa um valor inválido (< 0). Um valor 0 é literal
// ("sempre compactar"). A política real vem do config.json (AEP-0074); este
// pacote não lê configuração diretamente.
const defaultVacuumMinFreeBytes int64 = 16 * 1024 * 1024 // 16 MiB

// compactionMu serializa GLOBALMENTE toda compactação física do arquivo .db,
// independentemente da origem (loop de retenção ou botão "Limpar agora" da UI).
// Sem isso, dois VACUUM concorrentes poderiam disputar o lock exclusivo do
// SQLite e gerar SQLITE_BUSY (issue #292, AEP-0074).
var compactionMu sync.Mutex

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
	Mode            string `json:"mode"` // noop | incremental | full | skipped
	WALCheckpointed bool   `json:"walCheckpointed"`
	FreeBytesBefore int64  `json:"freeBytesBefore"`
	TotalSizeBefore int64  `json:"totalSizeBefore"`
	TotalSizeAfter  int64  `json:"totalSizeAfter"`
	ReclaimedBytes  int64  `json:"reclaimedBytes"`
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

// fileSize retorna o tamanho de um arquivo, ou 0 se não existir.
func fileSize(path string) int64 {
	if path == "" {
		return 0
	}
	info, err := os.Stat(path)
	if err != nil {
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
			log.Printf("[Database] manutenção: leitura de PRAGMA %s falhou: %v", pragma, err)
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

// Compact executa a manutenção física do arquivo SQLite (AEP-0074):
//
//   - wal_checkpoint(TRUNCATE) para limitar o arquivo -wal;
//   - em bancos no modo incremental: PRAGMA incremental_vacuum (barato);
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
// serializada globalmente por compactionMu.
func Compact(ctx context.Context, force bool, minFreeBytes int64) (CompactionResult, error) {
	res := CompactionResult{Mode: "noop"}
	if db == nil {
		return res, errors.New("banco de dados não inicializado")
	}
	if minFreeBytes < 0 {
		minFreeBytes = defaultVacuumMinFreeBytes
	}

	compactionMu.Lock()
	defer compactionMu.Unlock()

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
	if err := func() error {
		conn, err := sqlDB.Conn(ctx)
		if err != nil {
			return err
		}
		defer conn.Close()

		// 1) Checkpoint do WAL: devolve o conteúdo do -wal ao arquivo principal e
		//    trunca o -wal. Best-effort.
		if _, err := conn.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
			log.Printf("[Database] manutenção: wal_checkpoint falhou: %v", err)
		} else {
			res.WALCheckpointed = true
		}

		switch before.AutoVacuumMode {
		case "incremental":
			// Devolve todas as páginas livres ao SO. Sem argumento = todas.
			if _, err := conn.ExecContext(ctx, "PRAGMA incremental_vacuum"); err != nil {
				return fmt.Errorf("incremental_vacuum: %w", err)
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
			if _, err := conn.ExecContext(ctx, "PRAGMA auto_vacuum=INCREMENTAL"); err != nil {
				log.Printf("[Database] manutenção: set auto_vacuum=INCREMENTAL falhou: %v", err)
			}
			if _, err := conn.ExecContext(ctx, "VACUUM"); err != nil {
				return fmt.Errorf("vacuum: %w", err)
			}
			res.Mode = "full"
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
		log.Printf("[Database] manutenção: modo=%s wal=%v liberado=%dB (antes=%dB depois=%dB)",
			res.Mode, res.WALCheckpointed, res.ReclaimedBytes, res.TotalSizeBefore, res.TotalSizeAfter)
	}
	return res, nil
}
