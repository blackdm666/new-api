package setting

import "strings"

const (
	DefaultAntomGateway     = "https://open-sea-global.alipay.com"
	DefaultAntomDisplayName = "Global Wallet Payment"
)

var (
	AntomEnabled            bool
	AntomDisplayName        = DefaultAntomDisplayName
	AntomGateway            = DefaultAntomGateway
	AntomClientId           string
	AntomMerchantPrivateKey string
	AntomPublicKey          string
	AntomNotifyURL          string
	AntomRedirectURL        string
)

func GetAntomDisplayName() string {
	displayName := strings.TrimSpace(AntomDisplayName)
	if displayName == "" {
		return DefaultAntomDisplayName
	}
	return displayName
}
