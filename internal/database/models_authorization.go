package database

import "time"

// AuthorizationRule é uma decisão persistente. O seletor é validado pelo
// adaptador do domínio antes de entrar no repositório (AEP-0117).
type AuthorizationRule struct {
	UUIDModel
	UserID          string `gorm:"not null;uniqueIndex:idx_authorization_rule_key,priority:1;index:idx_authorization_scope,priority:1"`
	Domain          string `gorm:"not null;uniqueIndex:idx_authorization_rule_key,priority:2;index:idx_authorization_scope,priority:2"`
	Scope           string `gorm:"not null;uniqueIndex:idx_authorization_rule_key,priority:3;index:idx_authorization_scope,priority:3"`
	ScopeKey        string `gorm:"not null;uniqueIndex:idx_authorization_rule_key,priority:4;index:idx_authorization_scope,priority:4"`
	SelectorKey     string `gorm:"not null;uniqueIndex:idx_authorization_rule_key,priority:5"`
	Selector        string `gorm:"not null;type:text"`
	SelectorVersion int    `gorm:"not null"`
	Effect          string `gorm:"not null"`
	Operation       string
	Origin          string `gorm:"not null"`
	PolicyID        string `gorm:"index"`
	GrantedBy       string
	RevokedAt       *time.Time
	ExpiresAt       *time.Time
}

// AuthorizationImport impede que arquivos antigos ressuscitem regras revogadas.
type AuthorizationImport struct {
	UUIDModel
	UserID  string `gorm:"not null;uniqueIndex:idx_authorization_import,priority:1"`
	Domain  string `gorm:"not null;uniqueIndex:idx_authorization_import,priority:2"`
	Source  string `gorm:"not null;uniqueIndex:idx_authorization_import,priority:3"`
	Digest  string `gorm:"not null"`
	Version int    `gorm:"not null"`
}

type AuthorizationPolicy struct {
	UUIDModel
	UserID        string `gorm:"not null;uniqueIndex:idx_authorization_policy,priority:1"`
	Domain        string `gorm:"not null;uniqueIndex:idx_authorization_policy,priority:2"`
	Slug          string `gorm:"not null;uniqueIndex:idx_authorization_policy,priority:3"`
	Source        string `gorm:"not null;uniqueIndex:idx_authorization_policy,priority:4"`
	Name          string
	Description   string
	DefaultEffect string `gorm:"not null"`
	Revision      uint64 `gorm:"not null"`
}

type AuthorizationPolicyBinding struct {
	UUIDModel
	UserID   string `gorm:"not null;uniqueIndex:idx_authorization_binding,priority:1"`
	PolicyID string `gorm:"not null;uniqueIndex:idx_authorization_binding,priority:2"`
	Scope    string `gorm:"not null;uniqueIndex:idx_authorization_binding,priority:3"`
	ScopeKey string `gorm:"not null;uniqueIndex:idx_authorization_binding,priority:4"`
}
