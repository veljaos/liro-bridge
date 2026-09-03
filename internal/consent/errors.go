package consent

import "github.com/veljaos/liro-bridge/internal/errs"

// NeedsTSAChoice reports whether code's failure screen must offer the
// SPEC §12.8 choice — save without a timestamp (B-B, visibly marked) or
// cancel — rather than only "try again." Today that is exactly
// CodeTSAUnavailable (F5 §5.5's table); kept as a named function rather
// than an inline comparison at each call site so the one place that
// knows this rule can be tested and referenced instead of duplicated.
func NeedsTSAChoice(code errs.Code) bool {
	return code == errs.CodeTSAUnavailable
}
