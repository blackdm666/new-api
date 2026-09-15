package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var ErrInvoiceFileNotFound = errors.New("invoice file not found")

var (
	ErrInvoiceFileLimit            = errors.New("invoice file limit reached")
	ErrInvoiceFileMutationRejected = errors.New("rejected invoice files are immutable")
	ErrInvoiceFileMutationArchived = errors.New("archived invoice files are immutable")
	ErrInvoiceFileMutationFinal    = errors.New("final invoice files are immutable")
	ErrInvoiceIssuedFileRequired   = errors.New("issued invoice must keep at least one file")
)

type InvoiceFile struct {
	Id               int            `json:"id"`
	InvoiceRequestId int            `json:"invoice_request_id" gorm:"index;not null"`
	UploaderId       int            `json:"uploader_id" gorm:"index;not null"`
	FileName         string         `json:"file_name" gorm:"type:varchar(255);not null"`
	StoredName       string         `json:"-" gorm:"type:varchar(128);not null"`
	MimeType         string         `json:"mime_type" gorm:"type:varchar(128);not null"`
	Size             int64          `json:"size" gorm:"type:bigint;default:0"`
	StorageProfileId int            `json:"storage_profile_id" gorm:"index;not null;default:0"`
	StorageType      string         `json:"storage_type" gorm:"type:varchar(16);default:'local'"`
	StorageKey       string         `json:"-" gorm:"type:varchar(512);not null"`
	Sha256           string         `json:"sha256" gorm:"type:varchar(64);index"`
	CreatedTime      int64          `json:"created_time" gorm:"bigint;index"`
	DeletedAt        gorm.DeletedAt `json:"-" gorm:"index"`
}

type InvoiceFileCleanup struct {
	Id               int    `json:"id"`
	StorageProfileId int    `json:"storage_profile_id" gorm:"index;not null;default:0;uniqueIndex:idx_invoice_cleanup_profile_key"`
	StorageType      string `json:"storage_type" gorm:"type:varchar(16);not null"`
	StorageKey       string `json:"storage_key" gorm:"type:varchar(512);not null;uniqueIndex:idx_invoice_cleanup_profile_key"`
	Attempts         int    `json:"attempts" gorm:"type:int;default:0"`
	LastError        string `json:"last_error" gorm:"type:text"`
	NextAttemptTime  int64  `json:"next_attempt_time" gorm:"bigint;index"`
	LockedUntil      int64  `json:"locked_until" gorm:"bigint;index"`
	CreatedTime      int64  `json:"created_time" gorm:"bigint;index"`
}

func (file *InvoiceFile) BeforeCreate(tx *gorm.DB) error {
	if file.CreatedTime == 0 {
		file.CreatedTime = common.GetTimestamp()
	}
	return nil
}

func CreateInvoiceFile(file *InvoiceFile) error {
	return DB.Create(file).Error
}

func CreateInvoiceFileWithinLimit(file *InvoiceFile, maxCount int) error {
	if file == nil || maxCount <= 0 {
		return ErrInvoiceFileLimit
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var request InvoiceRequest
		if err := lockForUpdate(tx).First(&request, "id = ?", file.InvoiceRequestId).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvoiceRequestNotFound
			}
			return err
		}
		if request.RedactedTime != 0 {
			return ErrInvoiceFileMutationArchived
		}
		if request.Status == InvoiceStatusRejected {
			return ErrInvoiceFileMutationRejected
		}
		if request.Status != InvoiceStatusPending {
			return ErrInvoiceFileMutationFinal
		}
		var count int64
		if err := tx.Model(&InvoiceFile{}).Where("invoice_request_id = ?", request.Id).Count(&count).Error; err != nil {
			return err
		}
		if count >= int64(maxCount) {
			return ErrInvoiceFileLimit
		}
		if err := tx.Create(file).Error; err != nil {
			return err
		}
		return tx.Model(&InvoiceRequest{}).Where("id = ?", request.Id).
			Update("updated_time", common.GetTimestamp()).Error
	})
}

func GetInvoiceFileById(id int) (*InvoiceFile, error) {
	var file InvoiceFile
	if err := DB.First(&file, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvoiceFileNotFound
		}
		return nil, err
	}
	return &file, nil
}

func ListInvoiceFiles(requestId int) ([]*InvoiceFile, error) {
	var files []*InvoiceFile
	err := DB.Where("invoice_request_id = ?", requestId).Order("id ASC").Find(&files).Error
	return files, err
}

func CountInvoiceFiles(requestId int) (int64, error) {
	var count int64
	err := DB.Model(&InvoiceFile{}).Where("invoice_request_id = ?", requestId).Count(&count).Error
	return count, err
}

func DeleteInvoiceFile(id int) error {
	return DB.Delete(&InvoiceFile{}, id).Error
}

func QueueInvoiceFileDeletion(requestId int, fileId int) (*InvoiceFileCleanup, error) {
	var cleanup *InvoiceFileCleanup
	err := DB.Transaction(func(tx *gorm.DB) error {
		var request InvoiceRequest
		if err := lockForUpdate(tx).First(&request, "id = ?", requestId).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvoiceRequestNotFound
			}
			return err
		}
		if request.RedactedTime != 0 {
			return ErrInvoiceFileMutationArchived
		}
		if request.Status == InvoiceStatusRejected {
			return ErrInvoiceFileMutationRejected
		}
		if request.Status != InvoiceStatusPending {
			return ErrInvoiceFileMutationFinal
		}
		var file InvoiceFile
		if err := lockForUpdate(tx).First(&file, "id = ? AND invoice_request_id = ?", fileId, requestId).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvoiceFileNotFound
			}
			return err
		}
		now := common.GetTimestamp()
		cleanup = &InvoiceFileCleanup{
			StorageProfileId: file.StorageProfileId,
			StorageType:      file.StorageType,
			StorageKey:       file.StorageKey,
			NextAttemptTime:  now,
			CreatedTime:      now,
		}
		if err := tx.Create(cleanup).Error; err != nil {
			return err
		}
		if err := tx.Delete(&file).Error; err != nil {
			return err
		}
		return tx.Model(&InvoiceRequest{}).Where("id = ?", requestId).
			Update("updated_time", now).Error
	})
	return cleanup, err
}

func EnqueueInvoiceFileCleanup(storageProfileId int, storageType string, storageKey string, lastError string) error {
	now := common.GetTimestamp()
	cleanup := &InvoiceFileCleanup{
		StorageProfileId: storageProfileId,
		StorageType:      storageType,
		StorageKey:       storageKey,
		LastError:        lastError,
		NextAttemptTime:  now,
		CreatedTime:      now,
	}
	return DB.Where("storage_profile_id = ? AND storage_key = ?", storageProfileId, storageKey).FirstOrCreate(cleanup).Error
}

func ListPendingInvoiceFileCleanups(limit int, now int64) ([]*InvoiceFileCleanup, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	var cleanups []*InvoiceFileCleanup
	err := DB.Where("next_attempt_time <= ? AND locked_until <= ?", now, now).Order("id ASC").Limit(limit).Find(&cleanups).Error
	return cleanups, err
}

func ClaimInvoiceFileCleanup(id int, now int64, lockedUntil int64) (bool, error) {
	result := DB.Model(&InvoiceFileCleanup{}).
		Where("id = ? AND next_attempt_time <= ? AND locked_until <= ?", id, now, now).
		Update("locked_until", lockedUntil)
	return result.RowsAffected == 1, result.Error
}

func CompleteInvoiceFileCleanup(id int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var cleanup InvoiceFileCleanup
		if err := lockForUpdate(tx).First(&cleanup, "id = ?", id).Error; err != nil {
			return err
		}
		// The soft-deleted row is retained only while physical deletion is
		// retryable. Once the object is gone, remove that metadata permanently
		// so purged applications cannot leave database orphans behind.
		if err := tx.Unscoped().Where(
			"storage_profile_id = ? AND storage_key = ? AND deleted_at IS NOT NULL",
			cleanup.StorageProfileId,
			cleanup.StorageKey,
		).Delete(&InvoiceFile{}).Error; err != nil {
			return err
		}
		return tx.Delete(&cleanup).Error
	})
}

func RecordInvoiceFileCleanupFailure(id int, message string, nextAttemptTime int64) error {
	return DB.Model(&InvoiceFileCleanup{}).Where("id = ?", id).Updates(map[string]interface{}{
		"attempts":          gorm.Expr("attempts + ?", 1),
		"last_error":        message,
		"next_attempt_time": nextAttemptTime,
		"locked_until":      int64(0),
	}).Error
}

func ListInvoiceFileCleanups(limit int) ([]*InvoiceFileCleanup, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	var cleanups []*InvoiceFileCleanup
	err := DB.Order("created_time DESC, id DESC").Limit(limit).Find(&cleanups).Error
	return cleanups, err
}

func RetryInvoiceFileCleanup(id int) (*InvoiceFileCleanup, error) {
	now := common.GetTimestamp()
	result := DB.Model(&InvoiceFileCleanup{}).Where("id = ?", id).Updates(map[string]interface{}{
		"attempts":          0,
		"last_error":        "",
		"next_attempt_time": now,
		"locked_until":      int64(0),
	})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var cleanup InvoiceFileCleanup
	if err := DB.First(&cleanup, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &cleanup, nil
}

func ListInvoiceFileStorageKeys(profileId int) ([]string, error) {
	var keys []string
	if err := DB.Model(&InvoiceFile{}).Where("storage_profile_id = ?", profileId).Pluck("storage_key", &keys).Error; err != nil {
		return nil, err
	}
	var cleanupKeys []string
	if err := DB.Model(&InvoiceFileCleanup{}).Where("storage_profile_id = ?", profileId).Pluck("storage_key", &cleanupKeys).Error; err != nil {
		return nil, err
	}
	var uploadKeys []string
	if err := DB.Model(&InvoiceFileUpload{}).Where("storage_profile_id = ?", profileId).Pluck("storage_key", &uploadKeys).Error; err != nil {
		return nil, err
	}
	return append(append(keys, cleanupKeys...), uploadKeys...), nil
}

func ListInvoiceFilesByProfile(profileId int, afterId int, limit int) ([]*InvoiceFile, error) {
	if limit <= 0 || limit > 5000 {
		limit = 2000
	}
	var files []*InvoiceFile
	err := DB.Where("storage_profile_id = ? AND id > ?", profileId, afterId).Order("id ASC").Limit(limit).Find(&files).Error
	return files, err
}

func QueueInvoiceOrphanCleanup(profileId int, storageType string, storageKey string) (*InvoiceFileCleanup, error) {
	storageKey = strings.TrimSpace(storageKey)
	if profileId <= 0 || storageKey == "" {
		return nil, gorm.ErrInvalidData
	}
	var cleanup *InvoiceFileCleanup
	err := DB.Transaction(func(tx *gorm.DB) error {
		queries := []interface{}{&InvoiceFile{}, &InvoiceFileUpload{}}
		for _, target := range queries {
			var count int64
			query := tx.Model(target).Where("storage_profile_id = ? AND storage_key = ?", profileId, storageKey)
			if err := query.Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return errors.New("invoice storage key is still referenced")
			}
		}
		var existing InvoiceFileCleanup
		if err := tx.Where("storage_profile_id = ? AND storage_key = ?", profileId, storageKey).First(&existing).Error; err == nil {
			cleanup = &existing
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		now := common.GetTimestamp()
		cleanup = &InvoiceFileCleanup{
			StorageProfileId: profileId,
			StorageType:      storageType,
			StorageKey:       storageKey,
			NextAttemptTime:  now,
			CreatedTime:      now,
		}
		return tx.Create(cleanup).Error
	})
	return cleanup, err
}
