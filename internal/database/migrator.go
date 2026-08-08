package database

import (
	"assistente/internal/logging"
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// errMigrationDeferred sinaliza que uma migração não conseguiu concluir seu
// efeito neste boot mas é SEGURA para retentar: ela NÃO é registrada em
// `schema_migrations` e NÃO aborta o boot — roda de novo no próximo startup.
//
// Usada por passos de limpeza tolerantes a falha que, antes do versionamento,
// rodavam a cada boot até ter sucesso (ex.: `ALTER TABLE ... DROP COLUMN` de
// uma coluna legada num SQLite sem suporte ao comando). Sem este sinal, a
// migração seria gravada na primeira execução e o efeito pendente nunca mais
// seria tentado. Não use para falhas que exigem abortar o boot (ex.: dedup que
// precisa preceder um índice unique) — nesses casos retorne o erro real.
var errMigrationDeferred = errors.New("migração adiada para o próximo boot")

// deferIfErr converte um erro de passo best-effort (ex.: criação/limpeza de
// índice, que NÃO deve abortar o boot) num adiamento: a migração não é
// registrada e é retentada no próximo startup, preservando o comportamento
// pré-versionamento desses passos (rodavam a cada boot até ter sucesso).
// Retorna nil quando não há erro.
//
// Usa errors.Join para preservar TANTO o sentinela `errMigrationDeferred`
// (para `errors.Is`) QUANTO o erro original na cadeia de unwrap (para
// inspeção/`errors.As`).
func deferIfErr(err error) error {
	if err == nil {
		return nil
	}
	return errors.Join(err, errMigrationDeferred)
}

// Versionamento de schema (AEP-0076).
//
// Este arquivo implementa o mecanismo de migrações versionadas do banco.
// Antes dele, o histórico de schema era implícito: AutoMigrate + um punhado
// de migrações custom (UUIDv7, refresh_url→enc, dedup de credenciais, vários
// ensure*Index) rodavam/checavam a CADA boot. Isso reexecutava verificações
// pesadas (ex.: a checagem de UUIDv7) em todo startup e tornava o estado do
// schema difícil de auditar.
//
// Agora cada mudança estrutural é uma `migration` numerada e idempotente,
// registrada na tabela `schema_migrations` (e espelhada em
// `PRAGMA user_version`). Cada migração roda no máximo uma vez por banco.
// `AutoMigrate` permanece responsável por ADIÇÃO de colunas/tabelas novas;
// mudanças estruturais (conversões de tipo, índices, normalizações de dados)
// passam a ser versionadas aqui.

// migrationPhase indica quando uma migração roda em relação ao AutoMigrate.
//
// Algumas migrações precisam rodar ANTES do AutoMigrate (ex.: a conversão
// UUIDv7 reescreve tabelas inteiras e precisa preceder a criação de colunas
// novas; o dedup de credential_entries precisa rodar antes do AutoMigrate
// criar o índice unique (user_id, pattern), senão a criação do índice falha).
// As demais rodam DEPOIS, quando todas as tabelas/colunas já existem (ex.:
// criação de índices auxiliares, normalizações de dados legados).
type migrationPhase int

const (
	phasePreAutoMigrate  migrationPhase = iota // antes do AutoMigrate
	phasePostAutoMigrate                       // depois do AutoMigrate
)

// migration descreve uma mudança estrutural versionada do schema.
type migration struct {
	// Version é o número sequencial e único da migração (>= 1). Define a ordem
	// de aplicação e a chave em `schema_migrations`. NUNCA reutilize ou
	// renumere versões já liberadas — apenas adicione novas ao final.
	Version int
	// Name é um identificador estável e legível (snake_case) registrado em
	// `schema_migrations.name` para auditoria.
	Name string
	// Phase indica se roda antes ou depois do AutoMigrate.
	Phase migrationPhase
	// Run aplica a migração. DEVE ser idempotente: rodar de novo num banco já
	// migrado não pode causar dano (a tabela schema_migrations evita a
	// reexecução no caminho feliz, mas idempotência é a rede de segurança
	// contra crash entre aplicar e registrar).
	Run func(*gorm.DB) error
	// DetectApplied (opcional) inspeciona o banco e retorna true quando um
	// banco LEGADO (anterior ao versionamento) já possui esta migração
	// aplicada. Quando retorna true e a migração ainda não está registrada,
	// ela é marcada como aplicada SEM reexecutar `Run`. É o que evita
	// reexecutar migrações pesadas/destrutivas (ex.: UUIDv7) em bancos que já
	// passaram por elas no fluxo antigo de boot. Para migrações idempotentes
	// e baratas, pode ser deixada nil — elas simplesmente rodam uma vez sob o
	// novo mecanismo e são registradas.
	DetectApplied func(*gorm.DB) (bool, error)
}

// schemaMigrations é a lista ORDENADA (por Version crescente) de todas as
// migrações versionadas. Para adicionar uma migração nova, acrescente uma
// entrada ao final com a próxima Version livre.
//
// IMPORTANTE — bancos pré-existentes: as migrações v1..v9 abaixo encapsulam
// as migrações custom que JÁ existiam no boot antigo. Todas são idempotentes
// e auto-detectam seu estado, então rodar uma vez sob o novo mecanismo num
// banco legado é seguro. A migração mais pesada (UUIDv7) ainda traz um
// DetectApplied para ser apenas MARCADA como aplicada em bancos que já
// estavam em UUID, sem reexecutar a reescrita de tabelas.
var schemaMigrations = []migration{
	{
		Version: 1,
		Name:    "uuidv7_id_migration",
		Phase:   phasePreAutoMigrate,
		Run:     func(*gorm.DB) error { return migrateToUUIDv7() },
		// Banco legado já em UUIDv7 (id TEXT) é marcado sem reexecutar a
		// reescrita big-bang. Banco novo (sem a tabela) deixa `Run` rodar,
		// onde a conversão é no-op.
		DetectApplied: uuidMigrationAlreadyApplied,
	},
	{
		Version: 2,
		Name:    "dedup_credential_entries_pre_unique",
		Phase:   phasePreAutoMigrate,
		Run:     func(*gorm.DB) error { return dedupCredentialEntriesBeforeMigrate() },
	},
	{
		Version: 3,
		Name:    "task_note_external_unique_index",
		Phase:   phasePostAutoMigrate,
		Run:     func(*gorm.DB) error { return deferIfErr(ensureTaskNoteExternalUniqueIndex()) },
	},
	{
		Version: 4,
		Name:    "task_list_user_slug_unique_index",
		Phase:   phasePostAutoMigrate,
		Run:     func(*gorm.DB) error { return deferIfErr(ensureTaskListSlugUniqueIndex()) },
	},
	{
		Version: 5,
		Name:    "chat_message_window_indexes",
		Phase:   phasePostAutoMigrate,
		Run:     func(*gorm.DB) error { return deferIfErr(ensureChatMessageWindowIndex()) },
	},
	{
		Version: 6,
		Name:    "credential_entry_legacy_index_cleanup",
		Phase:   phasePostAutoMigrate,
		Run:     func(*gorm.DB) error { return deferIfErr(ensureCredentialEntryUserPatternIndex()) },
	},
	{
		Version: 7,
		Name:    "username_case_insensitive",
		Phase:   phasePostAutoMigrate,
		// A função decide a severidade: erros de NORMALIZAÇÃO de dados (scan,
		// colisão, UPDATE) são reais e abortam o boot (não registram → retry);
		// apenas a falha de CREATE INDEX vem como errMigrationDeferred (adia
		// sem abortar). Por isso NÃO usamos deferIfErr aqui.
		Run: func(*gorm.DB) error { return ensureUsernameCaseInsensitive() },
	},
	{
		Version: 8,
		Name:    "normalize_summarizing_in_progress_bool",
		Phase:   phasePostAutoMigrate,
		Run: func(database *gorm.DB) error {
			// SQLite guarda bool como INTEGER; valores corrompidos (ex.: 4)
			// quebram o Scan do GORM. Normaliza para 0/1.
			return database.Exec(`UPDATE conversations SET summarizing_in_progress = CASE WHEN summarizing_in_progress > 0 THEN 1 ELSE 0 END WHERE summarizing_in_progress NOT IN (0, 1)`).Error
		},
	},
	{
		Version: 9,
		Name:    "refresh_url_to_enc",
		Phase:   phasePostAutoMigrate,
		Run:     func(*gorm.DB) error { return migrateRefreshURLToEnc() },
	},
	{
		Version: 10,
		Name:    "acp_session_user_id_not_null",
		// PRÉ: o AutoMigrate aplica o NOT NULL recriando a tabela, e a cópia
		// das linhas quebra se alguma tiver user_id nulo.
		Phase: phasePreAutoMigrate,
		Run:   normalizeACPSessionUserID,
	},
	{
		Version: 11,
		Name:    "drop_acp_session_prompt_prefix_hash",
		// PÓS: o campo saiu do model, então o AutoMigrate não recria a coluna
		// e o drop pode acontecer com a tabela já no formato novo.
		Phase: phasePostAutoMigrate,
		Run:   dropACPSessionPromptPrefixHash,
	},
	{
		Version: 12,
		Name:    "acp_providers_single_type",
		// PÓS: a conversão grava na coluna acp_agent_id, que é o AutoMigrate
		// quem acrescenta.
		Phase: phasePostAutoMigrate,
		Run:   migrateAgentProvidersToSingleType,
	},
}

// runMigrations aplica, na ordem de Version, todas as migrações da fase
// indicada que ainda não constam em `schema_migrations`. Usa a lista global
// `schemaMigrations`.
func runMigrations(database *gorm.DB, phase migrationPhase) error {
	return runMigrationList(database, phase, schemaMigrations)
}

// runMigrationList é a implementação testável de runMigrations: recebe a lista
// de migrações explicitamente para permitir testes determinísticos do
// mecanismo (ordem, idempotência, DetectApplied, parada em erro).
//
// Contrato de atomicidade: a aplicação de uma migração e o seu registro em
// `schema_migrations` NÃO compartilham uma transação única (várias migrações
// — ex.: UUIDv7 — gerenciam transações próprias). Como toda migração é
// idempotente, um crash entre aplicar e registrar apenas faz a migração rodar
// de novo no próximo boot, sem dano.
func runMigrationList(database *gorm.DB, phase migrationPhase, migrations []migration) error {
	if database == nil {
		return nil
	}
	// Validação barata: a lista DEVE estar com Version estritamente crescente
	// (logo, sem duplicatas). O loop aplica na ordem do slice; sem esta checagem,
	// editar `schemaMigrations` fora de ordem (ou passar uma lista arbitrária)
	// aplicaria versões fora de sequência silenciosamente, quebrando a premissa
	// de aplicação sequencial (e a contiguidade de user_version).
	for i := 1; i < len(migrations); i++ {
		if migrations[i].Version <= migrations[i-1].Version {
			return fmt.Errorf("lista de migrações fora de ordem ou com versão duplicada: v%d (índice %d) não é maior que v%d (índice %d)",
				migrations[i].Version, i, migrations[i-1].Version, i-1)
		}
	}
	if err := ensureSchemaMigrationsTable(database); err != nil {
		return err
	}
	applied, err := appliedMigrationVersions(database)
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if m.Phase != phase {
			continue
		}
		if applied[m.Version] {
			continue
		}

		if m.DetectApplied != nil {
			alreadyApplied, derr := m.DetectApplied(database)
			if derr != nil {
				return fmt.Errorf("detectar estado da migração %d (%s): %w", m.Version, m.Name, derr)
			}
			if alreadyApplied {
				if rerr := recordMigration(database, m); rerr != nil {
					return rerr
				}
				logging.Infof(context.Background(), "database.migrator", "[Migration] v%d %q já presente em banco legado — marcada como aplicada sem reexecutar", m.Version, m.Name)
				continue
			}
		}

		if m.Run != nil {
			if rerr := m.Run(database); rerr != nil {
				// Adiamento: efeito incompleto mas seguro para retentar. NÃO
				// registra nem aborta o boot — a migração roda de novo no
				// próximo startup. Versões seguintes ainda rodam (continue), o
				// que pode deixar um buraco no prefixo registrado; por isso
				// user_version reflete a maior versão CONTÍGUA (ver
				// syncUserVersion), não saltando à frente da pendente.
				if errors.Is(rerr, errMigrationDeferred) {
					logging.Warnf(context.Background(), "database.migrator", "[Migration] v%d %q adiada — será retentada no próximo boot: %v", m.Version, m.Name, rerr)
					continue
				}
				return fmt.Errorf("aplicar migração %d (%s): %w", m.Version, m.Name, rerr)
			}
		}
		if rerr := recordMigration(database, m); rerr != nil {
			return rerr
		}
		logging.Infof(context.Background(), "database.migrator", "[Migration] v%d %q aplicada", m.Version, m.Name)
	}

	return nil
}

// ensureSchemaMigrationsTable cria a tabela de controle de versões, se ausente.
func ensureSchemaMigrationsTable(database *gorm.DB) error {
	if err := database.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		name       TEXT NOT NULL,
		applied_at DATETIME NOT NULL
	)`).Error; err != nil {
		return fmt.Errorf("criar tabela schema_migrations: %w", err)
	}
	return nil
}

// appliedMigrationVersions lê o conjunto de versões já aplicadas.
func appliedMigrationVersions(database *gorm.DB) (map[int]bool, error) {
	var versions []int
	if err := database.Raw(`SELECT version FROM schema_migrations`).Scan(&versions).Error; err != nil {
		return nil, fmt.Errorf("ler schema_migrations: %w", err)
	}
	set := make(map[int]bool, len(versions))
	for _, v := range versions {
		set[v] = true
	}
	return set, nil
}

// recordMigration registra a migração como aplicada e espelha a maior versão
// CONTÍGUA em PRAGMA user_version (inspeção rápida via `sqlite3 ... 'PRAGMA
// user_version'` sem precisar ler a tabela). INSERT OR IGNORE torna o registro
// idempotente caso a versão já exista.
func recordMigration(database *gorm.DB, m migration) error {
	if err := database.Exec(
		`INSERT OR IGNORE INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
		m.Version, m.Name, time.Now().UTC(),
	).Error; err != nil {
		return fmt.Errorf("registrar migração %d (%s): %w", m.Version, m.Name, err)
	}
	return syncUserVersion(database)
}

// syncUserVersion ajusta PRAGMA user_version para a maior versão CONTÍGUA
// aplicada (o maior N tal que 1..N estão todos registrados em
// schema_migrations, sem buracos). user_version é um espelho informativo; a
// fonte de verdade é a tabela schema_migrations.
//
// Por que contíguo (e não MAX): uma migração adiada (errMigrationDeferred) não
// é registrada, mas as versões seguintes seguem rodando e SÃO registradas — o
// que pode deixar um buraco (ex.: v4 registrada com v3 pendente). Espelhar
// MAX(version) faria user_version "pular" à frente de uma migração anterior
// ainda pendente, enfraquecendo a leitura rápida de "schema está na versão N".
// Refletindo apenas o prefixo contíguo, user_version trava na maior versão sem
// buraco, sinalizando que há migração anterior pendente; ele avança assim que o
// adiamento é resolvido no próximo boot. Buracos permanecem visíveis (e
// auditáveis) na tabela schema_migrations.
func syncUserVersion(database *gorm.DB) error {
	var versions []int
	if err := database.Raw(`SELECT version FROM schema_migrations ORDER BY version`).Scan(&versions).Error; err != nil {
		return fmt.Errorf("ler versões aplicadas: %w", err)
	}
	contiguous := 0
	for _, v := range versions {
		if v == contiguous+1 {
			contiguous = v
		} else if v > contiguous+1 {
			break // buraco: para no fim do prefixo contíguo
		}
	}
	// user_version não aceita placeholder; contiguous é int controlado por nós,
	// então a interpolação é segura.
	if err := database.Exec(fmt.Sprintf("PRAGMA user_version = %d", contiguous)).Error; err != nil {
		return fmt.Errorf("atualizar user_version: %w", err)
	}
	return nil
}

// uuidMigrationAlreadyApplied detecta se um banco legado já está em UUIDv7,
// permitindo marcar a migração v1 como aplicada sem reexecutar a reescrita de
// tabelas. Retorna:
//   - false quando a tabela `conversations` não existe (banco novo): deixa o
//     Run rodar — a conversão é no-op nesse caso e registra a versão;
//   - false quando `conversations.id` ainda é INTEGER (banco legado por
//     migrar): o Run executa a conversão real;
//   - true quando `conversations.id` já é TEXT (banco já migrado): marca como
//     aplicada sem trabalho pesado.
func uuidMigrationAlreadyApplied(database *gorm.DB) (bool, error) {
	sqlDB, err := database.DB()
	if err != nil {
		return false, err
	}
	var count int
	if err := sqlDB.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='conversations'`).Scan(&count); err != nil {
		return false, err
	}
	if count == 0 {
		return false, nil
	}
	needed, err := isUUIDMigrationNeeded(sqlDB)
	if err != nil {
		return false, err
	}
	return !needed, nil
}
