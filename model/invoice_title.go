package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// 88api: saved invoice titles let users reuse their billing details.
const InvoiceTitleMaxPerUser = 20

var (
	ErrInvoiceTitleNotFound = errors.New("invoice title not found")
	ErrInvoiceTitleLimit    = errors.New("invoice title limit reached")
)

// InvoiceTitleDetails are the billing details printed on an invoice, shared
// by invoice requests and saved titles.
type InvoiceTitleDetails struct {
	Region         string
	CompanyName    string
	TaxNumber      string
	BankName       string
	BankAccount    string
	CompanyAddress string
	CompanyPhone   string
}

// Normalize trims and validates the details for their region. A mainland
// invoice needs the company name and tax number; a Hong Kong invoice carries
// only the recipient name, so every other field is dropped.
func (details InvoiceTitleDetails) Normalize() (InvoiceTitleDetails, error) {
	normalized := InvoiceTitleDetails{
		Region:      strings.TrimSpace(details.Region),
		CompanyName: strings.TrimSpace(details.CompanyName),
	}
	if normalized.Region == "" {
		normalized.Region = InvoiceRegionMainland
	}
	if normalized.Region != InvoiceRegionMainland && normalized.Region != InvoiceRegionHongKong {
		return InvoiceTitleDetails{}, ErrInvoiceRegionInvalid
	}
	if normalized.CompanyName == "" {
		return InvoiceTitleDetails{}, ErrInvoiceCompanyEmpty
	}
	if len([]rune(normalized.CompanyName)) > InvoiceCompanyNameMaxLength {
		return InvoiceTitleDetails{}, ErrInvoiceCompanyInvalid
	}
	if normalized.Region == InvoiceRegionHongKong {
		return normalized, nil
	}
	normalized.TaxNumber = strings.TrimSpace(details.TaxNumber)
	if normalized.TaxNumber == "" {
		return InvoiceTitleDetails{}, ErrInvoiceTaxNumberEmpty
	}
	if len([]rune(normalized.TaxNumber)) > InvoiceTaxNumberMaxLength {
		return InvoiceTitleDetails{}, ErrInvoiceTaxNumberInvalid
	}
	normalized.BankName = strings.TrimSpace(details.BankName)
	normalized.BankAccount = strings.TrimSpace(details.BankAccount)
	normalized.CompanyAddress = strings.TrimSpace(details.CompanyAddress)
	normalized.CompanyPhone = strings.TrimSpace(details.CompanyPhone)
	if len([]rune(normalized.BankName)) > InvoiceBankNameMaxLength ||
		len([]rune(normalized.BankAccount)) > InvoiceBankAccountMaxLength ||
		len([]rune(normalized.CompanyAddress)) > InvoiceCompanyAddressMaxLength ||
		len([]rune(normalized.CompanyPhone)) > InvoiceCompanyPhoneMaxLength {
		return InvoiceTitleDetails{}, ErrInvoiceOptionalFieldInvalid
	}
	return normalized, nil
}

type InvoiceTitle struct {
	Id             int    `json:"id"`
	UserId         int    `json:"-" gorm:"index;not null"`
	Region         string `json:"region" gorm:"type:varchar(16);not null;default:'mainland'"`
	CompanyName    string `json:"company_name" gorm:"type:varchar(255);not null"`
	TaxNumber      string `json:"tax_number" gorm:"type:varchar(64)"`
	BankName       string `json:"bank_name" gorm:"type:varchar(255)"`
	BankAccount    string `json:"bank_account" gorm:"type:varchar(128)"`
	CompanyAddress string `json:"company_address" gorm:"type:varchar(512)"`
	CompanyPhone   string `json:"company_phone" gorm:"type:varchar(32)"`
	IsDefault      bool   `json:"is_default"`
	CreatedTime    int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime    int64  `json:"updated_time" gorm:"bigint"`
}

func (title *InvoiceTitle) applyDetails(details InvoiceTitleDetails) {
	title.Region = details.Region
	title.CompanyName = details.CompanyName
	title.TaxNumber = details.TaxNumber
	title.BankName = details.BankName
	title.BankAccount = details.BankAccount
	title.CompanyAddress = details.CompanyAddress
	title.CompanyPhone = details.CompanyPhone
}

// ListInvoiceTitles returns the user's saved titles, defaults first and then
// the most recently updated.
func ListInvoiceTitles(userId int) ([]*InvoiceTitle, error) {
	titles := make([]*InvoiceTitle, 0)
	err := DB.Where("user_id = ?", userId).
		Order("is_default DESC").Order("updated_time DESC").Order("id DESC").
		Find(&titles).Error
	return titles, err
}

// lockInvoiceTitleOwner serializes title changes of one user so the
// per-user limit and the single default per region hold under concurrency.
func lockInvoiceTitleOwner(tx *gorm.DB, userId int) error {
	var user User
	return lockForUpdate(tx).Select("id").First(&user, "id = ?", userId).Error
}

func clearOtherInvoiceTitleDefaultsTx(tx *gorm.DB, title *InvoiceTitle) error {
	return tx.Model(&InvoiceTitle{}).
		Where("user_id = ? AND region = ? AND id <> ?", title.UserId, title.Region, title.Id).
		Update("is_default", false).Error
}

func CreateInvoiceTitle(userId int, details InvoiceTitleDetails, isDefault bool) (*InvoiceTitle, error) {
	normalized, err := details.Normalize()
	if err != nil {
		return nil, err
	}
	title := &InvoiceTitle{UserId: userId, IsDefault: isDefault}
	title.applyDetails(normalized)
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockInvoiceTitleOwner(tx, userId); err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&InvoiceTitle{}).Where("user_id = ?", userId).Count(&count).Error; err != nil {
			return err
		}
		if count >= InvoiceTitleMaxPerUser {
			return ErrInvoiceTitleLimit
		}
		now := common.GetTimestamp()
		title.CreatedTime = now
		title.UpdatedTime = now
		if err := tx.Create(title).Error; err != nil {
			return err
		}
		if isDefault {
			return clearOtherInvoiceTitleDefaultsTx(tx, title)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return title, nil
}

func UpdateInvoiceTitle(id int, userId int, details InvoiceTitleDetails, isDefault bool) (*InvoiceTitle, error) {
	normalized, err := details.Normalize()
	if err != nil {
		return nil, err
	}
	var title InvoiceTitle
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockInvoiceTitleOwner(tx, userId); err != nil {
			return err
		}
		if err := tx.Where("id = ? AND user_id = ?", id, userId).First(&title).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvoiceTitleNotFound
			}
			return err
		}
		title.applyDetails(normalized)
		title.IsDefault = isDefault
		title.UpdatedTime = common.GetTimestamp()
		if err := tx.Model(&InvoiceTitle{}).Where("id = ?", title.Id).Select(
			"region", "company_name", "tax_number", "bank_name", "bank_account",
			"company_address", "company_phone", "is_default", "updated_time",
		).Updates(&title).Error; err != nil {
			return err
		}
		if isDefault {
			return clearOtherInvoiceTitleDefaultsTx(tx, &title)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &title, nil
}

func DeleteInvoiceTitle(id int, userId int) error {
	result := DB.Where("id = ? AND user_id = ?", id, userId).Delete(&InvoiceTitle{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrInvoiceTitleNotFound
	}
	return nil
}

// rememberInvoiceTitleTx saves the details of a submitted request: a title
// with the same region, name and tax number is refreshed, otherwise a new one
// is added while the user is below the limit. A full list is left as it is
// so the request itself still succeeds.
func rememberInvoiceTitleTx(tx *gorm.DB, userId int, details InvoiceTitleDetails) error {
	if err := lockInvoiceTitleOwner(tx, userId); err != nil {
		return err
	}
	now := common.GetTimestamp()
	var existing InvoiceTitle
	err := tx.Where("user_id = ? AND region = ? AND company_name = ? AND tax_number = ?",
		userId, details.Region, details.CompanyName, details.TaxNumber).First(&existing).Error
	if err == nil {
		existing.applyDetails(details)
		existing.UpdatedTime = now
		return tx.Model(&InvoiceTitle{}).Where("id = ?", existing.Id).Select(
			"bank_name", "bank_account", "company_address", "company_phone", "updated_time",
		).Updates(&existing).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var count int64
	if err := tx.Model(&InvoiceTitle{}).Where("user_id = ?", userId).Count(&count).Error; err != nil {
		return err
	}
	if count >= InvoiceTitleMaxPerUser {
		return nil
	}
	title := &InvoiceTitle{UserId: userId, CreatedTime: now, UpdatedTime: now}
	title.applyDetails(details)
	return tx.Create(title).Error
}
