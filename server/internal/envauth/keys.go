package envauth

// IsPlatformAuthEnvKey reports official ACP CLI auth keys that must not be
// injected via platform-level sandbox.env / opts.Env. Project sandbox env may
// store them as a workflow baseline (forced Secret); Agent env overrides.
//
// Kept in a tiny dependency-free package so services and runtime can share the
// rule without services→runtime coupling.
func IsPlatformAuthEnvKey(k string) bool {
	switch k {
	case "CURSOR_API_KEY", "ANTHROPIC_API_KEY", "CODEBUDDY_API_KEY",
		"TRAE_API_KEY", "TRAECLI_PERSONAL_ACCESS_TOKEN":
		return true
	default:
		return false
	}
}
