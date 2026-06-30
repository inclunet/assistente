package database

import (
	"assistente/internal/logging"
	"context"
	"errors"
	"fmt"
	"strings"
)

// ensureTaskNoteExternalUniqueIndex aplica índice único parcial em (external_source, external_id).
//
// Escolha de modelagem: chave única global por origem (sem task_id na unicidade), alinhada à
// preferência de produto e ao padrão “ID estável no sistema remoto”. O mesmo comentário Jira
// (por exemplo) deve mapear a no máximo uma TaskNote no app, impedindo duplicatas em re-syncs.
// Notas manuais permanecem fora do índice (WHERE ambos os campos não vazios).
//
// Se a mesma referência externa for associada a outra task local, UpsertTaskNoteByExternal
// retorna erro explícito em vez de duplicar linhas.
func ensureTaskNoteExternalUniqueIndex() error {
	if db == nil {
		return nil
	}
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ux_task_notes_external_source_id ON task_notes (external_source, external_id) WHERE external_source <> '' AND external_id <> ''`).Error; err != nil {
		return fmt.Errorf("criar índice ux_task_notes_external_source_id: %w", err)
	}
	return nil
}

// ensureChatMessageWindowIndex cria os índices de ordenação/paginação de
// chat_messages. Os campos CreatedAt/UpdatedAt vivem em UUIDModel (issue #20),
// compartilhado por vários models, então NÃO adicionamos `gorm:"index"` lá —
// indexaria todas as tabelas indevidamente. Em vez disso, os índices ficam
// explícitos aqui, escopados a chat_messages. Todos usam CREATE INDEX IF NOT
// EXISTS, garantindo idempotência no boot/AutoMigrate.
//
// Cobertura de queries (verificado em database.go):
//   - Os ORDER BY de mensagens usam sempre `chat_messages.created_at` (ASC/DESC)
//     com desempate por `id`, quase sempre filtrando por `conversation_id`
//     (e por vezes `parent_id`/`turn_id`). Os índices _window/_timeline_window
//     cobrem os caminhos com parent_id; idx_chat_messages_created_at
//     (conversation_id, created_at, id) cobre o caso sem o predicado de
//     parent_id — ex.: GetAllConversationMessagesWithContext, que ordena por
//     `created_at ASC, id ASC`. O `id` no índice permite resolver os empates de
//     created_at pela própria leitura ordenada do índice, sem sort adicional.
//   - Nenhuma query atual ordena ou filtra `chat_messages.updated_at`
//     (ordenações por updated_at existem apenas em conversations/mcp). Ainda
//     assim, o issue #20 pede o índice e o campo é atualizado a cada edição de
//     mensagem; idx_chat_messages_updated_at (conversation_id, updated_at)
//     deixa pronto o caminho para listagens incrementais por "modificadas
//     recentemente" sem custo relevante de manutenção.
//
// Falhas de criação de índice são logadas como aviso e a função retorna o
// primeiro erro encontrado. No boot, o wrapper da migração trata esse erro como
// adiamento (errMigrationDeferred via deferIfErr): NÃO aborta a inicialização e
// retenta no próximo startup — o app ainda funciona sem o índice (apenas com
// queries mais lentas), e abortar o boot por causa de um índice seria pior do
// que degradar performance. (Os call sites em testes tratam o erro como fatal.)
func ensureChatMessageWindowIndex() error {
	if db == nil {
		return nil
	}
	indexStmts := []string{
		`CREATE INDEX IF NOT EXISTS idx_chat_messages_window ON chat_messages (conversation_id, parent_id, created_at, id)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_messages_timeline_window ON chat_messages (conversation_id, parent_id, turn_id, created_at, id)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_messages_created_at ON chat_messages (conversation_id, created_at, id)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_messages_updated_at ON chat_messages (conversation_id, updated_at)`,
	}
	var firstErr error
	for _, stmt := range indexStmts {
		if err := db.Exec(stmt).Error; err != nil {
			logging.Warnf(context.Background(), "database.schema-migrations", "[Database] AVISO: falha ao criar índice de chat_messages: %v (stmt: %s)", err, stmt)
			if firstErr == nil {
				firstErr = fmt.Errorf("criar índice chat_messages: %w", err)
			}
		}
	}
	return firstErr
}

// dedupCredentialEntriesBeforeMigrate remove duplicatas em
// `(user_id, pattern)` em bases legadas antes que o AutoMigrate tente
// criar o índice unique. Mantém a entry mais recente (maior `updated_at`,
// ties por `id` UUIDv7 desc) por chave (user_id, pattern). É idempotente:
// se a tabela ainda não existe ou já está sem duplicatas, é noop.
//
// Roda **antes** do AutoMigrate porque o GORM cria o índice unique a
// partir da tag `uniqueIndex` no model, e bases pré-AEP-0052 podiam ter
// `pattern` repetido entre registros legacy sem dono — sem dedup prévio o
// AutoMigrate falha e o app não sobe (review do AEP-0052, Bloco 6, B31).
//
// Retorno (sob o versionamento de schema, AEP-0076):
//   - nil quando não há nada a fazer de forma definitiva (sem tabela ou já
//     deduplicado) ou após deduplicar com sucesso — a v2 é registrada;
//   - erro real quando o DELETE falha — a v2 NÃO é registrada e o boot aborta
//     (o índice unique quebraria de qualquer forma); retentada no próximo boot;
//   - errMigrationDeferred quando a tabela existe mas ainda não tem `user_id`
//     (base pré-AEP-0052): adia sem registrar, pois o dedup por (user_id,
//     pattern) só é possível depois que o AutoMigrate adicionar a coluna.
func dedupCredentialEntriesBeforeMigrate() error {
	if db == nil {
		return nil
	}
	if !db.Migrator().HasTable("credential_entries") {
		return nil
	}
	if !legacyColumnExists("credential_entries", "user_id") {
		// Base legada pré-AEP-0052: a tabela existe mas ainda NÃO tem a coluna
		// user_id (será adicionada pelo AutoMigrate). O dedup por (user_id,
		// pattern) só é possível depois disso, então adia para o próximo boot
		// em vez de registrar a v2 prematuramente. No boot seguinte (já com
		// user_id) o dedup roda de fato — inclusive recuperando o caso em que o
		// AutoMigrate falhou ao criar o índice unique por duplicatas recém-
		// expostas (a coluna user_id é adicionada mesmo quando a criação do
		// índice falha, então o retry consegue deduplicar e destravar o boot).
		return fmt.Errorf("credential_entries ainda sem coluna user_id (pré-AutoMigrate): %w", errMigrationDeferred)
	}
	res := db.Exec(`
		DELETE FROM credential_entries
		WHERE pattern IS NOT NULL
		AND id NOT IN (
			SELECT id FROM credential_entries ce
			WHERE ce.id = (
			    SELECT inner_ce.id FROM credential_entries inner_ce
			    WHERE inner_ce.user_id = ce.user_id
			      AND inner_ce.pattern = ce.pattern
			    ORDER BY inner_ce.updated_at DESC, inner_ce.id DESC
			    LIMIT 1
			)
		)
	`)
	if res.Error != nil {
		// Propaga o erro: o dedup PRECISA preceder a criação do índice unique
		// (user_id, pattern) pelo AutoMigrate. Se falhar e a migração fosse
		// registrada mesmo assim, o índice continuaria quebrando em todo boot
		// e o app ficaria permanentemente sem subir. Retornando erro, a v2 não
		// é registrada e é retentada no próximo startup.
		return fmt.Errorf("dedup de credential_entries: %w", res.Error)
	}
	if res.RowsAffected > 0 {
		logging.Infof(context.Background(), "database.schema-migrations", "[Database] dedup de credential_entries: %d duplicatas removidas (user_id, pattern)", res.RowsAffected)
	}
	return nil
}

// ensureCredentialEntryUserPatternIndex limpa índices legados que possam
// existir em DBs antigos. O índice unique atual em (user_id, pattern) é
// criado pela tag `uniqueIndex:ux_credential_entries_user_pattern` no
// `CredentialEntry` model durante o AutoMigrate.
//
// Limitação aceita (review do AEP-0052, M42): o índice é full, não filtra
// pattern vazio. Patterns vazios também disputam unicidade. Isso é exigência
// do SQLite para que o UPSERT (`clause.OnConflict`) usado em
// `credentials/db_store.go` funcione — SQLite só aceita ON CONFLICT contra
// índices unique sem `WHERE`. Em prática o app sempre grava patterns
// não-vazios.
func ensureCredentialEntryUserPatternIndex() error {
	if db == nil {
		return nil
	}

	if err := db.Exec(`DROP INDEX IF EXISTS idx_credential_entries_pattern`).Error; err != nil {
		return fmt.Errorf("limpar índice legado idx_credential_entries_pattern: %w", err)
	}
	return nil
}

// ensureUsernameCaseInsensitive normaliza usernames legados para lowercase e
// aplica defesa em DB contra registros case-variantes (Alice vs alice).
//
// Decisões (review do AEP-0052, Bloco 6, B34):
//
//   - **Normalização one-shot:** percorre `users` cujo `username` contém
//     maiúsculas e tenta `LOWER(username)`. Se há colisão (ex.: já existe
//     `alice` e tentamos baixar `Alice`), preserva o registro mais antigo
//     (menor `id` UUIDv7 ≈ criado primeiro) e desativa o duplicado em vez de
//     deletar — evita perda silenciosa de dados de um usuário real.
//   - **Defesa em DB:** cria `UNIQUE INDEX users_username_lower_unique ON
//     users(LOWER(username))` para impedir que INSERTs futuros com case
//     diferente coexistam (defense in depth — `IdentityService` já normaliza
//     no `Save`, mas migrações externas ou ferramentas administrativas podiam
//     burlar).
//   - **Compatibilidade:** mantém o índice unique padrão em `username` —
//     ambos coexistem (o em LOWER apenas adiciona invariante adicional).
func ensureUsernameCaseInsensitive() error {
	if db == nil {
		return nil
	}
	if !db.Migrator().HasTable(&User{}) {
		return nil
	}

	type userRow struct {
		ID       string
		Username string
	}
	var legacyMixedCase []userRow
	if err := db.Raw(`SELECT id, username FROM users WHERE username <> LOWER(username)`).Scan(&legacyMixedCase).Error; err != nil {
		return fmt.Errorf("scan legacy mixed-case usernames: %w", err)
	}

	for _, row := range legacyMixedCase {
		lower := strings.ToLower(row.Username)
		var conflictID string
		err := db.Raw(`SELECT id FROM users WHERE username = ? AND id <> ? LIMIT 1`, lower, row.ID).Scan(&conflictID).Error
		if err != nil {
			return fmt.Errorf("check username collision %q: %w", row.Username, err)
		}
		if conflictID != "" {
			// Preserva o registro MAIS ANTIGO (menor UUIDv7 ≈ criado primeiro) e
			// desativa o duplicado MAIS RECENTE: loser é o de MAIOR id.
			loser := row.ID
			if conflictID > loser {
				loser = conflictID
			}
			suffix := loser
			if len(suffix) > 8 {
				suffix = suffix[:8]
			}
			deactivated := fmt.Sprintf("%s.legacy.%s", lower, suffix)
			if err := db.Exec(`UPDATE users SET username = ?, is_active = 0 WHERE id = ?`, deactivated, loser).Error; err != nil {
				return fmt.Errorf("deactivate legacy duplicate username %q: %w", row.Username, err)
			}
			logging.Warnf(context.Background(), "database.schema-migrations", "[Database] AVISO: username legacy %q desativado por colisão case-insensitive (id=%s renomeado para %q)", row.Username, loser, deactivated)
			if loser == row.ID {
				continue
			}
		}
		if err := db.Exec(`UPDATE users SET username = ? WHERE id = ?`, lower, row.ID).Error; err != nil {
			return fmt.Errorf("normalize username %q: %w", row.Username, err)
		}
	}

	if len(legacyMixedCase) > 0 {
		logging.Errorf(context.Background(), "database.schema-migrations", "[Database] usernames legacy normalizados para lowercase: %d", len(legacyMixedCase))
	}

	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS users_username_lower_unique ON users (LOWER(username))`).Error; err != nil {
		// Só o índice é best-effort: adia (não aborta o boot) e retenta no
		// próximo startup. As normalizações de dados acima, se falharem,
		// retornam erro real e abortam o boot (não são adiadas).
		// errors.Join preserva o erro original do SQLite na cadeia de unwrap
		// (inspeção via errors.As) além do sentinela errMigrationDeferred.
		return errors.Join(fmt.Errorf("criar índice users_username_lower_unique: %w", err), errMigrationDeferred)
	}
	return nil
}

// migrateRefreshURLToEnc move dados da coluna legacy `refresh_url` (texto plano)
// para `refresh_token_enc` em `credential_entries` e dropa a coluna antiga.
//
// Decisões (review do AEP-0052, Bloco 6, B30):
//
//   - **Idempotente:** se a coluna `refresh_url` já foi dropada em boot
//     anterior, é noop.
//   - **Não cifra aqui:** o conteúdo era texto plano (URL com token na
//     query) e vai para `refresh_token_enc` como está, porque esta migração
//     roda em `Init()`, ANTES da DEK ser carregada do keychain — e este
//     pacote não pode importar `credentials` (ciclo de import). A cifragem
//     acontece logo em seguida, no MESMO startup: o `credentials.Manager`
//     executa a re-cifragem one-shot idempotente
//     (`reencryptLegacyPlaintextRefreshTokens`, issue #236) dentro de
//     `LoadInstanceSecrets`, assim que é configurado com a DEK — antes de
//     qualquer login. Se a DEK não estiver disponível neste boot (keychain
//     ausente/pré-Setup), os valores ficam como estavam no disco (eram
//     plain em `refresh_url` de qualquer forma) e a re-cifragem ocorre no
//     primeiro boot com DEK.
//   - **Logs:** registra quantas linhas foram tocadas e se o drop falhou.
//   - **DROP COLUMN via SQL direto:** `Migrator().DropColumn` faz lookup na
//     struct Go (que não tem mais o campo) e vira noop silencioso. SQL puro
//     `ALTER TABLE ... DROP COLUMN` é suportado em SQLite >= 3.35 (todas as
//     builds modernas, inclusive `glebarez/sqlite` Pure Go).
//   - **Sem transação dedicada:** GORM/SQLite executa ALTER + UPDATE em
//     auto-commit, mas em caso de crash entre passos a próxima execução
//     reinicia do ponto correto (idempotente).
func migrateRefreshURLToEnc() error {
	if db == nil {
		return nil
	}
	if !legacyColumnExists("credential_entries", "refresh_url") {
		return nil
	}

	res := db.Exec(`UPDATE credential_entries SET refresh_token_enc = refresh_url WHERE refresh_url IS NOT NULL AND refresh_url <> '' AND (refresh_token_enc IS NULL OR refresh_token_enc = '')`)
	if res.Error != nil {
		return fmt.Errorf("migrar refresh_url para refresh_token_enc: %w", res.Error)
	}
	if res.RowsAffected > 0 {
		logging.Errorf(context.Background(), "database.schema-migrations", "[Database] credential_entries: %d linhas migradas refresh_url → refresh_token_enc", res.RowsAffected)
	}

	if err := db.Exec(`ALTER TABLE credential_entries DROP COLUMN refresh_url`).Error; err != nil {
		// Os dados já foram copiados para refresh_token_enc acima; só o DROP da
		// coluna legada falhou (ex.: SQLite sem suporte a DROP COLUMN). Sinaliza
		// adiamento: não aborta o boot e NÃO registra a v9, para que o drop seja
		// retentado no próximo startup — preservando o comportamento anterior ao
		// versionamento (rodava a cada boot até a coluna sumir) e evitando deixar
		// a coluna em texto plano gravada como "migrada".
		logging.Warnf(context.Background(), "database.schema-migrations", "[Database] AVISO: falha ao dropar coluna legacy refresh_url (será retentado no próximo boot): %v", err)
		// errors.Join preserva o erro original na cadeia de unwrap além do
		// sentinela errMigrationDeferred (mesmo padrão de deferIfErr).
		return errors.Join(fmt.Errorf("dropar coluna legacy refresh_url: %w", err), errMigrationDeferred)
	}
	return nil
}

// legacyColumnExists checa se uma coluna existe no DB via PRAGMA, sem
// depender da struct Go atual (necessário para colunas removidas do model
// mas ainda presentes no schema legado).
func legacyColumnExists(table, column string) bool {
	if db == nil {
		return false
	}
	var n int
	err := db.Raw(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n).Error
	return err == nil && n > 0
}
