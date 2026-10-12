package middleware

import (
	"fmt"

	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
)

// accessToken88APIRouteRules declares the token rule of the 88API dashboard
// routes. They live apart from the upstream table so upstream merges do not
// conflict; init adds them to accessTokenRouteRules and panics when a key is
// declared in both. Routes that move money out of the system (payout requests,
// Alipay transfers) and playground relays stay browser-session only.
var accessToken88APIRouteRules = map[string]accessTokenRouteRule{
	// router/api-router.go: /api/user (self) affiliate and Antom
	"GET /api/user/aff/summary":             accessTokenScopeRule("wallet:read"),
	"GET /api/user/aff/commissions":         accessTokenScopeRule("wallet:read"),
	"GET /api/user/aff/invitees":            accessTokenScopeRule("wallet:read"),
	"GET /api/user/aff/invitee_stats":       accessTokenScopeRule("wallet:read"),
	"GET /api/user/aff/transfers":           accessTokenScopeRule("wallet:read"),
	"POST /api/user/aff/transfers":          accessTokenScopeRule("wallet:write"),
	"GET /api/user/aff/payouts/summary":     accessTokenScopeRule("wallet:read"),
	"GET /api/user/aff/payouts":             accessTokenScopeRule("wallet:read"),
	"POST /api/user/aff/payouts":            accessTokenSessionRule,
	"POST /api/user/aff/payouts/:id/cancel": accessTokenScopeRule("wallet:write"),
	"POST /api/user/antom/pay":              accessTokenScopeRule("wallet:write"),
	"POST /api/user/antom/query":            accessTokenScopeRule("wallet:write"),

	// router/api-router.go: /api/user (admin)
	"GET /api/user/inviter-options": accessTokenScopeRule("user:read"),

	// router/api-router.go: /api/affiliate/admin (AdminAuth)
	"GET /api/affiliate/admin/summary":                          accessTokenScopeRule("billing:read"),
	"GET /api/affiliate/admin/invitees":                         accessTokenScopeRule("billing:read"),
	"GET /api/affiliate/admin/commissions":                      accessTokenScopeRule("billing:read"),
	"GET /api/affiliate/admin/transfers":                        accessTokenScopeRule("billing:read"),
	"POST /api/affiliate/admin/commissions/:id/approve":         accessTokenScopeRule("billing:write"),
	"POST /api/affiliate/admin/commissions/:id/reject":          accessTokenScopeRule("billing:write"),
	"GET /api/affiliate/admin/payouts/summary":                  accessTokenScopeRule("billing:read"),
	"GET /api/affiliate/admin/payouts":                          accessTokenScopeRule("billing:read"),
	"GET /api/affiliate/admin/payout-provider":                  accessTokenScopeRule("billing:read"),
	"POST /api/affiliate/admin/payouts/:id/approve":             accessTokenScopeRule("billing:write"),
	"POST /api/affiliate/admin/payouts/:id/reject":              accessTokenScopeRule("billing:write"),
	"POST /api/affiliate/admin/payouts/:id/paid":                accessTokenScopeRule("billing:write"),
	"POST /api/affiliate/admin/payouts/:id/alipay":              accessTokenSessionRule,
	"POST /api/affiliate/admin/payouts/:id/alipay/status":       accessTokenScopeRule("billing:write"),
	"GET /api/affiliate/admin/upgrade-candidates":               accessTokenScopeRule("billing:read"),
	"POST /api/affiliate/admin/upgrade-candidates/:id/approve":  accessTokenScopeRule("billing:write"),
	"GET /api/affiliate/admin/notification-failures":            accessTokenScopeRule("billing:read"),
	"POST /api/affiliate/admin/notification-failures/:id/retry": accessTokenScopeRule("billing:write"),

	// router/api-router.go: /api/affiliate/root (RootAuth)
	"PUT /api/affiliate/root/settings":              accessTokenScopeRule("option:write"),
	"GET /api/affiliate/root/payout-settings":       accessTokenScopeRule("option:read"),
	"PUT /api/affiliate/root/payout-settings":       accessTokenScopeRule("option:write"),
	"POST /api/affiliate/root/payout-settings/test": accessTokenScopeRule("option:write"),

	// router/api-router.go: /api/invoice (UserAuth)
	"GET /api/invoice/config":                      accessTokenScopeRule("wallet:read"),
	"GET /api/invoice/eligible_orders":             accessTokenScopeRule("wallet:read"),
	"POST /api/invoice/requests":                   accessTokenScopeRule("wallet:write"),
	"GET /api/invoice/requests":                    accessTokenScopeRule("wallet:read"),
	"GET /api/invoice/requests/:id":                accessTokenScopeRule("wallet:read"),
	"POST /api/invoice/requests/:id/withdraw":      accessTokenScopeRule("wallet:write"),
	"GET /api/invoice/requests/:id/files/:file_id": accessTokenScopeRule("wallet:read"),

	// router/api-router.go: /api/invoice/admin (AdminAuth)
	"GET /api/invoice/admin/requests":                                      accessTokenScopeRule("billing:read"),
	"GET /api/invoice/admin/requests/:id":                                  accessTokenScopeRule("billing:read"),
	"GET /api/invoice/admin/requests/:id/user-profile":                     accessTokenScopeRule("billing:read"),
	"POST /api/invoice/admin/requests/:id/files":                           accessTokenScopeRule("billing:write"),
	"DELETE /api/invoice/admin/requests/:id/files/:file_id":                accessTokenScopeRule("billing:write"),
	"POST /api/invoice/admin/requests/:id/notifications/issued/resend":     accessTokenScopeRule("billing:write"),
	"PUT /api/invoice/admin/requests/:id/status":                           accessTokenScopeRule("billing:write"),
	"DELETE /api/invoice/admin/requests/:id":                               accessTokenScopeRule("billing:write"),
	"GET /api/invoice/admin/maintenance":                                   accessTokenScopeRule("billing:read"),
	"POST /api/invoice/admin/maintenance/cleanups/:cleanup_id/retry":       accessTokenScopeRule("billing:write"),
	"POST /api/invoice/admin/maintenance/notifications/:delivery_id/retry": accessTokenScopeRule("billing:write"),
	"POST /api/invoice/admin/maintenance/reconcile":                        accessTokenScopeRule("billing:write"),
	"POST /api/invoice/admin/maintenance/orphans/cleanup":                  accessTokenScopeRule("billing:write"),

	// router/api-router.go: /api/option (RootAuth)
	"POST /api/option":                                    accessTokenScopeRule("option:write"),
	"PUT /api/option/bot-protection":                      accessTokenScopeRule("option:write"),
	"POST /api/option/smtp-test":                          accessTokenScopeRule("option:write"),
	"GET /api/option/smtp/marketing-accounts":             accessTokenScopeRule("option:read"),
	"POST /api/option/smtp/marketing-accounts":            accessTokenScopeRule("option:write"),
	"PUT /api/option/smtp/marketing-accounts/:id":         accessTokenScopeRule("option:write"),
	"PUT /api/option/smtp/marketing-accounts/:id/enabled": accessTokenScopeRule("option:write"),
	"DELETE /api/option/smtp/marketing-accounts/:id":      accessTokenScopeRule("option:write"),
	"POST /api/option/smtp/marketing-accounts/:id/test":   accessTokenScopeRule("option:write"),
	"GET /api/option/smtp/receipts":                       accessTokenScopeRule("option:read"),
	"PUT /api/option/smtp/receipts":                       accessTokenScopeRule("option:write"),
	"POST /api/option/smtp/receipts/token":                accessTokenScopeRule("option:write"),
	"GET /api/option/email_templates":                     accessTokenScopeRule("option:read"),
	"POST /api/option/email_templates/save":               accessTokenScopeRule("option:write"),
	"POST /api/option/email_templates/preview":            accessTokenScopeRule("option:read"),
	"POST /api/option/email_templates/reset":              accessTokenScopeRule("option:write"),
	"GET /api/option/email_deliveries":                    accessTokenScopeRule("option:read"),
	"GET /api/option/email_deliveries/stats":              accessTokenScopeRule("option:read"),
	"POST /api/option/email_deliveries/:id/retry":         accessTokenScopeRule("option:write"),
	"POST /api/option/email_deliveries/retry":             accessTokenScopeRule("option:write"),
	"PUT /api/option/invoice":                             accessTokenScopeRule("option:write"),
	"POST /api/option/invoice/storage-test":               accessTokenScopeRule("option:write"),

	// router/api-router.go: /api/marketing (RootAuth). Marketing mail uses the
	// root-only system settings scope; option:write already covers the SMTP
	// accounts these campaigns send through.
	"GET /api/marketing/user-notices/users":             accessTokenScopeRule("option:read"),
	"GET /api/marketing/user-notices":                   accessTokenScopeRule("option:read"),
	"POST /api/marketing/user-notices":                  accessTokenScopeRule("option:write"),
	"GET /api/marketing/overview":                       accessTokenScopeRule("option:read"),
	"GET /api/marketing/campaigns":                      accessTokenScopeRule("option:read"),
	"POST /api/marketing/campaigns":                     accessTokenScopeRule("option:write"),
	"PUT /api/marketing/campaigns/:id":                  accessTokenScopeRule("option:write"),
	"POST /api/marketing/campaigns/preview":             accessTokenScopeRule("option:read"),
	"POST /api/marketing/campaigns/:id/schedule":        accessTokenScopeRule("option:write"),
	"POST /api/marketing/campaigns/:id/pause":           accessTokenScopeRule("option:write"),
	"POST /api/marketing/campaigns/:id/resume":          accessTokenScopeRule("option:write"),
	"POST /api/marketing/campaigns/:id/cancel":          accessTokenScopeRule("option:write"),
	"DELETE /api/marketing/campaigns/:id":               accessTokenScopeRule("option:write"),
	"POST /api/marketing/campaigns/:id/clone":           accessTokenScopeRule("option:write"),
	"POST /api/marketing/campaigns/:id/retry-untracked": accessTokenScopeRule("option:write"),
	"GET /api/marketing/campaigns/:id/recipients":       accessTokenScopeRule("option:read"),
	"POST /api/marketing/test":                          accessTokenScopeRule("option:write"),
	"POST /api/marketing/preview":                       accessTokenScopeRule("option:read"),
	"GET /api/marketing/automations":                    accessTokenScopeRule("option:read"),
	"GET /api/marketing/announcements/latest":           accessTokenScopeRule("option:read"),
	"PUT /api/marketing/automations/:scene":             accessTokenScopeRule("option:write"),
	"GET /api/marketing/automations/:scene/preview":     accessTokenScopeRule("option:read"),
	"GET /api/marketing/suppressions":                   accessTokenScopeRule("option:read"),
	"POST /api/marketing/suppressions":                  accessTokenScopeRule("option:write"),
	"DELETE /api/marketing/suppressions/:id":            accessTokenScopeRule("option:write"),

	// router/api-router.go: /api/token/group/migrate (RootAuth)
	"POST /api/token/group/migrate": accessTokenScopeRule("group:write"),

	// router/api-router.go: /api/task
	"GET /api/task/:task_id/preview-url": accessTokenScopeRule("usage:read"),

	// router/channel-router.go: the balance query token route is RootAuth, not
	// Casbin-guarded, and reveals a channel secret like the key route.
	"POST /api/channel/:id/balance_query/token": accessTokenScopeRule(service.AccessTokenScopeOf(authz.ChannelSecretView)),

	// router/relay-router.go: playground relays, like /pg/chat/completions
	"POST /pg/images/generations": accessTokenSessionRule,
	"POST /pg/videos":             accessTokenSessionRule,
	"GET /pg/videos/:task_id":     accessTokenSessionRule,
}

func init() {
	for key, rule := range accessToken88APIRouteRules {
		if _, ok := accessTokenRouteRules[key]; ok {
			panic(fmt.Sprintf("access token route %s is declared twice", key))
		}
		accessTokenRouteRules[key] = rule
	}
}
