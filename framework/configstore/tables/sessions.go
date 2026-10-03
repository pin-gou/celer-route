package tables

import (
	"fmt"
	"time"

	"github.com/pin-gou/celer-route/framework/encrypt"
	"gorm.io/gorm"
)

// SessionKind distinguishes which auth path minted a sessions-table row. The
// admin and member logins share the sessions table (the member cookie carries
// "<user_id>:<random>" while the table stores only the random half, hashed for
// lookup). Marking the kind lets the admin validation path refuse member-issued
// tokens and the member path refuse admin tokens, closing the cross-auth
// privilege escalation where a leaked member session token's random half would
// otherwise pass the admin validateSession lookup unchanged.
const (
	SessionKindAdmin  = "admin"
	SessionKindMember = "member"
)

// SessionsTable represents a session in the database
type SessionsTable struct {
	ID               int       `gorm:"primaryKey;autoIncrement" json:"id"`
	Token            string    `gorm:"type:text;not null;uniqueIndex" json:"token"`
	ExpiresAt        time.Time `gorm:"index;not null" json:"expires_at,omitempty"`
	CreatedAt        time.Time `gorm:"index;not null" json:"created_at"`
	UpdatedAt        time.Time `gorm:"index;not null" json:"updated_at"`
	EncryptionStatus string    `gorm:"type:varchar(20);default:'plain_text'" json:"-"`
	TokenHash        string    `gorm:"type:varchar(64);index:idx_session_token_hash,unique" json:"-"`
	Kind             string    `gorm:"type:varchar(20);default:'admin';index" json:"-"`
}

// TableName sets the table name for each model
func (SessionsTable) TableName() string { return "sessions" }

// BeforeSave hook to hash and encrypt the session token
func (s *SessionsTable) BeforeSave(tx *gorm.DB) error {
	// Hash must be computed before encryption (from plaintext value)
	if s.Token != "" {
		s.TokenHash = encrypt.HashSHA256(s.Token)
	}
	if encrypt.IsEnabled() && s.Token != "" {
		if err := encryptString(&s.Token); err != nil {
			return fmt.Errorf("failed to encrypt session token: %w", err)
		}
		s.EncryptionStatus = EncryptionStatusEncrypted
	}
	return nil
}

// AfterFind hook to decrypt the session token
func (s *SessionsTable) AfterFind(tx *gorm.DB) error {
	if s.EncryptionStatus == EncryptionStatusEncrypted {
		if err := decryptString(&s.Token); err != nil {
			return fmt.Errorf("failed to decrypt session token: %w", err)
		}
	}
	return nil
}
