// Code generated from labels by github.com/epistemer/i18n. DO NOT EDIT.

package settings

import "github.com/epistemer/i18n"

var (
	L10nAuth                      = i18n.Key("settings.auth", "Sign-in and sessions")
	L10nAuthIssuer                = i18n.Key("settings.auth.issuer", "Issuer: the name in tokens and authenticator apps")
	L10nAuthSecret                = i18n.Key("settings.auth.secret", "Master secret, at least 32 bytes; every key is derived from it")
	L10nAuthSecretPrevious        = i18n.Key("settings.auth.secret_previous", "Previous master secret, while rotating")
	L10nAuthTrustedOrigins        = i18n.Key("settings.auth.trusted_origins", "Origins whose cross-origin requests are trusted")
	L10nAuthSession               = i18n.Key("settings.auth.session", "Sessions")
	L10nAuthSessionIdle           = i18n.Key("settings.auth.session.idle", "Idle lifetime of a session")
	L10nAuthSessionRememberedIdle = i18n.Key("settings.auth.session.remembered_idle", "Idle lifetime of a remembered session")
	L10nAuthSessionLifetime       = i18n.Key("settings.auth.session.lifetime", "Longest lifetime of a session, whatever its use")
	L10nAuthAccess                = i18n.Key("settings.auth.access", "Access tokens")
	L10nAuthAccessLifetime        = i18n.Key("settings.auth.access.lifetime", "Lifetime of an access token")
	L10nAuthRefresh               = i18n.Key("settings.auth.refresh", "Refresh tokens")
	L10nAuthRefreshGrace          = i18n.Key("settings.auth.refresh.grace", "How long a replaced refresh token still returns its successor")
	L10nAuthFlow                  = i18n.Key("settings.auth.flow", "Sign-in flows")
	L10nAuthFlowLifetime          = i18n.Key("settings.auth.flow.lifetime", "Lifetime of a sign-in flow")
	L10nAuthFresh                 = i18n.Key("settings.auth.fresh", "Fresh proofs")
	L10nAuthFreshWindow           = i18n.Key("settings.auth.fresh.window", "How recent a proof must be to change a factor")
	L10nAuthCode                  = i18n.Key("settings.auth.code", "Codes sent by email or SMS")
	L10nAuthCodeLength            = i18n.Key("settings.auth.code.length", "Digits in a code")
	L10nAuthCodeLifetime          = i18n.Key("settings.auth.code.lifetime", "Lifetime of a code")
	L10nAuthCodeTries             = i18n.Key("settings.auth.code.tries", "Wrong tries a code allows")
	L10nAuthCodeResend            = i18n.Key("settings.auth.code.resend", "Wait before another code may be sent")
	L10nAuthCodeHourly            = i18n.Key("settings.auth.code.hourly", "Codes a user may be sent in an hour")
	L10nAuthLink                  = i18n.Key("settings.auth.link", "Links sent by email")
	L10nAuthLinkLifetime          = i18n.Key("settings.auth.link.lifetime", "Lifetime of a recovery link")
	L10nAuthInvitation            = i18n.Key("settings.auth.invitation", "Invitations")
	L10nAuthInvitationLifetime    = i18n.Key("settings.auth.invitation.lifetime", "Lifetime of an invitation")
	L10nAuthDevice                = i18n.Key("settings.auth.device", "Registered devices")
	L10nAuthDeviceLifetime        = i18n.Key("settings.auth.device.lifetime", "Lifetime of a registered device")
	L10nAuthPassword              = i18n.Key("settings.auth.password", "Passwords")
	L10nAuthPasswordMinLength     = i18n.Key("settings.auth.password.min_length", "Fewest characters in a password")
	L10nAuthPasswordMinScore      = i18n.Key("settings.auth.password.min_score", "Least strength a new password needs, 3 or 4")
	L10nAuthPasswordMemory        = i18n.Key("settings.auth.password.memory", "Memory for each password hash, in MiB")
	L10nAuthPasswordTime          = i18n.Key("settings.auth.password.time", "Passes of each password hash")
	L10nAuthPasswordConcurrency   = i18n.Key("settings.auth.password.concurrency", "Password hashes at once")
	L10nAuthThrottle              = i18n.Key("settings.auth.throttle", "Throttling of wrong proofs")
	L10nAuthThrottleFree          = i18n.Key("settings.auth.throttle.free", "Failures before waits start")
	L10nAuthThrottleCeiling       = i18n.Key("settings.auth.throttle.ceiling", "Longest wait")
	L10nAuthThrottleWindow        = i18n.Key("settings.auth.throttle.window", "Time without a failure after which a counter starts again")
	L10nAuthCookie                = i18n.Key("settings.auth.cookie", "Cookies")
	L10nAuthCookieInsecure        = i18n.Key("settings.auth.cookie.insecure", "Drop Secure and the __Host- prefix, for development over plain HTTP")
	L10nI18n                      = i18n.Key("settings.i18n", "Translations")
	L10nI18nDefaultLocale         = i18n.Key("settings.i18n.default_locale", "Locale for visitors without a chosen locale; empty means the source locale")
	L10nServer                    = i18n.Key("settings.server", "Web server")
	L10nServerAddr                = i18n.Key("settings.server.addr", "Listen address")
	L10nServerStore               = i18n.Key("settings.server.store", "Store file")
)

// l10n holds every label that has a path.
var l10n = map[string]i18n.TKer{
	"auth":                         L10nAuth,
	"auth.issuer":                  L10nAuthIssuer,
	"auth.secret":                  L10nAuthSecret,
	"auth.secret_previous":         L10nAuthSecretPrevious,
	"auth.trusted_origins":         L10nAuthTrustedOrigins,
	"auth.session":                 L10nAuthSession,
	"auth.session.idle":            L10nAuthSessionIdle,
	"auth.session.remembered_idle": L10nAuthSessionRememberedIdle,
	"auth.session.lifetime":        L10nAuthSessionLifetime,
	"auth.access":                  L10nAuthAccess,
	"auth.access.lifetime":         L10nAuthAccessLifetime,
	"auth.refresh":                 L10nAuthRefresh,
	"auth.refresh.grace":           L10nAuthRefreshGrace,
	"auth.flow":                    L10nAuthFlow,
	"auth.flow.lifetime":           L10nAuthFlowLifetime,
	"auth.fresh":                   L10nAuthFresh,
	"auth.fresh.window":            L10nAuthFreshWindow,
	"auth.code":                    L10nAuthCode,
	"auth.code.length":             L10nAuthCodeLength,
	"auth.code.lifetime":           L10nAuthCodeLifetime,
	"auth.code.tries":              L10nAuthCodeTries,
	"auth.code.resend":             L10nAuthCodeResend,
	"auth.code.hourly":             L10nAuthCodeHourly,
	"auth.link":                    L10nAuthLink,
	"auth.link.lifetime":           L10nAuthLinkLifetime,
	"auth.invitation":              L10nAuthInvitation,
	"auth.invitation.lifetime":     L10nAuthInvitationLifetime,
	"auth.device":                  L10nAuthDevice,
	"auth.device.lifetime":         L10nAuthDeviceLifetime,
	"auth.password":                L10nAuthPassword,
	"auth.password.min_length":     L10nAuthPasswordMinLength,
	"auth.password.min_score":      L10nAuthPasswordMinScore,
	"auth.password.memory":         L10nAuthPasswordMemory,
	"auth.password.time":           L10nAuthPasswordTime,
	"auth.password.concurrency":    L10nAuthPasswordConcurrency,
	"auth.throttle":                L10nAuthThrottle,
	"auth.throttle.free":           L10nAuthThrottleFree,
	"auth.throttle.ceiling":        L10nAuthThrottleCeiling,
	"auth.throttle.window":         L10nAuthThrottleWindow,
	"auth.cookie":                  L10nAuthCookie,
	"auth.cookie.insecure":         L10nAuthCookieInsecure,
	"i18n":                         L10nI18n,
	"i18n.default_locale":          L10nI18nDefaultLocale,
	"server":                       L10nServer,
	"server.addr":                  L10nServerAddr,
	"server.store":                 L10nServerStore,
}

// L10n returns the label at path, or nil when there is none.
func L10n(path string) i18n.TKer { return l10n[path] }
