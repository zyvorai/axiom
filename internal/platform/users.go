package platform

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleViewer   = "viewer"
)

// NormalizeRole maps any input to a known role. Empty/unknown values become
// admin so a typo never silently locks an operator into read-only (existing
// Create User API behavior). Explicit "viewer" / "operator" are preserved.
func NormalizeRole(r string) string {
	switch strings.ToLower(strings.TrimSpace(r)) {
	case RoleViewer:
		return RoleViewer
	case RoleOperator:
		return RoleOperator
	default:
		return RoleAdmin
	}
}

// User is a local Axiom account, distinct from the single env-configured
// operator account (AXIOM_USERNAME/AXIOM_PASSWORD) and from OIDC/SAML users.
//
// Namespaces restricts a viewer account to reading only the listed
// namespaces; empty means unrestricted (every namespace), which is also the
// only valid value for an admin account — namespace-scoping only applies to
// viewers, since an admin needs cross-namespace write access anyway.
type User struct {
	Username     string   `json:"username"`
	PasswordHash string   `json:"passwordHash"`
	Role         string   `json:"role"`
	Namespaces   []string `json:"namespaces,omitempty"`
	CreatedAt    string   `json:"createdAt"`

	// Two-factor sign-in (TOTP). MFASecret is the active shared secret and is
	// only meaningful while MFAEnabled; MFAPending holds a secret that has been
	// issued but not yet confirmed with a code. MFARecovery are SHA-256 hashes of
	// the unused one-time recovery codes. MFALastStep is the last TOTP time step
	// accepted, which stops a code being replayed. These live in the same Secret
	// as the password hashes and are never returned by the API.
	MFAEnabled  bool     `json:"mfaEnabled,omitempty"`
	MFASecret   string   `json:"mfaSecret,omitempty"`
	MFAPending  string   `json:"mfaPending,omitempty"`
	MFARecovery []string `json:"mfaRecovery,omitempty"`
	MFALastStep int64    `json:"mfaLastStep,omitempty"`
}

// UserView is the safe representation returned by the Users API — never
// includes the password hash or any second-factor secret.
type UserView struct {
	Username   string   `json:"username"`
	Role       string   `json:"role"`
	Namespaces []string `json:"namespaces,omitempty"`
	CreatedAt  string   `json:"createdAt"`
	MFAEnabled bool     `json:"mfaEnabled"`
}

func (u User) View() UserView {
	return UserView{Username: u.Username, Role: u.Role, Namespaces: u.Namespaces, CreatedAt: u.CreatedAt, MFAEnabled: u.MFAEnabled}
}

// normalizeNamespaces trims/dedupes and enforces that only viewers/operators
// can be namespace-restricted — an admin account always gets unrestricted (nil).
func normalizeNamespaces(role string, namespaces []string) []string {
	if role == RoleAdmin {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(namespaces))
	for _, ns := range namespaces {
		ns = strings.TrimSpace(ns)
		if ns == "" || seen[ns] {
			continue
		}
		seen[ns] = true
		out = append(out, ns)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func CheckPassword(hash, password string) bool {
	if hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// UserStore persists local accounts across restarts.
type UserStore interface {
	List(ctx context.Context) ([]User, error)
	Create(ctx context.Context, u User) error
	Update(ctx context.Context, u User) error
	Delete(ctx context.Context, username string) error
}

type MemoryUserStore struct {
	mu    sync.Mutex
	users map[string]User
}

func NewMemoryUserStore() *MemoryUserStore {
	return &MemoryUserStore{users: map[string]User{}}
}

func (m *MemoryUserStore) List(ctx context.Context) ([]User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]User, 0, len(m.users))
	for _, u := range m.users {
		out = append(out, u)
	}
	return out, nil
}

func (m *MemoryUserStore) Create(ctx context.Context, u User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := strings.ToLower(u.Username)
	if _, exists := m.users[key]; exists {
		return fmt.Errorf("user %q already exists", u.Username)
	}
	m.users[key] = u
	return nil
}

func (m *MemoryUserStore) Update(ctx context.Context, u User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := strings.ToLower(u.Username)
	if _, exists := m.users[key]; !exists {
		return fmt.Errorf("user %q not found", u.Username)
	}
	m.users[key] = u
	return nil
}

func (m *MemoryUserStore) Delete(ctx context.Context, username string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := strings.ToLower(username)
	if _, ok := m.users[key]; !ok {
		return fmt.Errorf("user %q not found", username)
	}
	delete(m.users, key)
	return nil
}

// UserHub owns the persistent UserStore plus an in-memory cache so every
// HTTP request can authenticate without a store round-trip.
type UserHub struct {
	mu    sync.RWMutex
	store UserStore
	cache map[string]User
	// mfaMu serialises second-factor checks, which read-modify-write the
	// replay counter and recovery codes and persist the result.
	mfaMu sync.Mutex
	// onInvalidate is called with a username whenever something that should end
	// that person's sessions changes (password, role, namespaces, MFA, delete).
	onInvalidate func(username string)
}

// SetInvalidateHook registers the function that revokes a user's sessions.
func (h *UserHub) SetInvalidateHook(fn func(username string)) {
	h.mu.Lock()
	h.onInvalidate = fn
	h.mu.Unlock()
}

func (h *UserHub) invalidate(username string) {
	h.mu.RLock()
	fn := h.onInvalidate
	h.mu.RUnlock()
	if fn != nil {
		fn(username)
	}
}

func NewUserHub(store UserStore) *UserHub {
	if store == nil {
		store = NewMemoryUserStore()
	}
	return &UserHub{store: store, cache: map[string]User{}}
}

func (h *UserHub) Load(ctx context.Context) error {
	users, err := h.store.List(ctx)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cache = make(map[string]User, len(users))
	for _, u := range users {
		h.cache[strings.ToLower(u.Username)] = u
	}
	return nil
}

// Authenticate implements auth.UserAuthenticator.
func (h *UserHub) Authenticate(username, password string) (string, []string, bool) {
	h.mu.RLock()
	u, ok := h.cache[strings.ToLower(strings.TrimSpace(username))]
	h.mu.RUnlock()
	if !ok || !CheckPassword(u.PasswordHash, password) {
		return "", nil, false
	}
	return u.Role, u.Namespaces, true
}

func (h *UserHub) List() []UserView {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]UserView, 0, len(h.cache))
	for _, u := range h.cache {
		out = append(out, u.View())
	}
	return out
}

func (h *UserHub) Create(ctx context.Context, username, password, role string, namespaces []string) (UserView, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return UserView{}, fmt.Errorf("username is required")
	}
	// HTTP Basic splits credentials on the first ':', so such an account could be created but never sign in.
	if strings.ContainsAny(username, ":\x00\r\n") {
		return UserView{}, fmt.Errorf("username must not contain ':' or control characters")
	}
	if len(password) < 8 {
		return UserView{}, fmt.Errorf("password must be at least 8 characters")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return UserView{}, err
	}
	normRole := NormalizeRole(role)
	u := User{
		Username:     username,
		PasswordHash: hash,
		Role:         normRole,
		Namespaces:   normalizeNamespaces(normRole, namespaces),
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	if err := h.store.Create(ctx, u); err != nil {
		return UserView{}, err
	}
	h.mu.Lock()
	h.cache[strings.ToLower(username)] = u
	h.mu.Unlock()
	return u.View(), nil
}

// UpdateRole changes an existing local account's role. Promoting to admin
// clears any namespace restriction, since admins are never namespace-scoped.
func (h *UserHub) UpdateRole(ctx context.Context, username, role string) (UserView, error) {
	key := strings.ToLower(strings.TrimSpace(username))
	h.mu.RLock()
	u, ok := h.cache[key]
	h.mu.RUnlock()
	if !ok {
		return UserView{}, fmt.Errorf("user %q not found", username)
	}
	u.Role = NormalizeRole(role)
	u.Namespaces = normalizeNamespaces(u.Role, u.Namespaces)
	if err := h.store.Update(ctx, u); err != nil {
		return UserView{}, err
	}
	h.mu.Lock()
	h.cache[key] = u
	h.mu.Unlock()
	h.invalidate(u.Username)
	return u.View(), nil
}

// UpdateNamespaces sets the namespace restriction for an existing viewer
// account. Pass an empty slice to remove the restriction. No-ops (returns an
// error) for an admin account, since admins can't be namespace-scoped.
func (h *UserHub) UpdateNamespaces(ctx context.Context, username string, namespaces []string) (UserView, error) {
	key := strings.ToLower(strings.TrimSpace(username))
	h.mu.RLock()
	u, ok := h.cache[key]
	h.mu.RUnlock()
	if !ok {
		return UserView{}, fmt.Errorf("user %q not found", username)
	}
	if u.Role != RoleViewer && u.Role != RoleOperator {
		return UserView{}, fmt.Errorf("only viewer/operator accounts can be namespace-restricted")
	}
	u.Namespaces = normalizeNamespaces(u.Role, namespaces)
	if err := h.store.Update(ctx, u); err != nil {
		return UserView{}, err
	}
	h.mu.Lock()
	h.cache[key] = u
	h.mu.Unlock()
	h.invalidate(u.Username)
	return u.View(), nil
}

// ResetPassword sets a new password for an existing local account.
func (h *UserHub) ResetPassword(ctx context.Context, username, password string) (UserView, error) {
	if len(password) < 8 {
		return UserView{}, fmt.Errorf("password must be at least 8 characters")
	}
	key := strings.ToLower(strings.TrimSpace(username))
	h.mu.RLock()
	u, ok := h.cache[key]
	h.mu.RUnlock()
	if !ok {
		return UserView{}, fmt.Errorf("user %q not found", username)
	}
	hash, err := HashPassword(password)
	if err != nil {
		return UserView{}, err
	}
	u.PasswordHash = hash
	if err := h.store.Update(ctx, u); err != nil {
		return UserView{}, err
	}
	h.mu.Lock()
	h.cache[key] = u
	h.mu.Unlock()
	h.invalidate(u.Username)
	return u.View(), nil
}

func (h *UserHub) Delete(ctx context.Context, username string) error {
	if err := h.store.Delete(ctx, username); err != nil {
		return err
	}
	h.mu.Lock()
	delete(h.cache, strings.ToLower(username))
	h.mu.Unlock()
	h.invalidate(username)
	return nil
}

func (h *UserHub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.cache)
}
