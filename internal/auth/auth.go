// Package auth holds the (intentionally small) authentication primitives used
// between the controller and the agents.
//
// For the MVP we use a pre-shared token over TLS, as chosen during design: the
// controller holds the expected token and each agent presents it in its Hello
// frame. Comparison is constant-time to avoid timing side channels. mTLS can be
// layered on later without changing the message flow.
package auth

import "crypto/subtle"

// TokenValid reports whether presented equals expected in constant time.
// An empty expected token means auth is effectively disabled; callers should
// treat that as a misconfiguration in production and warn accordingly.
func TokenValid(expected, presented string) bool {
	if expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(presented)) == 1
}
