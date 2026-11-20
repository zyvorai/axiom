package auth

import "sync"

// LDAPBindFunc is the signature of LDAPAuthenticate.
type LDAPBindFunc func(cfg LDAPConfig, username, password string) (role string, ok bool, err error)

var (
	ldapBindMu sync.RWMutex
	ldapBind   LDAPBindFunc = LDAPAuthenticate
)

// ldapAuthenticate is the directory bind the Guard uses. It is LDAPAuthenticate
// unless a test has swapped in a fake directory with SwapLDAPAuthenticate.
func ldapAuthenticate(cfg LDAPConfig, username, password string) (string, bool, error) {
	ldapBindMu.RLock()
	fn := ldapBind
	ldapBindMu.RUnlock()
	return fn(cfg, username, password)
}

// SwapLDAPAuthenticate replaces the directory bind used by every Guard and
// returns a function that restores the previous one. It exists for tests in
// other packages that need a fake directory without a network listener; it is
// never called by production code.
func SwapLDAPAuthenticate(fn LDAPBindFunc) (restore func()) {
	ldapBindMu.Lock()
	prev := ldapBind
	ldapBind = fn
	ldapBindMu.Unlock()
	return func() {
		ldapBindMu.Lock()
		ldapBind = prev
		ldapBindMu.Unlock()
	}
}
