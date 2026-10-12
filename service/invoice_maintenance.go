package service

import (
	"context"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/invoicefile"
)

type InvoiceStorageReconcileProfile struct {
	StorageProfileId int    `json:"storage_profile_id"`
	StorageType      string `json:"storage_type"`
	ObjectsScanned   int    `json:"objects_scanned"`
	Truncated        bool   `json:"truncated"`
	Error            string `json:"error,omitempty"`
}

type InvoiceStorageReconcileReport struct {
	Profiles     []InvoiceStorageReconcileProfile `json:"profiles"`
	OrphanKeys   []model.InvoiceStorageKey        `json:"orphan_keys"`
	MissingFiles []model.InvoiceStorageKey        `json:"missing_files"`
}

// ReconcileInvoiceStorage compares each storage profile with the invoice
// metadata. Listing is limited to the invoice file key space and only keys
// with the exact invoice file layout can be reported as orphans, so unrelated
// objects in a shared bucket are neither listed broadly nor offered for
// deletion.
func ReconcileInvoiceStorage(ctx context.Context, limitPerProfile int) (*InvoiceStorageReconcileReport, error) {
	if limitPerProfile <= 0 || limitPerProfile > 5000 {
		limitPerProfile = 2000
	}
	profiles, err := model.ListInvoiceStorageProfiles()
	if err != nil {
		return nil, err
	}
	referencedKeys, err := model.ListReferencedInvoiceStorageKeys()
	if err != nil {
		return nil, err
	}
	referenced := make(map[string]struct{}, len(referencedKeys))
	for _, key := range referencedKeys {
		referenced[key] = struct{}{}
	}
	report := &InvoiceStorageReconcileReport{
		Profiles:     make([]InvoiceStorageReconcileProfile, 0, len(profiles)),
		OrphanKeys:   []model.InvoiceStorageKey{},
		MissingFiles: []model.InvoiceStorageKey{},
	}
	for _, profile := range profiles {
		profileReport := InvoiceStorageReconcileProfile{StorageProfileId: profile.Id, StorageType: profile.StorageType}
		storage, storageErr := invoicefile.ForProfile(profile.Id, profile.StorageType)
		if storageErr != nil {
			profileReport.Error = storageErr.Error()
			report.Profiles = append(report.Profiles, profileReport)
			continue
		}
	prefixes:
		for _, prefix := range model.InvoiceFileStorageListPrefixes() {
			cursor := ""
			for {
				objects, nextCursor, listErr := storage.ListKeysPage(ctx, prefix, cursor, limitPerProfile)
				profileReport.ObjectsScanned += len(objects)
				if listErr != nil {
					profileReport.Error = listErr.Error()
					break prefixes
				}
				for _, key := range objects {
					if !model.IsInvoiceFileStorageKey(key) {
						continue
					}
					if _, exists := referenced[key]; !exists {
						report.OrphanKeys = append(report.OrphanKeys, model.InvoiceStorageKey{
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
					break prefixes
				}
				cursor = nextCursor
			}
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
					report.MissingFiles = append(report.MissingFiles, model.InvoiceStorageKey{
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
