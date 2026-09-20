package service

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/invoicefile"
)

func InitializeInvoiceStorageProfiles() error {
	profiles, err := model.ListInvoiceStorageProfiles()
	if err != nil {
		return err
	}
	for _, existing := range profiles {
		if _, err := invoicefile.ConfigFromProfile(existing); err != nil {
			return err
		}
	}
	profile, err := invoicefile.EnsureCurrentProfile()
	if err != nil {
		return err
	}
	return model.BackfillLegacyInvoiceStorageProfile(profile.Id)
}
