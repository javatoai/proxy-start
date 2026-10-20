package native

import "proxy-start/internal/application"

// Production returns the native adapters supported by the shipped product.
//
// Codex supports Windows and macOS; Claude supports its Windows Store package.
// Other applications, including Chrome and Edge, are selected by the
// application layer's forced-proxy fallback. Keeping this list here makes the
// production allowlist explicit and prevents a platform resolver from
// accidentally turning an arbitrary executable basename into a native launch.
func Production() []application.NativeAdapter {
	return []application.NativeAdapter{NewCodex(), NewClaude()}
}
