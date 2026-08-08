package database

import (
	"assistente/internal/logging"
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
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
		logging.Infof(context.Background(), "database.schema-migrations", "[Database] usernames legacy normalizados para lowercase: %d", len(legacyMixedCase))
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
		logging.Infof(context.Background(), "database.schema-migrations", "[Database] credential_entries: %d linhas migradas refresh_url → refresh_token_enc", res.RowsAffected)
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

// dedupACPSessionsSemDono elege uma única sobrevivente por (conversa, provider)
// entre as linhas que ficarão com `user_id` vazio depois da normalização.
//
// O `COALESCE(i.user_id, '') = ''` junta num mesmo grupo o que o índice unique
// hoje enxerga como linhas diferentes: NULL e string vazia. O `ORDER BY` decide
// quem fica — `(i.user_id IS NULL) ASC` põe a linha que JÁ está com string
// vazia na frente (ela é a que o app grava hoje), e entre linhas nulas vence a
// mais recente, com desempate por `id` UUIDv7 desc.
const dedupACPSessionsSemDono = `
	DELETE FROM acp_sessions
	WHERE user_id IS NULL
	  AND id NOT IN (
		SELECT s.id FROM acp_sessions s
		WHERE s.user_id IS NULL
		  AND s.id = (
			SELECT i.id FROM acp_sessions i
			WHERE COALESCE(i.user_id, '') = ''
			  AND i.conversation_id = s.conversation_id
			  AND i.provider_id = s.provider_id
			ORDER BY (i.user_id IS NULL) ASC, i.updated_at DESC, i.id DESC
			LIMIT 1
		  )
	  )`

// normalizeACPSessionUserID troca `acp_sessions.user_id` NULL por string vazia
// em bases criadas antes de a coluna virar `NOT NULL DEFAULT ''` (AEP-0084
// D12).
//
// Roda **antes** do AutoMigrate porque é ele quem aplica a constraint, e o
// SQLite não sabe alterar coluna no lugar: o driver recria a tabela e copia as
// linhas para dentro do novo formato. Com uma única linha nula essa cópia falha
// com `NOT NULL constraint failed: acp_sessions__temp.user_id` — o Init aborta
// e o app não sobe. Mesma razão do dedup de credential_entries (v2).
//
// A limpeza vem antes da troca porque hoje o índice unique não protege quem não
// tem dono: no SQLite dois NULL não se comparam iguais, então
// `idx_acp_sessions_scope` deixa passar várias linhas nulas para a mesma
// conversa e provider. Trocar tudo por string vazia de uma vez faria essas
// linhas colidirem — o UPDATE morre no índice antes mesmo do AutoMigrate.
//
// Perder um vínculo desses custa: sem ele o app não reencontra a sessão que o
// agente ainda mantém, e ela fica órfã do lado dele (AEP-0084 D4). Por isso a
// remoção é anunciada no log, e o critério guarda a linha mais recente — a que
// tem a maior chance de descrever a sessão viva.
//
// Erros aqui abortam o boot em vez de adiar: o AutoMigrate logo em seguida
// falharia do mesmo jeito, só que com uma mensagem sobre uma tabela temporária
// que não existe em lugar nenhum do código.
func normalizeACPSessionUserID(database *gorm.DB) error {
	if database == nil {
		return nil
	}
	if !database.Migrator().HasTable("acp_sessions") {
		return nil
	}
	var semDono int64
	if err := database.Raw(`SELECT count(*) FROM acp_sessions WHERE user_id IS NULL`).Scan(&semDono).Error; err != nil {
		return fmt.Errorf("contar sessões ACP com user_id nulo: %w", err)
	}
	if semDono == 0 {
		return nil
	}

	res := database.Exec(dedupACPSessionsSemDono)
	if res.Error != nil {
		return fmt.Errorf("dedup de acp_sessions sem dono: %w", res.Error)
	}
	if res.RowsAffected > 0 {
		logging.Warnf(context.Background(), "database.schema-migrations", "[Database] acp_sessions: %d vínculos sem dono descartados por disputarem a mesma conversa e provider — a sessão correspondente pode ter ficado aberta no agente", res.RowsAffected)
	}

	up := database.Exec(`UPDATE acp_sessions SET user_id = '' WHERE user_id IS NULL`)
	if up.Error != nil {
		return fmt.Errorf("normalizar acp_sessions.user_id nulo: %w", up.Error)
	}
	if up.RowsAffected > 0 {
		logging.Infof(context.Background(), "database.schema-migrations", "[Database] acp_sessions: %d linhas com user_id nulo normalizadas para string vazia", up.RowsAffected)
	}
	return nil
}

// agentTypeToRegistryID é a tradução usada uma única vez, na migração v12, dos
// tipos de provedor que existiram enquanto cada agente tinha o seu (AEP-0086
// D11, emenda).
//
// Ela mora aqui, e não num pacote de domínio, porque é o que ela é: memória de
// um vocabulário que o app não fala mais. Depois de convertidos os provedores,
// nada no código volta a perguntar isso — quem quiser saber qual agente é um
// provedor lê o `acp_agent_id` dele.
var agentTypeToRegistryID = map[string]string{
	"cursor":      "cursor",
	"claude-code": "claude-acp",
}

// migrateAgentProvidersToSingleType converte os provedores de agente gravados
// com tipo próprio para o tipo único `acp` com o `id` do registro à parte.
//
// O comando não é tocado: ele é a escolha de quem configurou o provedor, e a
// Fase 3 do AEP-0084 já decidiu que nem a detecção o sobrescreve. O que muda é
// o rótulo e o campo novo — o agente continua subindo exatamente igual.
//
// A conversão olha o `api_format`, e não só o tipo, porque o tipo sozinho é
// ambíguo por herança: um provedor HTTP chamado `cursor` por alguém que digitou
// aquilo à mão não é agente, e trocar o tipo dele o quebraria.
//
// Roda depois do AutoMigrate porque depende da coluna `acp_agent_id` já existir,
// e falhar aqui adia em vez de abortar: o provedor continua subindo o mesmo
// comando com o tipo antigo, e a conversão é retentada no próximo boot.
func migrateAgentProvidersToSingleType(database *gorm.DB) error {
	if database == nil || !database.Migrator().HasTable("llm_providers") {
		return nil
	}
	if !database.Migrator().HasColumn(&LLMProvider{}, "acp_agent_id") {
		return errors.Join(
			fmt.Errorf("a coluna acp_agent_id ainda não existe em llm_providers"),
			errMigrationDeferred,
		)
	}
	for tipo, agentID := range agentTypeToRegistryID {
		res := database.Exec(
			`UPDATE llm_providers SET type = ?, acp_agent_id = ? WHERE type = ? AND api_format = ?`,
			string(providerTypeACP), agentID, tipo, apiFormatACP,
		)
		if res.Error != nil {
			return errors.Join(
				fmt.Errorf("converter os provedores do agente %s: %w", tipo, res.Error),
				errMigrationDeferred,
			)
		}
		if res.RowsAffected > 0 {
			logging.Infof(context.Background(), "database.schema-migrations",
				"[Database] llm_providers: %d provedores do agente %s passaram a ser do tipo acp", res.RowsAffected, tipo)
		}
	}
	return nil
}

// Os dois valores que a migração escreve. Estão aqui como literais em vez de
// virem de `internal/llm` porque migração descreve o banco no momento em que
// rodou: se o domínio renomear o tipo depois, esta migração continua tendo que
// escrever o que escreveu na época, ou bancos convertidos e por converter
// deixariam de concordar.
const (
	providerTypeACP = "acp"
	apiFormatACP    = "acp"
)

// dropACPSessionPromptPrefixHash solta a coluna que guardava o resumo do
// prefixo de perfil que a sessão do agente já tinha ouvido. Ela existia para
// não repetir persona e skills a cada turno; agora nada do app vai ao agente
// (AEP-0084 D4, revisto na Fase 8), e a coluna só guardaria o hash de um texto
// que ninguém mais envia.
//
// Roda depois do AutoMigrate: o campo saiu do model, então não há quem a
// recrie. Falhar aqui não impede o app de subir — a coluna sobra sem uso até a
// próxima tentativa —, e por isso o erro vira adiamento, como no drop de
// refresh_url (v9).
func dropACPSessionPromptPrefixHash(database *gorm.DB) error {
	if database == nil || !database.Migrator().HasTable("acp_sessions") {
		return nil
	}
	var existe int64
	if err := database.Raw(`SELECT COUNT(*) FROM pragma_table_info('acp_sessions') WHERE name = 'prompt_prefix_hash'`).Scan(&existe).Error; err != nil {
		return errors.Join(fmt.Errorf("procurar a coluna prompt_prefix_hash: %w", err), errMigrationDeferred)
	}
	if existe == 0 {
		return nil
	}
	if err := database.Exec(`ALTER TABLE acp_sessions DROP COLUMN prompt_prefix_hash`).Error; err != nil {
		logging.Warnf(context.Background(), "database.schema-migrations",
			"[Database] AVISO: falha ao dropar a coluna prompt_prefix_hash de acp_sessions (será retentado no próximo boot): %v", err)
		return errors.Join(fmt.Errorf("dropar a coluna prompt_prefix_hash: %w", err), errMigrationDeferred)
	}
	logging.Infof(context.Background(), "database.schema-migrations",
		"[Database] acp_sessions: coluna prompt_prefix_hash removida — o app não manda mais instruções ao agente")
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
