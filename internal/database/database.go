package database

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"assistente/internal/configdir"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var db *gorm.DB

// dbPath guarda o caminho absoluto do arquivo SQLite resolvido em Init().
// Usado pela manutenção física (maintenance.go) para medir tamanho em disco.
var dbPath string

// gormLogLevel controla o nível de log do GORM. Padrão: Warn.
// Use SetLogLevel(logger.Silent) para silenciar completamente (ex.: CLI sem --verbose).
var gormLogLevel = logger.Warn

// SetLogLevel define o nível de log do GORM antes de Init().
func SetLogLevel(level logger.LogLevel) {
	gormLogLevel = level
}

// ErrConversationDeleted é retornado quando se tenta salvar mensagem em conversa que foi deletada
// Os chamadores devem verificar esse erro e abortar o processamento graciosamente
var ErrConversationDeleted = errors.New("conversa foi deletada")

// ErrParentMessageDeleted é retornado quando se tenta criar mensagem com parentId que não existe mais
// Isso acontece quando a conversa foi limpa (clear) - as mensagens foram deletadas mas a conversa ainda existe
var ErrParentMessageDeleted = errors.New("mensagem pai foi deletada")

// DB retorna a instância do banco de dados
func DB() *gorm.DB {
	return db
}

// SetDB define a instância do banco de dados (usado em testes)
func SetDB(database *gorm.DB) {
	db = database
}

// Close fecha a conexão com o banco de dados
func Close() error {
	if db == nil {
		return nil
	}

	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	return sqlDB.Close()
}

// Init inicializa o banco de dados
// Resolve conversations.db nos 3 diretórios (exe > home > workdir).
// Se não existir em nenhum, cria em ~/.assistente/
func Init() error {
	rootResolver := configdir.NewResolver("")

	resolved, err := rootResolver.Resolve("conversations.db")
	if err != nil {
		// Não existe em nenhum diretório — criar no home
		if err := rootResolver.EnsureHomeDir(); err != nil {
			return err
		}
		dbPath = filepath.Join(configdir.GetHomeDir(), "conversations.db")
	} else {
		dbPath = resolved.Path
	}

	db, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(gormLogLevel),
	})
	if err != nil {
		return err
	}

	// auto_vacuum=INCREMENTAL: bancos NOVOS nascem podendo devolver páginas
	// livres ao SO via incremental_vacuum, sem reescrever o arquivo inteiro.
	// DEVE ser definido antes de qualquer tabela existir (e antes do WAL, para
	// não escrever páginas que fixem o modo none). Em bancos legados o pragma é
	// no-op aqui; a conversão ocorre no primeiro VACUUM da manutenção
	// (maintenance.go, AEP-0074).
	db.Exec("PRAGMA auto_vacuum=INCREMENTAL")
	// Ativa modo WAL para melhor performance com arquivos grandes
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA synchronous=NORMAL")
	// busy_timeout: sob contenção (WAL com writers de background) operações
	// como VACUUM aguardam o lock em vez de falhar com SQLITE_BUSY (AEP-0074).
	db.Exec("PRAGMA busy_timeout=5000")

	// Migração: converter IDs INTEGER → UUIDv7 (AEP-0046)
	if err := migrateToUUIDv7(); err != nil {
		return fmt.Errorf("erro na migração UUIDv7: %w", err)
	}

	// Bases legadas pré-AEP-0052 podem ter (user_id, pattern) duplicado em
	// credential_entries; dedup antes do AutoMigrate criar o índice unique.
	dedupCredentialEntriesBeforeMigrate()

	// Auto migrate das tabelas persistidas no SQLite; perfis continuam
	// gerenciados via arquivos JSON em .assistente/profiles/.
	if err := db.AutoMigrate(
		&User{},
		&Session{},
		&Conversation{},
		&ChatMessage{},
		&MemoryRecord{},
		&CredentialEntry{},
		&CredentialKeyWrap{},
		&LLMProvider{},
		&TaskListWorkflow{},
		&TaskList{},
		&Task{},
		&TaskNote{},
		&MCPServer{},
		&MCPServerLog{},
		&ToolCatalog{},
		&Tag{},
		&TagAssignment{},
		&JobPipeline{},
		&Job{},
		&JobTrigger{},
		&JobRun{},
		&JobEvent{},
		&JobRunEvent{},
		&ToolInvocation{},
		&SubAgentRun{},
	); err != nil {
		return err
	}

	ensureTaskNoteExternalUniqueIndex()
	ensureTaskListSlugUniqueIndex()
	ensureChatMessageWindowIndex()
	ensureCredentialEntryUserPatternIndex()
	if err := ensureUsernameCaseInsensitive(); err != nil {
		return err
	}

	// Normalizar campos booleanos: SQLite armazena bool como INTEGER 0/1,
	// mas valores corrompidos (ex: 4) causam erro no GORM Scan.
	db.Exec(`UPDATE conversations SET summarizing_in_progress = CASE WHEN summarizing_in_progress > 0 THEN 1 ELSE 0 END WHERE summarizing_in_progress NOT IN (0, 1)`)

	if err := migrateRefreshURLToEnc(); err != nil {
		return err
	}

	// Inicializa FTS5 (full-text search) para busca em mensagens
	if err := initFTS5(); err != nil {
		return fmt.Errorf("erro ao inicializar FTS5: %w", err)
	}

	// Verifica se o índice FTS5 está desatualizado e precisa de rebuild
	sqlDB, err := db.DB()
	if err == nil {
		var ftsCount, msgCount int
		_ = sqlDB.QueryRow(`SELECT count(*) FROM chat_messages_fts`).Scan(&ftsCount)
		_ = sqlDB.QueryRow(`SELECT count(*) FROM chat_messages WHERE role IN ('user','assistant') AND content != ''`).Scan(&msgCount)
		if msgCount > 0 && ftsCount < msgCount {
			log.Printf("[Database] Índice FTS5 desatualizado (%d/%d), reconstruindo...", ftsCount, msgCount)
			if err := RebuildFTSIndex(context.Background()); err != nil {
				log.Printf("[Database] ERRO: falha ao reconstruir FTS5 — busca de histórico pode estar incompleta. Será retentado no próximo startup. Erro: %v", err)
			} else {
				log.Printf("[Database] Índice FTS5 reconstruído (%d mensagens)", msgCount)
			}
		}
	}

	return nil
}

// AdoptLegacyData vincula registros single-user existentes (user_id IS NULL
// ou user_id vazio) ao usuário ativo. A operação é idempotente.
//
// PONTOS DE CHAMADA (P0-4 do re-review da Fatia 1):
//   - Login (`app_auth.go` em `adoptLegacyDataForUser`): roda em TODO
//     login bem-sucedido. Idempotente após o primeiro: a partir do
//     segundo login do mesmo usuário, o WHERE não casa nada.
//   - RefreshAuth (`app_auth.go` em `adoptLegacyDataForUser`): roda em
//     TODO refresh bem-sucedido. Mesma idempotência.
//
// (`CreateAdminUser` por si só NÃO chama AdoptLegacyData — quem adota
// é o primeiro Login após a criação do admin, exatamente como descrito
// nos call sites acima.)
//
// SECURITY: instance-wide — varre TODAS as tabelas que carregam
// `user_id`. O WHERE é restrito a registros sem dono,
// portanto registros legitimamente atribuídos a outro usuário NÃO são
// re-atribuídos. Concretamente: User B logando depois de User A NÃO
// herda dados de A — o A já adotou tudo no primeiro login dele e o
// WHERE da B não casa mais nada.
//
// PREMISSA CRÍTICA: nenhum caminho produz registros órfãos (user_id
// vazio) DEPOIS do bootstrap. Se alguma migração futura (import legacy,
// fix de schema, restore de backup pré-AEP-0052) introduzir órfãos em
// runtime, o próximo login a executar AdoptLegacyData os atribuirá ao
// caller — possivelmente ao usuário errado. Validar essa premissa
// antes de qualquer mudança que produza órfãos em runtime.
func AdoptLegacyData(userID string) error {
	if db == nil {
		return errors.New("banco de dados não inicializado")
	}
	if strings.TrimSpace(userID) == "" {
		return errors.New("userID obrigatório")
	}

	return db.Transaction(func(tx *gorm.DB) error {
		tables := []string{
			"llm_providers",
			"conversations",
			"task_lists",
			"memory_records",
		}
		for _, table := range tables {
			if !tx.Migrator().HasTable(table) {
				continue
			}
			if err := tx.Exec(
				fmt.Sprintf("UPDATE %s SET user_id = ? WHERE user_id IS NULL OR user_id = ''", table),
				userID,
			).Error; err != nil {
				return err
			}
		}
		// Antes do UPDATE genérico de credential_entries, removemos órfãs
		// (user_id IS NULL/'') cujo `pattern` JÁ está reivindicado pelo
		// userID corrente. Sem isso o UPDATE viola
		// `ux_credential_entries_user_pattern` (user_id, pattern) e a
		// transação inteira aborta, deixando o login do admin recém-criado
		// em estado inconsistente: o User existe no banco, mas a sessão
		// nunca completa e a próxima tentativa de CreateAdminUser bate em
		// "admin inicial já foi criado".
		//
		// O `dedupCredentialEntriesBeforeMigrate` que roda antes do
		// AutoMigrate só dedupa pares EXATAMENTE iguais em (user_id,
		// pattern), então não pega o cenário órfã+claimed do mesmo pattern.
		// A versão claimed é sempre canônica (foi escrita pelo user real,
		// possui chave wrap atualizada); a órfã é resíduo de boots antigos
		// e pode ser descartada sem perda de dados.
		if err := tx.Exec(
			`DELETE FROM credential_entries
			 WHERE (user_id IS NULL OR user_id = '')
			   AND pattern NOT LIKE 'internal-auth:%'
			   AND pattern NOT LIKE 'internal-tls:%'
			   AND EXISTS (
			     SELECT 1 FROM credential_entries claimed
			     WHERE claimed.pattern = credential_entries.pattern
			       AND claimed.user_id = ?
			   )`,
			userID,
		).Error; err != nil {
			return err
		}
		if err := tx.Exec(
			"UPDATE credential_entries SET user_id = ? WHERE (user_id IS NULL OR user_id = '') AND pattern NOT LIKE 'internal-auth:%' AND pattern NOT LIKE 'internal-tls:%'",
			userID,
		).Error; err != nil {
			return err
		}
		if err := tx.Exec(
			`DELETE FROM credential_entries
			 WHERE (pattern LIKE 'internal-auth:%' OR pattern LIKE 'internal-tls:%')
			   AND user_id != ''
			   AND EXISTS (
			     SELECT 1 FROM credential_entries existing
			     WHERE existing.pattern = credential_entries.pattern
			       AND (existing.user_id IS NULL OR existing.user_id = '')
			   )`,
		).Error; err != nil {
			return err
		}
		if err := tx.Exec(
			`UPDATE credential_entries
			 SET user_id = ''
			 WHERE (pattern LIKE 'internal-auth:%' OR pattern LIKE 'internal-tls:%')
			   AND user_id != ''`,
		).Error; err != nil {
			return err
		}
		return nil
	})
}
