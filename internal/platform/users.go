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
	RoleAdmin  = "admin"
	RoleViewer = "viewer"
)

// NormalizeRole maps any input to a known role, defaulting to admin so a
// typo never silently locks an operator into read-only.
func NormalizeRole(r string) string {
	if strings.EqualFold(strings.TrimSpace(r), RoleViewer) {
		return RoleViewer
	}
	return RoleAdmin
}

// User is a local Axiom account, distinct from the single env-configured
// operator account (AXIOM_USERNAME/AXIOM_PASSWORD) and from OIDC/SAML users.
type User struct {
	Username     string `json:"username"`
	PasswordHash string `json:"passwordHash"`
	Role         string `json:"role"`
	CreatedAt    string `json:"createdAt"`
}

// UserView is the safe representation returned by the Users API — never
// includes the password hash.
type UserView struct {
	Username  string `json:"username"`
	Role      string `json:"role"`
	CreatedAt string `json:"createdAt"`
}

func (u User) View() UserView {
	return UserView{Username: u.Username, Role: u.Role, CreatedAt: u.CreatedAt}
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
func (h *UserHub) Authenticate(username, password string) (string, bool) {
	h.mu.RLock()
	u, ok := h.cache[strings.ToLower(strings.TrimSpace(username))]
	h.mu.RUnlock()
	if !ok || !CheckPassword(u.PasswordHash, password) {
		return "", false
	}
	return u.Role, true
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

func (h *UserHub) Create(ctx context.Context, username, password, role string) (UserView, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return UserView{}, fmt.Errorf("username is required")
	}
	if len(password) < 8 {
		return UserView{}, fmt.Errorf("password must be at least 8 characters")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return UserView{}, err
	}
	u := User{
		Username:     username,
		PasswordHash: hash,
		Role:         NormalizeRole(role),
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

// UpdateRole changes an existing local account's role.
func (h *UserHub) UpdateRole(ctx context.Context, username, role string) (UserView, error) {
	key := strings.ToLower(strings.TrimSpace(username))
	h.mu.RLock()
	u, ok := h.cache[key]
	h.mu.RUnlock()
	if !ok {
		return UserView{}, fmt.Errorf("user %q not found", username)
	}
	u.Role = NormalizeRole(role)
	if err := h.store.Update(ctx, u); err != nil {
		return UserView{}, err
	}
	h.mu.Lock()
	h.cache[key] = u
	h.mu.Unlock()
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
	return u.View(), nil
}

func (h *UserHub) Delete(ctx context.Context, username string) error {
	if err := h.store.Delete(ctx, username); err != nil {
		return err
	}
	h.mu.Lock()
	delete(h.cache, strings.ToLower(username))
	h.mu.Unlock()
	return nil
}

func (h *UserHub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.cache)
}
