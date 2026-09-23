package database

import "time"

// ExternalIdentityAdminAudit registra decisões administrativas usadas no
// bootstrap legado e na criação de vínculos externos. A gravação deste model
// deve ser a primeira escrita da mesma transação que altera o mapeamento.
type ExternalIdentityAdminAudit struct {
	ID           string    `gorm:"type:text;primaryKey" json:"id"`
	Issuer       string    `gorm:"not null;type:text" json:"issuer"`
	ActorSubject string    `gorm:"not null;type:text" json:"actorSubject"`
	ActorUserID  string    `gorm:"not null;type:text" json:"actorUserId"`
	Action       string    `gorm:"not null;type:text" json:"action"`
	Subject      string    `gorm:"not null;type:text" json:"subject"`
	UserID       string    `gorm:"not null;type:text" json:"userId"`
	CreatedAt    time.Time `gorm:"not null" json:"createdAt"`
}

func (ExternalIdentityAdminAudit) TableName() string {
	return "external_identity_admin_audits"
}
