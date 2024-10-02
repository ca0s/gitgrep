package gitdown

import "sync"

type AuthPair struct {
	Name  string
	Value string
}

type AuthStorage struct {
	auths map[string]*AuthPair
	lock  sync.RWMutex
}

func NewAuthStorage() *AuthStorage {
	return &AuthStorage{
		auths: make(map[string]*AuthPair),
		lock:  sync.RWMutex{},
	}
}

func (s *AuthStorage) SetSiteAuth(site string, name string, value string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.auths[site] = &AuthPair{
		Name:  name,
		Value: value,
	}
}

func (s *AuthStorage) GetSiteAuth(site string) *AuthPair {
	s.lock.RLock()
	defer s.lock.RUnlock()

	auth, ok := s.auths[site]
	if !ok {
		return nil
	}

	return auth
}
