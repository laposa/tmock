// Package authtest provides an in-memory session store, so that handler tests can log
// users in without a database.
package authtest

import (
	"context"
	"sync"
	"time"

	"github.com/laposa/tmock/app-go/internal/store"
)

// Store is an in-memory auth.Store. It behaves like store.Sessions: lookups join the
// current user, so DeleteUser and SetAdmin take effect on the next request.
type Store struct {
	mu       sync.Mutex
	users    map[int64]store.User
	sessions map[string]session
}

type session struct {
	userID  int64
	expires time.Time
}

// New returns an empty store.
func New() *Store {
	return &Store{users: map[int64]store.User{}, sessions: map[string]session{}}
}

// AddUser adds or replaces a user.
func (s *Store) AddUser(u store.User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[u.ID] = u
}

// DeleteUser removes a user and, like the database's cascade, their sessions.
func (s *Store) DeleteUser(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.users, id)
	for k, v := range s.sessions {
		if v.userID == id {
			delete(s.sessions, k)
		}
	}
}

// Len returns the number of stored sessions, expired ones included.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

func (s *Store) Create(_ context.Context, id []byte, userID int64, expires time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return store.ErrForeignKey
	}
	if _, ok := s.sessions[string(id)]; ok {
		return store.ErrUnique
	}
	s.sessions[string(id)] = session{userID: userID, expires: expires}
	return nil
}

func (s *Store) User(_ context.Context, id []byte, now time.Time) (store.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[string(id)]
	if !ok || !sess.expires.After(now) {
		return store.User{}, store.ErrNotFound
	}
	u, ok := s.users[sess.userID]
	if !ok {
		return store.User{}, store.ErrNotFound
	}
	return u, nil
}

func (s *Store) Delete(_ context.Context, id []byte) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[string(id)]
	if !ok {
		return 0, store.ErrNotFound
	}
	delete(s.sessions, string(id))
	return sess.userID, nil
}

func (s *Store) DeleteExpired(_ context.Context, now time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int64
	for k, v := range s.sessions {
		if !v.expires.After(now) {
			delete(s.sessions, k)
			n++
		}
	}
	return n, nil
}
