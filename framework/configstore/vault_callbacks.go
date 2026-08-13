package configstore

import (
	"gorm.io/gorm"
)

// RegisterVaultCallbacks is a no-op since vault functionality has been removed.
func RegisterVaultCallbacks(_ *gorm.DB) {}