package gateshare

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"time"

	"github.com/cocofhu/approving/internal/models"

	"gorm.io/gorm"
)

const (
	nonceTTL         = 15 * time.Minute
	maxNoncesPerLink = 4 // multi-tab preview: keep the last N unused nonces
)

// NonceStore issues one-time preview nonces keyed by token hash (never plaintext).
// Persistence uses the GateShare database so multi-replica preview→decide share
// the same bucket (see SECURITY.md).
type NonceStore struct {
	db *gorm.DB
}

// NewNonceStore builds a DB-backed nonce store on the GateShare database.
func NewNonceStore(db *gorm.DB) *NonceStore {
	return &NonceStore{db: db}
}

// Issue adds a nonce for tokenHash without invalidating prior unexpired ones
// (up to maxNoncesPerLink) so multiple tabs can submit after each previewed.
func (s *NonceStore) Issue(tokenHash string) (string, error) {
	if s == nil || s.db == nil {
		return "", errors.New("nonce store unavailable")
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	n := hex.EncodeToString(buf)
	now := time.Now()
	row := models.GateShareNonce{
		TokenHash: tokenHash,
		Nonce:     n,
		ExpiresAt: now.Add(nonceTTL),
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("token_hash = ? AND expires_at <= ?", tokenHash, now).
			Delete(&models.GateShareNonce{}).Error; err != nil {
			return err
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		var alive []models.GateShareNonce
		if err := tx.Where("token_hash = ?", tokenHash).Order("id ASC").Find(&alive).Error; err != nil {
			return err
		}
		if len(alive) <= maxNoncesPerLink {
			return nil
		}
		drop := alive[:len(alive)-maxNoncesPerLink]
		ids := make([]uint, len(drop))
		for i, r := range drop {
			ids[i] = r.ID
		}
		return tx.Where("id IN ?", ids).Delete(&models.GateShareNonce{}).Error
	})
	if err != nil {
		return "", err
	}
	return n, nil
}

// Consume validates and deletes a matching nonce. Constant-time compare per item.
func (s *NonceStore) Consume(tokenHash, nonce string) bool {
	if s == nil || s.db == nil {
		return false
	}
	nonce = hexNormalize(nonce)
	now := time.Now()
	var rows []models.GateShareNonce
	if err := s.db.Where("token_hash = ? AND expires_at > ?", tokenHash, now).Find(&rows).Error; err != nil {
		return false
	}
	want := []byte(nonce)
	var matchID uint
	for i := range rows {
		got := []byte(rows[i].Nonce)
		if len(got) == len(want) && subtle.ConstantTimeCompare(got, want) == 1 {
			matchID = rows[i].ID
			break
		}
	}
	if matchID == 0 {
		_ = s.db.Where("token_hash = ? AND expires_at <= ?", tokenHash, now).Delete(&models.GateShareNonce{}).Error
		return false
	}
	res := s.db.Where("id = ?", matchID).Delete(&models.GateShareNonce{})
	return res.Error == nil && res.RowsAffected == 1
}

func hexNormalize(s string) string {
	return s
}
