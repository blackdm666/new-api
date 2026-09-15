package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	InvoiceFileUploadPending = "pending"
)

type InvoiceStorageProfile struct {
	Id              int    `json:"id"`
	Fingerprint     string `json:"fingerprint" gorm:"type:varchar(64);uniqueIndex;not null"`
	StorageType     string `json:"storage_type" gorm:"type:varchar(16);index;not null"`
	EncryptedConfig string `json:"-" gorm:"type:text;not null"`
	CreatedTime     int64  `json:"created_time" gorm:"bigint;index"`
}

type InvoiceFileUpload struct {
	Id               string `json:"id" gorm:"type:varchar(64);primaryKey"`
	InvoiceRequestId int    `json:"invoice_request_id" gorm:"index;not null"`
	UploaderId       int    `json:"uploader_id" gorm:"index;not null"`
	StorageProfileId int    `json:"storage_profile_id" gorm:"index;not null"`
	StorageType      string `json:"storage_type" gorm:"type:varchar(16);not null"`
	StorageKey       string `json:"storage_key" gorm:"type:varchar(512);not null;uniqueIndex"`
	FileName         string `json:"file_name" gorm:"type:varchar(255);not null"`
	StoredName       string `json:"stored_name" gorm:"type:varchar(128);not null"`
	MimeType         string `json:"mime_type" gorm:"type:varchar(128);not null"`
	Size             int64  `json:"size" gorm:"type:bigint;default:0"`
	CreatedTime      int64  `json:"created_time" gorm:"bigint;index"`
}

func (profile *InvoiceStorageProfile) BeforeCreate(tx *gorm.DB) error {
	if profile.CreatedTime == 0 {
		profile.CreatedTime = common.GetTimestamp()
	}
	return nil
}

func GetOrCreateInvoiceStorageProfile(storageType string, fingerprint string, encryptedConfig string) (*InvoiceStorageProfile, error) {
	storageType = strings.TrimSpace(storageType)
	fingerprint = strings.TrimSpace(fingerprint)
	if storageType == "" || fingerprint == "" || encryptedConfig == "" {
		return nil, gorm.ErrInvalidData
	}
	profile := &InvoiceStorageProfile{
		StorageType:     storageType,
		Fingerprint:     fingerprint,
		EncryptedConfig: encryptedConfig,
	}
	result := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(profile)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 1 {
		return profile, nil
	}
	if err := DB.Where("fingerprint = ?", fingerprint).First(profile).Error; err != nil {
		return nil, err
	}
	return profile, nil
}

func GetInvoiceStorageProfile(id int) (*InvoiceStorageProfile, error) {
	if id <= 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var profile InvoiceStorageProfile
	err := DB.First(&profile, "id = ?", id).Error
	return &profile, err
}

func ListInvoiceStorageProfiles() ([]*InvoiceStorageProfile, error) {
	var profiles []*InvoiceStorageProfile
	err := DB.Order("id ASC").Find(&profiles).Error
	return profiles, err
}

func BackfillLegacyInvoiceStorageProfile(profileId int) error {
	if profileId <= 0 {
		return gorm.ErrInvalidData
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		models := []interface{}{&InvoiceFile{}, &InvoiceFileCleanup{}, &InvoiceFileUpload{}}
		for _, target := range models {
			if err := tx.Model(target).Where("storage_profile_id = 0").Update("storage_profile_id", profileId).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func CreateInvoiceFileUpload(upload *InvoiceFileUpload, maxCount int) error {
	if upload == nil || upload.Id == "" || upload.StorageProfileId <= 0 || maxCount <= 0 {
		return gorm.ErrInvalidData
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var request InvoiceRequest
		if err := lockForUpdate(tx).First(&request, "id = ?", upload.InvoiceRequestId).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvoiceRequestNotFound
			}
			return err
		}
		if request.RedactedTime != 0 {
			return ErrInvoiceFileMutationArchived
		}
		if request.Status != InvoiceStatusPending {
			return ErrInvoiceFileMutationFinal
		}
		var fileCount int64
		if err := tx.Model(&InvoiceFile{}).Where("invoice_request_id = ?", request.Id).Count(&fileCount).Error; err != nil {
			return err
		}
		var uploadCount int64
		if err := tx.Model(&InvoiceFileUpload{}).Where("invoice_request_id = ?", request.Id).Count(&uploadCount).Error; err != nil {
			return err
		}
		if fileCount+uploadCount >= int64(maxCount) {
			return ErrInvoiceFileLimit
		}
		if upload.CreatedTime == 0 {
			upload.CreatedTime = common.GetTimestamp()
		}
		return tx.Create(upload).Error
	})
}

func FinalizeInvoiceFileUpload(uploadId string, sha256Value string) (*InvoiceFile, error) {
	var file *InvoiceFile
	err := DB.Transaction(func(tx *gorm.DB) error {
		var upload InvoiceFileUpload
		if err := lockForUpdate(tx).First(&upload, "id = ?", uploadId).Error; err != nil {
			return err
		}
		var request InvoiceRequest
		if err := lockForUpdate(tx).First(&request, "id = ?", upload.InvoiceRequestId).Error; err != nil {
			return err
		}
		if request.RedactedTime != 0 {
			return ErrInvoiceFileMutationArchived
		}
		if request.Status != InvoiceStatusPending {
			return ErrInvoiceFileMutationFinal
		}
		file = &InvoiceFile{
			InvoiceRequestId: upload.InvoiceRequestId,
			UploaderId:       upload.UploaderId,
			FileName:         upload.FileName,
			StoredName:       upload.StoredName,
			MimeType:         upload.MimeType,
			Size:             upload.Size,
			StorageProfileId: upload.StorageProfileId,
			StorageType:      upload.StorageType,
			StorageKey:       upload.StorageKey,
			Sha256:           strings.TrimSpace(sha256Value),
		}
		if err := tx.Create(file).Error; err != nil {
			return err
		}
		if err := tx.Delete(&upload).Error; err != nil {
			return err
		}
		return tx.Model(&InvoiceRequest{}).Where("id = ?", request.Id).
			Update("updated_time", common.GetTimestamp()).Error
	})
	return file, err
}

func AbandonInvoiceFileUpload(uploadId string, lastError string) (*InvoiceFileCleanup, error) {
	var cleanup *InvoiceFileCleanup
	err := DB.Transaction(func(tx *gorm.DB) error {
		var upload InvoiceFileUpload
		if err := lockForUpdate(tx).First(&upload, "id = ?", uploadId).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		now := common.GetTimestamp()
		cleanup = &InvoiceFileCleanup{
			StorageProfileId: upload.StorageProfileId,
			StorageType:      upload.StorageType,
			StorageKey:       upload.StorageKey,
			LastError:        strings.TrimSpace(lastError),
			NextAttemptTime:  now,
			CreatedTime:      now,
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(cleanup).Error; err != nil {
			return err
		}
		return tx.Delete(&upload).Error
	})
	return cleanup, err
}

func ListStaleInvoiceFileUploads(cutoff int64, limit int) ([]*InvoiceFileUpload, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var uploads []*InvoiceFileUpload
	err := DB.Where("created_time <= ?", cutoff).Order("created_time ASC").Limit(limit).Find(&uploads).Error
	return uploads, err
}

func ListInvoiceFileUploads(limit int) ([]*InvoiceFileUpload, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var uploads []*InvoiceFileUpload
	err := DB.Order("created_time ASC").Limit(limit).Find(&uploads).Error
	return uploads, err
}
