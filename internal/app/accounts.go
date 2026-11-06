package app

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"mcl/internal/core/auth"
	"mcl/internal/core/launch"
)

type Accounts struct {
	Profiles []auth.Profile `json:"profiles"`
	ActiveID string         `json:"activeId"`
}

func (a Accounts) Active() (auth.Profile, bool) { return a.find(a.ActiveID) }

func (a Accounts) find(id string) (auth.Profile, bool) {
	for _, p := range a.Profiles {
		if p.ID() == id {
			return p, true
		}
	}
	return auth.Profile{}, false
}

func (a Accounts) Validate() error {
	if len(a.Profiles) > 64 {
		return errors.New("最多可保存 64 个账户")
	}
	seen := make(map[string]bool, len(a.Profiles))
	for _, p := range a.Profiles {
		if err := p.Validate(); err != nil {
			return err
		}
		id := p.ID()
		if seen[id] {
			return errors.New("账户列表包含重复玩家")
		}
		seen[id] = true
	}
	if (len(a.Profiles) != 0 || a.ActiveID != "") && !seen[a.ActiveID] {
		return errors.New("当前账户在账户列表中不存在")
	}
	return nil
}

type AccountRepository interface {
	Load(context.Context) (Accounts, error)
	Lock(context.Context) (release func(), err error)
	Save(context.Context, Accounts) error
}

var ErrCredentialMissing = errors.New("微软登录凭据不存在，请重新登录")

type AccountCredentials interface {
	Get(id string) (string, error)
	Set(id, refreshToken string) error
	Delete(id string) error
}

type AccountService struct {
	repository  AccountRepository
	credentials AccountCredentials
	microsoft   *auth.MicrosoftClient
	sessions    sync.Map // Account ID to launch.Session
}

func NewAccountService(repository AccountRepository, credentials AccountCredentials, microsoft *auth.MicrosoftClient) *AccountService {
	return &AccountService{repository: repository, credentials: credentials, microsoft: microsoft}
}

func (s *AccountService) Load(ctx context.Context) (Accounts, error) {
	return s.repository.Load(ctx)
}

func (s *AccountService) AddOffline(ctx context.Context, name string) (Accounts, error) {
	profile, err := auth.OfflineProfile(name)
	if err != nil {
		return Accounts{}, err
	}
	return s.change(ctx, func(accounts *Accounts) error {
		if _, exists := accounts.find(profile.ID()); !exists {
			accounts.Profiles = append(accounts.Profiles, profile)
		}
		accounts.ActiveID = profile.ID()
		return s.repository.Save(ctx, *accounts)
	})
}

func (s *AccountService) Select(ctx context.Context, id string) (Accounts, error) {
	return s.change(ctx, func(accounts *Accounts) error {
		if _, exists := accounts.find(id); !exists {
			return errors.New("此账户已移除")
		}
		accounts.ActiveID = id
		return s.repository.Save(ctx, *accounts)
	})
}

func (s *AccountService) SetOfflineHead(ctx context.Context, id string, head []byte) (Accounts, error) {
	return s.change(ctx, func(accounts *Accounts) error {
		for i, p := range accounts.Profiles {
			if p.ID() == id && p.Kind == auth.Offline {
				accounts.Profiles[i].Head = bytes.Clone(head)
				return s.repository.Save(ctx, *accounts)
			}
		}
		return errors.New("离线账户已移除")
	})
}

func (s *AccountService) RefreshOfflineHead(ctx context.Context, id string, replace bool) ([]byte, error) {
	accounts, err := s.repository.Load(ctx)
	if err != nil {
		return nil, err
	}
	profile, exists := accounts.find(id)
	if !exists || profile.Kind != auth.Offline {
		return nil, errors.New("离线账户已移除")
	}
	if !replace && len(profile.Head) > 0 {
		return nil, nil
	}
	head, err := auth.PublicPlayerHead(ctx, profile.Name)
	if err != nil {
		return nil, err
	}
	// Network work stays outside the account lock. Keep any newer avatar.
	_, err = s.change(ctx, func(accounts *Accounts) error {
		for i, current := range accounts.Profiles {
			if current.ID() != id || current.Kind != auth.Offline {
				continue
			}
			if !bytes.Equal(current.Head, profile.Head) {
				return errors.New("账户头像已变更，请重试")
			}
			accounts.Profiles[i].Head = head
			return s.repository.Save(ctx, *accounts)
		}
		return errors.New("离线账户已移除")
	})
	if err != nil {
		return nil, err
	}
	return head, nil
}

func (s *AccountService) SignIn(ctx context.Context, expectedID string, openBrowser func(string) error) (Accounts, error) {
	if s.microsoft == nil {
		return Accounts{}, errors.New("微软登录需要 mcl 自有的应用 Client ID")
	}
	grant, err := s.microsoft.SignIn(ctx, openBrowser)
	if err != nil {
		return Accounts{}, err
	}
	id := grant.Profile.ID()
	if expectedID != "" && id != expectedID {
		return Accounts{}, errors.New("浏览器登录了另一个 Minecraft 玩家，请重试或选择添加账户")
	}
	return s.change(ctx, func(accounts *Accounts) error {
		old, exists := accounts.find(id)
		if expectedID != "" && !exists {
			return errors.New("此账户已移除，请重新添加")
		}
		token, err := s.credentials.Get(id)
		if err != nil && !errors.Is(err, ErrCredentialMissing) {
			return err
		}
		updateProfile(accounts, grant.Profile, old)
		accounts.ActiveID = id
		if err := s.storeGrant(ctx, *accounts, grant, token); err != nil {
			return err
		}
		s.sessions.Store(id, grant.Session)
		return nil
	})
}

func (s *AccountService) Remove(ctx context.Context, id string) (Accounts, error) {
	return s.change(ctx, func(accounts *Accounts) error {
		profile, exists := accounts.find(id)
		if !exists {
			return errors.New("此账户已移除")
		}
		accounts.Profiles = slices.DeleteFunc(accounts.Profiles, func(p auth.Profile) bool { return p.ID() == id })
		if accounts.ActiveID == id {
			accounts.ActiveID = ""
			if len(accounts.Profiles) > 0 {
				accounts.ActiveID = accounts.Profiles[0].ID()
			}
		}
		if profile.Kind == auth.Offline {
			return s.repository.Save(ctx, *accounts)
		}
		previous, err := s.credentials.Get(id)
		if err != nil && !errors.Is(err, ErrCredentialMissing) {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.credentials.Delete(id); err != nil {
			return err
		}
		s.sessions.Delete(id)
		if err := s.repository.Save(ctx, *accounts); err != nil {
			if previous != "" {
				return errors.Join(err, s.credentials.Set(id, previous))
			}
			return err
		}
		return nil
	})
}

func (s *AccountService) Session(ctx context.Context, id string) (launch.Session, error) {
	release, err := s.repository.Lock(ctx)
	if err != nil {
		return launch.Session{}, err
	}
	defer release()
	accounts, err := s.repository.Load(ctx)
	if err != nil {
		return launch.Session{}, err
	}
	profile, exists := accounts.find(id)
	if !exists {
		return launch.Session{}, errors.New("请先选择一个账户")
	}
	if profile.Kind == auth.Offline {
		return launch.NewOfflineSession(profile.Name, profile.UUID)
	}
	if s.microsoft == nil {
		return launch.Session{}, errors.New("请先配置 mcl 的微软应用 Client ID")
	}
	if cached, ok := s.sessions.Load(id); ok {
		session := cached.(launch.Session)
		if session.ExpiresAt.After(time.Now().Add(5*time.Minute)) && session.Name == profile.Name {
			return session, nil
		}
	}
	old, err := s.credentials.Get(id)
	if err != nil {
		return launch.Session{}, err
	}
	grant, err := s.microsoft.Refresh(ctx, old)
	if err != nil {
		return launch.Session{}, err
	}
	if grant.Profile.ID() != id {
		return launch.Session{}, errors.New("微软返回了另一个玩家，请重新登录")
	}
	updateProfile(&accounts, grant.Profile, profile)
	if err := s.storeGrant(ctx, accounts, grant, old); err != nil {
		return launch.Session{}, err
	}
	s.sessions.Store(id, grant.Session)
	return grant.Session, nil
}

func (s *AccountService) change(ctx context.Context, change func(*Accounts) error) (Accounts, error) {
	release, err := s.repository.Lock(ctx)
	if err != nil {
		return Accounts{}, err
	}
	defer release()
	accounts, err := s.repository.Load(ctx)
	if err != nil {
		return Accounts{}, err
	}
	if err := change(&accounts); err != nil {
		return Accounts{}, err
	}
	return accounts, nil
}

func (s *AccountService) storeGrant(ctx context.Context, accounts Accounts, grant auth.Grant, previous string) error {
	if err := accounts.Validate(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	id, refresh := grant.Profile.ID(), grant.RefreshToken()
	changed := refresh != previous
	if changed {
		if err := s.credentials.Set(id, refresh); err != nil {
			return err
		}
	}
	err := s.repository.Save(ctx, accounts)
	if err != nil && changed {
		if previous == "" {
			return errors.Join(err, s.credentials.Delete(id))
		}
		return errors.Join(err, s.credentials.Set(id, previous))
	}
	return err
}

func updateProfile(accounts *Accounts, profile, previous auth.Profile) {
	if len(profile.Head) == 0 {
		profile.Head = previous.Head
	}
	for index, p := range accounts.Profiles {
		if p.ID() == profile.ID() {
			accounts.Profiles[index] = profile
			return
		}
	}
	accounts.Profiles = append(accounts.Profiles, profile)
}
