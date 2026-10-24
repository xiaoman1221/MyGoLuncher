package auth

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"MyMinecraftGoLuncher/internal/secretstore"
	"MyMinecraftGoLuncher/internal/util"
)

// Kind identifies how an account authenticates.
type Kind string

const (
	KindOffline   Kind = "offline"
	KindMicrosoft Kind = "microsoft"
	KindAuthlib   Kind = "authlib"
)

// Account is persisted. Passwords are never stored; Microsoft refresh tokens
// and third-party access tokens are kept in the encrypted secret store.
type Account struct {
	ID           string    `json:"id"`
	Kind         Kind      `json:"kind"`
	Name         string    `json:"name"`
	UUID         string    `json:"uuid"`
	AccessToken  string    `json:"accessToken,omitempty"`
	RefreshToken string    `json:"refreshToken,omitempty"`
	ClientToken  string    `json:"clientToken,omitempty"`
	ServerURL    string    `json:"serverUrl,omitempty"`
	XUID         string    `json:"xuid,omitempty"`
	ExpiresAt    time.Time `json:"expiresAt,omitempty"`
	AddedAt      time.Time `json:"addedAt"`

	// OAuth2 marks accounts obtained through the site's device-code flow.
	// RefreshToken then stores the site refresh token instead of a Yggdrasil one.
	OAuth2              bool   `json:"oauth2,omitempty"`
	OAuth2TokenEndpoint string `json:"oauth2TokenEndpoint,omitempty"`
	OAuth2ClientID      string `json:"oauth2ClientId,omitempty"`
}

// View is the safe account representation exposed to the UI.
type View struct {
	ID        string `json:"id"`
	Kind      Kind   `json:"kind"`
	Name      string `json:"name"`
	UUID      string `json:"uuid"`
	ServerURL string `json:"serverUrl"`
	ExpiresAt string `json:"expiresAt"`
	Label     string `json:"label"`
}

// View strips secrets and formats timestamps for the frontend.
func (a Account) View() View {
	label := "离线"
	switch a.Kind {
	case KindMicrosoft:
		label = "Microsoft 正版"
	case KindAuthlib:
		label = "外置登录"
	}
	expires := ""
	if !a.ExpiresAt.IsZero() {
		expires = a.ExpiresAt.Format(time.RFC3339)
	}
	return View{
		ID: a.ID, Kind: a.Kind, Name: a.Name, UUID: a.UUID,
		ServerURL: a.ServerURL, ExpiresAt: expires, Label: label,
	}
}

type database struct {
	Accounts   []Account `json:"accounts"`
	SelectedID string    `json:"selectedId"`
}

// Store manages persisted accounts.
type Store struct {
	mu       sync.RWMutex
	secrets  secretstore.Store
	accounts []Account
	selected string
}

const accountsSecretName = "accounts"

// Open is a file-backed helper used by tests. The desktop application uses
// OpenWithSecrets with the registry/OS secret store.
func Open(path, defaultPlayerName string) (*Store, error) {
	secrets, err := secretstore.NewFileStore(filepath.Join(filepath.Dir(path), "secrets"))
	if err != nil {
		return nil, err
	}
	return OpenWithSecrets(secrets, path, defaultPlayerName)
}

// OpenWithSecrets loads accounts from encrypted storage, migrating a legacy
// plaintext accounts.json if it exists.
func OpenWithSecrets(secrets secretstore.Store, legacyPath, defaultPlayerName string) (*Store, error) {
	if secrets == nil {
		secrets = secretstore.NewMemory()
	}
	s := &Store{secrets: secrets}
	if data := strings.TrimSpace(defaultPlayerName); data == "" {
		defaultPlayerName = "Steve"
	}
	raw, err := secrets.Get(accountsSecretName)
	if err != nil {
		return nil, fmt.Errorf("读取账户数据失败: %w", err)
	}
	var db database
	if strings.TrimSpace(raw) == "" {
		if strings.TrimSpace(legacyPath) != "" {
			if err := util.ReadJSON(legacyPath, &db); err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("读取账户数据失败: %w", err)
			}
			if len(db.Accounts) > 0 {
				s.accounts = db.Accounts
				s.selected = db.SelectedID
				if err := s.saveLocked(); err != nil {
					return nil, err
				}
			}
		}
	} else if err := json.Unmarshal([]byte(raw), &db); err != nil {
		return nil, fmt.Errorf("解析账户数据失败: %w", err)
	} else {
		s.accounts = db.Accounts
		s.selected = db.SelectedID
	}
	if len(s.accounts) == 0 {
		offline := newOfflineAccount(defaultPlayerName)
		s.accounts = []Account{offline}
		s.selected = offline.ID
		if err := s.saveLocked(); err != nil {
			return nil, err
		}
	}
	if s.findLocked(s.selected) == nil {
		s.selected = s.accounts[0].ID
	}
	// Remove any plaintext account file once the encrypted copy is in place.
	removeLegacyAccounts(legacyPath)
	return s, nil
}

func removeLegacyAccounts(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		// Overwrite any remaining plaintext secrets if the file cannot be removed.
		_ = os.WriteFile(path, []byte("{}\n"), 0o600)
	}
}

// List returns all accounts without secrets.
func (s *Store) List() []View {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]View, 0, len(s.accounts))
	for _, account := range s.accounts {
		out = append(out, account.View())
	}
	return out
}

// SelectedID returns the active account ID.
func (s *Store) SelectedID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.selected
}

// Selected returns the active account.
func (s *Store) Selected() (Account, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, account := range s.accounts {
		if account.ID == s.selected {
			return account, true
		}
	}
	return Account{}, false
}

// Select switches the active account.
func (s *Store) Select(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.findLocked(id) == nil {
		return errors.New("账户不存在")
	}
	s.selected = id
	return s.saveLocked()
}

// Add inserts or updates an account and makes it active.
func (s *Store) Add(account Account) (Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if account.ID == "" {
		account.ID = randomID()
	}
	account.UUID = strings.ReplaceAll(strings.TrimSpace(account.UUID), "-", "")
	if account.AddedAt.IsZero() {
		account.AddedAt = time.Now()
	}
	// Re-authenticating the same Microsoft or third-party identity should
	// update the existing entry instead of creating duplicates.
	if account.Kind != KindOffline {
		for idx, existing := range s.accounts {
			if existing.Kind != account.Kind {
				continue
			}
			sameIdentity := existing.UUID != "" && strings.EqualFold(existing.UUID, account.UUID)
			if account.Kind == KindAuthlib {
				sameIdentity = sameIdentity && sameAuthlibServer(existing.ServerURL, account.ServerURL)
			}
			if sameIdentity {
				account.ID = existing.ID
				account.AddedAt = existing.AddedAt
				s.accounts[idx] = account
				s.selected = account.ID
				return account, s.saveLocked()
			}
		}
	}
	s.accounts = append(s.accounts, account)
	s.selected = account.ID
	return account, s.saveLocked()
}

// Update persists token changes for an existing account.
func (s *Store) Update(account Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for idx := range s.accounts {
		if s.accounts[idx].ID == account.ID {
			s.accounts[idx] = account
			return s.saveLocked()
		}
	}
	return errors.New("账户不存在")
}

// AddOfflineAccount creates a new offline identity (or selects the existing
// one with the same name) and makes it active.
func (s *Store) AddOfflineAccount(name string) (Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		return Account{}, errors.New("玩家名称不能为空")
	}
	if len([]rune(name)) > 16 {
		name = string([]rune(name)[:16])
	}
	for idx := range s.accounts {
		existing := &s.accounts[idx]
		if existing.Kind == KindOffline && strings.EqualFold(existing.Name, name) {
			s.selected = existing.ID
			return *existing, s.saveLocked()
		}
	}
	account := newOfflineAccount(name)
	s.accounts = append(s.accounts, account)
	s.selected = account.ID
	return account, s.saveLocked()
}

// RenameOfflineAccount changes an offline account's display name and UUID.
func (s *Store) RenameOfflineAccount(id, name string) (Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		return Account{}, errors.New("玩家名称不能为空")
	}
	if len([]rune(name)) > 16 {
		name = string([]rune(name)[:16])
	}
	account := s.findLocked(id)
	if account == nil || account.Kind != KindOffline {
		return Account{}, errors.New("离线账户不存在")
	}
	for _, other := range s.accounts {
		if other.ID != id && other.Kind == KindOffline && strings.EqualFold(other.Name, name) {
			return Account{}, errors.New("已存在同名离线账户")
		}
	}
	account.Name = name
	account.UUID = OfflineUUID(name)
	s.selected = account.ID
	return *account, s.saveLocked()
}

// OfflineName returns the offline profile name.
func (s *Store) OfflineName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if account := s.findOfflineLocked(); account != nil {
		return account.Name
	}
	return "Steve"
}

// Remove deletes a non-offline account and selects a fallback if necessary.
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	account := s.findLocked(id)
	if account == nil {
		return errors.New("账户不存在")
	}
	if len(s.accounts) <= 1 {
		return errors.New("至少需要保留一个账户")
	}
	next := make([]Account, 0, len(s.accounts)-1)
	for _, existing := range s.accounts {
		if existing.ID != id {
			next = append(next, existing)
		}
	}
	s.accounts = next
	if s.selected == id {
		s.selected = s.accounts[0].ID
	}
	return s.saveLocked()
}

func (s *Store) findLocked(id string) *Account {
	for idx := range s.accounts {
		if s.accounts[idx].ID == id {
			return &s.accounts[idx]
		}
	}
	return nil
}

func (s *Store) findOfflineLocked() *Account {
	for idx := range s.accounts {
		if s.accounts[idx].Kind == KindOffline {
			return &s.accounts[idx]
		}
	}
	return nil
}

func (s *Store) saveLocked() error {
	raw, err := json.Marshal(database{Accounts: s.accounts, SelectedID: s.selected})
	if err != nil {
		return err
	}
	return s.secrets.Set(accountsSecretName, string(raw))
}

// sameAuthlibServer treats the site root and its API location as the same
// identity provider, so password and OAuth2 logins update one account.
func sameAuthlibServer(left, right string) bool {
	left = strings.TrimRight(strings.TrimSpace(left), "/")
	right = strings.TrimRight(strings.TrimSpace(right), "/")
	if strings.EqualFold(left, right) {
		return true
	}
	leftURL, leftErr := url.Parse(left)
	rightURL, rightErr := url.Parse(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return leftURL.Host != "" && strings.EqualFold(leftURL.Host, rightURL.Host)
}

func newOfflineAccount(name string) Account {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Steve"
	}
	return Account{
		ID:      randomID(),
		Kind:    KindOffline,
		Name:    name,
		UUID:    OfflineUUID(name),
		AddedAt: time.Now(),
	}
}

// OfflineUUID generates the standard offline-mode UUID.
func OfflineUUID(name string) string {
	sum := md5.Sum([]byte("OfflinePlayer:" + name))
	sum[6] = (sum[6] & 0x0f) | 0x30
	sum[8] = (sum[8] & 0x3f) | 0x80
	return hex.EncodeToString(sum[:])
}

func randomID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// RandomClientToken creates an authlib client token.
func RandomClientToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
