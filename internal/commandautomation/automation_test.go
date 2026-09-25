package commandautomation

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"assistente/internal/commanddecision"
	"assistente/internal/commandsecurity"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type acceptPresenter struct{}

func (acceptPresenter) Present(_ context.Context, r commanddecision.Request) (commanddecision.Response, error) {
	return commanddecision.Response{DecisionID: r.DecisionID, ActionID: commanddecision.ApplyAction}, nil
}

type automationFixture struct {
	t        *testing.T
	db       *gorm.DB
	store    *Store
	receipts *commanddecision.Store
	owner    Owner
	rule     Rule
	epoch    commandsecurity.EpochSnapshot
	now      time.Time
	clock    *time.Time
}

func uuid7(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func newAutomationFixture(t *testing.T) *automationFixture {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Millisecond)
	path := filepath.Join(t.TempDir(), "automation.db")
	db, err := gorm.Open(sqlite.Open("file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode=WAL"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(16)
	t.Cleanup(func() { _ = sqlDB.Close() })
	ctx := context.Background()
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := commanddecision.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&ruleRow{}); err != nil {
		t.Fatal(err)
	}
	clock := now
	store, err := New(db, func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}
	receipts, err := commanddecision.New(db, acceptPresenter{}, func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}
	owner := Owner{UserID: uuid7(t)}
	rule := Rule{ID: uuid7(t), Owner: owner, LayerRef: RuleRef{Kind: "builtin", Ref: "layer:execution"}, RuleRef: RuleRef{Kind: "user", Ref: ""}, Mode: "event", Condition: `{"state":"running"}`, Lifecycle: "persistent", EventName: JobRunStateEvent, AllowedInternalProducerTypes: []string{JobsRuntime}, Enabled: false, Source: "user"}
	rule.RuleRef.Ref = rule.ID
	rule.ReviewStatus = "active"
	row := ruleRow{ID: rule.ID, UserID: owner.UserID, LayerRefKind: rule.LayerRef.Kind, LayerRef: rule.LayerRef.Ref, RuleRefKind: rule.RuleRef.Kind, RuleRef: rule.RuleRef.Ref, Mode: rule.Mode, Condition: rule.Condition, Lifecycle: rule.Lifecycle, EventName: rule.EventName, AllowedProducerTypes: `["jobs.runtime"]`, Enabled: false, Source: rule.Source, ReviewStatus: rule.ReviewStatus}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	return &automationFixture{t: t, db: db, store: store, receipts: receipts, owner: owner, rule: rule, epoch: commandsecurity.EpochSnapshot{UserID: owner.UserID, SessionID: uuid7(t), AuthGeneration: "auth-1", SecurityGeneration: "security-1"}, now: now, clock: &clock}
}

func fixtureKeys(context.Context, string) ([]byte, error) {
	return []byte("automation-fixture-key-012345678901234567890123"), nil
}

func (f *automationFixture) confirmed(t *testing.T) *ConfirmedGrantChange {
	t.Helper()
	change, err := f.store.Prepare(context.Background(), f.owner, f.rule.RuleRef)
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := f.store.ConfirmGrant(context.Background(), change, f.epoch, f.receipts, "v1", fixtureKeys, f.now.Add(time.Minute), func(Rule) (string, error) { return "confirmar ativação", nil })
	if err != nil {
		t.Fatal(err)
	}
	return confirmed
}

func TestConfirmReceiptAndCommitGrantUpdatesRuleAtomically(t *testing.T) {
	f := newAutomationFixture(t)
	confirmed := f.confirmed(t)
	if err := f.store.CommitConfirmedGrant(context.Background(), confirmed, f.epoch); err != nil {
		t.Fatal(err)
	}
	grant, err := f.store.LoadActive(context.Background(), f.owner, keyFromRule(f.rule))
	if err != nil || grant.AutomationGrantGeneration != 1 || grant.AuthorizationDecisionID != confirmed.request.DecisionID {
		t.Fatalf("grant não persistido: grant=%+v err=%v", grant, err)
	}
	var rule ruleRow
	if err := f.db.Take(&rule, "id = ?", f.rule.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !rule.Enabled || rule.AutomationGrantID == nil || *rule.AutomationGrantID != grant.ID {
		t.Fatalf("regra não vinculada ao grant: %+v", rule)
	}
	var status string
	if err := f.db.Table("command_decision_receipts").Select("status").Where("decision_id = ?", confirmed.request.DecisionID).Scan(&status).Error; err != nil {
		t.Fatal(err)
	}
	if status != commanddecision.Consumed {
		t.Fatalf("receipt = %q, esperado consumed", status)
	}
}

func TestMigrateIsExplicitAndCreatesExactActiveScopeIndexes(t *testing.T) {
	f := newAutomationFixture(t)
	if err := Migrate(context.Background(), f.db); err != nil {
		t.Fatal(err)
	}
	var indexes []struct {
		Name string
		SQL  string
	}
	if err := f.db.Raw("SELECT name, sql FROM sqlite_master WHERE type = 'index' AND tbl_name = ? AND name LIKE 'ux_command_layer_automation_grants_%' ORDER BY name", "command_layer_automation_grants").Scan(&indexes).Error; err != nil {
		t.Fatal(err)
	}
	if len(indexes) != 4 {
		t.Fatalf("índices = %d, esperado 4: %+v", len(indexes), indexes)
	}
	for _, index := range indexes {
		if index.Name == "ux_command_layer_automation_grants_active_global" && index.SQL == "" {
			t.Fatal("índice global sem definição")
		}
	}
	var tables int64
	if err := f.db.Raw("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='command_layer_activation_rules'").Scan(&tables).Error; err != nil {
		t.Fatal(err)
	}
	if tables != 1 {
		t.Fatal("o pacote deve consumir a tabela de regras, não criá-la")
	}
}

func TestCommitReplayDoesNotConsumeReceiptOrCreateSecondGrant(t *testing.T) {
	f := newAutomationFixture(t)
	confirmed := f.confirmed(t)
	if err := f.store.CommitConfirmedGrant(context.Background(), confirmed, f.epoch); err != nil {
		t.Fatal(err)
	}
	if err := f.store.CommitConfirmedGrant(context.Background(), confirmed, f.epoch); !errors.Is(err, commanddecision.ErrStale) {
		t.Fatalf("replay = %v, esperado receipt stale", err)
	}
	var count int64
	if err := f.db.Model(&grantRow{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("grants após replay = %d err=%v", count, err)
	}
}

func TestConfirmForeignScopeAndOwnerAreRejected(t *testing.T) {
	f := newAutomationFixture(t)
	foreign := Owner{UserID: uuid7(t)}
	if _, err := f.store.Prepare(context.Background(), foreign, f.rule.RuleRef); !errors.Is(err, ErrNotFound) {
		t.Fatalf("owner estrangeiro = %v, esperado not found escopado", err)
	}
	workspace := uuid7(t)
	if _, err := f.store.Prepare(context.Background(), Owner{UserID: f.owner.UserID, WorkspaceID: &workspace}, f.rule.RuleRef); !errors.Is(err, ErrNotFound) {
		t.Fatalf("workspace estrangeiro = %v, esperado not found escopado", err)
	}
}

func TestCommitPolicyFailureRollsBackGrantRuleAndReceipt(t *testing.T) {
	f := newAutomationFixture(t)
	confirmed := f.confirmed(t)
	want := errors.New("política revogada")
	if err := f.store.CommitConfirmedGrant(context.Background(), confirmed, f.epoch, func(context.Context, *gorm.DB, Owner, Rule) error { return want }); !errors.Is(err, want) {
		t.Fatalf("política = %v, esperado %v", err, want)
	}
	var grants int64
	if err := f.db.Model(&grantRow{}).Count(&grants).Error; err != nil || grants != 0 {
		t.Fatalf("grant vazou: %d err=%v", grants, err)
	}
	var enabled bool
	if err := f.db.Table("command_layer_activation_rules").Select("enabled").Where("id = ?", f.rule.ID).Scan(&enabled).Error; err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("regra habilitada apesar do rollback")
	}
	var status string
	if err := f.db.Table("command_decision_receipts").Select("status").Where("decision_id = ?", confirmed.request.DecisionID).Scan(&status).Error; err != nil {
		t.Fatal(err)
	}
	if status != commanddecision.Accepted {
		t.Fatalf("receipt rollback = %q", status)
	}
}

func TestCommitRuleWriteFailureRollsBackInsertedGrantAndReceipt(t *testing.T) {
	f := newAutomationFixture(t)
	confirmed := f.confirmed(t)
	if err := f.db.Exec("CREATE TRIGGER automation_reject_rule_update BEFORE UPDATE ON command_layer_activation_rules BEGIN SELECT RAISE(ABORT, 'fixture rule failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := f.store.CommitConfirmedGrant(context.Background(), confirmed, f.epoch); err == nil {
		t.Fatal("update da regra deveria falhar")
	}
	var grants int64
	if err := f.db.Model(&grantRow{}).Count(&grants).Error; err != nil || grants != 0 {
		t.Fatalf("grant inserido vazou após falha da regra: %d err=%v", grants, err)
	}
	var status string
	if err := f.db.Table("command_decision_receipts").Select("status").Where("decision_id = ?", confirmed.request.DecisionID).Scan(&status).Error; err != nil {
		t.Fatal(err)
	}
	if status != commanddecision.Accepted {
		t.Fatalf("receipt não voltou para accepted: %q", status)
	}
}

func TestConcurrentConfirmedGrantHasSingleWinner(t *testing.T) {
	f := newAutomationFixture(t)
	first := f.confirmed(t)
	// O segundo preview é legítimo antes de qualquer CAS; somente um commit
	// poderá transformar a regra desabilitada em regra concedida.
	second := f.confirmed(t)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, proposal := range []*ConfirmedGrantChange{first, second} {
		wg.Add(1)
		go func(p *ConfirmedGrantChange) {
			defer wg.Done()
			<-start
			results <- f.store.CommitConfirmedGrant(context.Background(), p, f.epoch)
		}(proposal)
	}
	close(start)
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		} else if !errors.Is(err, commanddecision.ErrStale) && !errors.Is(err, ErrStale) {
			t.Fatalf("erro concorrente inesperado: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("vencedores = %d", wins)
	}
	var count int64
	if err := f.db.Model(&grantRow{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("grants concorrentes = %d err=%v", count, err)
	}
}

func TestRevokeTxKeepsHistoryAndRegrantUsesNextGeneration(t *testing.T) {
	f := newAutomationFixture(t)
	confirmed := f.confirmed(t)
	if err := f.store.CommitConfirmedGrant(context.Background(), confirmed, f.epoch); err != nil {
		t.Fatal(err)
	}
	key := keyFromRule(f.rule)
	grant, err := f.store.LoadActive(context.Background(), f.owner, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.Transaction(func(tx *gorm.DB) error {
		return f.store.RevokeTx(context.Background(), tx, f.owner, key, f.owner.UserID, "regra alterada")
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.LoadActive(context.Background(), f.owner, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("grant revogado ainda ativo: %v", err)
	}
	var old grantRow
	if err := f.db.Take(&old, "id = ?", grant.ID).Error; err != nil || old.RevokedAt == nil {
		t.Fatalf("histórico perdido: %+v err=%v", old, err)
	}
	if err := f.db.Model(&ruleRow{}).Where("id = ?", f.rule.ID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	second := f.confirmed(t)
	if err := f.store.CommitConfirmedGrant(context.Background(), second, f.epoch); err != nil {
		t.Fatal(err)
	}
	active, err := f.store.LoadActive(context.Background(), f.owner, key)
	if err != nil || active.AutomationGrantGeneration != 2 {
		t.Fatalf("regrant não avançou geração: grant=%+v err=%v", active, err)
	}
	var total int64
	if err := f.db.Model(&grantRow{}).Count(&total).Error; err != nil || total != 2 {
		t.Fatalf("histórico de revoke/regrant = %d err=%v", total, err)
	}
}

func TestRevokeLayerTxIsScopedAndDoesNotOpenTransaction(t *testing.T) {
	f := newAutomationFixture(t)
	confirmed := f.confirmed(t)
	if err := f.store.CommitConfirmedGrant(context.Background(), confirmed, f.epoch); err != nil {
		t.Fatal(err)
	}
	otherLayer := RuleRef{Kind: "builtin", Ref: "layer:other"}
	if err := f.db.Transaction(func(tx *gorm.DB) error {
		return f.store.RevokeLayerTx(context.Background(), tx, f.owner, f.rule.LayerRef, f.owner.UserID, "layer disabled")
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.LoadActive(context.Background(), f.owner, keyFromRule(f.rule)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("grant da layer não revogado: %v", err)
	}
	if err := f.db.Transaction(func(tx *gorm.DB) error {
		return RevokeLayerTx(context.Background(), tx, f.owner, otherLayer, f.owner.UserID, "sem efeito")
	}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateTxRequiresExactReferenceAndIsolatesJobsByScope(t *testing.T) {
	f := newAutomationFixture(t)
	confirmed := f.confirmed(t)
	if err := f.store.CommitConfirmedGrant(context.Background(), confirmed, f.epoch); err != nil {
		t.Fatal(err)
	}
	grant, err := f.store.LoadActive(context.Background(), f.owner, keyFromRule(f.rule))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.Transaction(func(tx *gorm.DB) error {
		got, err := ValidateTx(context.Background(), tx, f.owner, keyFromRule(f.rule), GrantReference{ID: grant.ID, Generation: grant.AutomationGrantGeneration, Fingerprint: grant.AutomationGrantFingerprint})
		if err != nil || got.ID != grant.ID {
			return errors.New("validação exata falhou")
		}
		_, err = ValidateTx(context.Background(), tx, f.owner, keyFromRule(f.rule), GrantReference{ID: grant.ID, Generation: 2, Fingerprint: grant.AutomationGrantFingerprint})
		return err
	}); !errors.Is(err, ErrStale) {
		t.Fatalf("referência divergente = %v", err)
	}
	other := Owner{UserID: uuid7(t)}
	if _, err := f.store.LoadActive(context.Background(), other, NaturalKey{Owner: other, LayerRef: f.rule.LayerRef, RuleRef: f.rule.RuleRef}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("isolamento de jobs/owner = %v", err)
	}
}
