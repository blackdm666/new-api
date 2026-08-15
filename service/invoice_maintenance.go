package service

import (
	"context"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/invoicefile"
)

type InvoiceStorageKey struct {
	StorageProfileId int    `json:"storage_profile_id"`
	StorageType      string `json:"storage_type"`
	StorageKey       string `json:"storage_key"`
}

type InvoiceStorageReconcileProfile struct {
	StorageProfileId int    `json:"storage_profile_id"`
	StorageType      string `json:"storage_type"`
	ObjectsScanned   int    `json:"objects_scanned"`
	Truncated        bool   `json:"truncated"`
	Error            string `json:"error,omitempty"`
}

type InvoiceStorageReconcileReport struct {
	Profiles     []InvoiceStorageReconcileProfile `json:"profiles"`
	OrphanKeys   []InvoiceStorageKey              `json:"orphan_keys"`
	MissingFiles []InvoiceStorageKey              `json:"missing_files"`
}

func ReconcileInvoiceStorage(ctx context.Context, limitPerProfile int) (*InvoiceStorageReconcileReport, error) {
	if limitPerProfile <= 0 || limitPerProfile > 5000 {
		limitPerProfile = 2000
	}
	profiles, err := model.ListInvoiceStorageProfiles()
	if err != nil {
		return nil, err
	}
	report := &InvoiceStorageReconcileReport{
		Profiles:     make([]InvoiceStorageReconcileProfile, 0, len(profiles)),
		OrphanKeys:   []InvoiceStorageKey{},
		MissingFiles: []InvoiceStorageKey{},
	}
	for _, profile := range profiles {
		profileReport := InvoiceStorageReconcileProfile{StorageProfileId: profile.Id, StorageType: profile.StorageType}
		storage, storageErr := invoicefile.ForProfile(profile.Id, profile.StorageType)
		if storageErr != nil {
			profileReport.Error = storageErr.Error()
			report.Profiles = append(report.Profiles, profileReport)
			continue
		}
		knownKeys, knownErr := model.ListInvoiceFileStorageKeys(profile.Id)
		if knownErr != nil {
			return nil, knownErr
		}
		known := make(map[string]struct{}, len(knownKeys))
		for _, key := range knownKeys {
			known[key] = struct{}{}
		}
		cursor := ""
		for {
			objects, nextCursor, listErr := storage.ListKeysPage(ctx, "", cursor, limitPerProfile)
			profileReport.ObjectsScanned += len(objects)
			if listErr != nil {
				profileReport.Error = listErr.Error()
				break
			}
			for _, key := range objects {
				if _, exists := known[key]; !exists {
					report.OrphanKeys = append(report.OrphanKeys, InvoiceStorageKey{
						StorageProfileId: profile.Id,
						StorageType:      profile.StorageType,
						StorageKey:       key,
					})
				}
			}
			if nextCursor == "" {
				break
			}
			if nextCursor == cursor {
				profileReport.Error = "storage pagination returned a repeated cursor"
				profileReport.Truncated = true
				break
			}
			cursor = nextCursor
		}
		lastFileID := 0
		for {
			files, filesErr := model.ListInvoiceFilesByProfile(profile.Id, lastFileID, limitPerProfile)
			if filesErr != nil {
				return nil, filesErr
			}
			for _, file := range files {
				lastFileID = file.Id
				exists, existsErr := storage.Exists(ctx, file.StorageKey)
				if existsErr != nil {
					if profileReport.Error == "" {
						profileReport.Error = existsErr.Error()
					}
					continue
				}
				if !exists {
					report.MissingFiles = append(report.MissingFiles, InvoiceStorageKey{
						StorageProfileId: profile.Id,
						StorageType:      profile.StorageType,
						StorageKey:       file.StorageKey,
					})
				}
			}
			if len(files) < limitPerProfile {
				break
			}
		}
		report.Profiles = append(report.Profiles, profileReport)
	}
	return report, nil
}

func QueueInvoiceOrphanCleanups(keys []InvoiceStorageKey) ([]*model.InvoiceFileCleanup, error) {
	cleanups := make([]*model.InvoiceFileCleanup, 0, len(keys))
	for _, key := range keys {
		cleanup, err := model.QueueInvoiceOrphanCleanup(key.StorageProfileId, key.StorageType, key.StorageKey)
		if err != nil {
			return cleanups, err
		}
		cleanups = append(cleanups, cleanup)
	}
	return cleanups, nil
}
