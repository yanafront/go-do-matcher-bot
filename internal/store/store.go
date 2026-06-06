package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type User struct {
	ChatID   int64  `json:"chat_id"`
	Name     string `json:"name"`
	Query    string `json:"query"`
	State    string `json:"state"`
	Active   bool   `json:"active"`
	Updated  string `json:"updated"`
}

type Vacancy struct {
	ChannelMsgID int    `json:"channel_msg_id"`
	Text         string `json:"text"`
	CreatedAt    string `json:"created_at"`
}

type Store struct {
	path string
	mu   sync.Mutex
	data diskState
}

type diskState struct {
	Users     map[string]User         `json:"users"`
	Vacancies []Vacancy               `json:"vacancies"`
	Sent      map[string]bool         `json:"sent"`
}

func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{
		path: filepath.Join(dataDir, "bot.json"),
		data: diskState{
			Users: make(map[string]User),
			Sent:  make(map[string]bool),
		},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.save()
}

func (s *Store) load() error {
	b, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(b) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := json.Unmarshal(b, &s.data); err != nil {
		return err
	}
	s.ensure()
	return nil
}

func (s *Store) save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensure()
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) ensure() {
	if s.data.Users == nil {
		s.data.Users = make(map[string]User)
	}
	if s.data.Sent == nil {
		s.data.Sent = make(map[string]bool)
	}
}

func userKey(chatID int64) string {
	return fmt.Sprintf("%d", chatID)
}

func sentKey(chatID int64, msgID int) string {
	return fmt.Sprintf("%d:%d", chatID, msgID)
}

func (s *Store) GetUser(chatID int64) (User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensure()
	u, ok := s.data.Users[userKey(chatID)]
	return u, ok
}

func (s *Store) SaveUser(u User) error {
	s.mu.Lock()
	u.Updated = time.Now().UTC().Format(time.RFC3339)
	s.data.Users[userKey(u.ChatID)] = u
	s.mu.Unlock()
	return s.save()
}

func (s *Store) ActiveUsers() []User {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensure()
	var out []User
	for _, u := range s.data.Users {
		if u.Active && u.State == "ready" && u.Query != "" {
			out = append(out, u)
		}
	}
	return out
}

func (s *Store) AddVacancy(v Vacancy) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensure()
	for _, existing := range s.data.Vacancies {
		if existing.ChannelMsgID == v.ChannelMsgID {
			return false, nil
		}
	}
	s.data.Vacancies = append([]Vacancy{v}, s.data.Vacancies...)
	if len(s.data.Vacancies) > 500 {
		s.data.Vacancies = s.data.Vacancies[:500]
	}
	s.mu.Unlock()
	return true, s.save()
}

func (s *Store) RecentVacancies(limit int) []Vacancy {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > len(s.data.Vacancies) {
		limit = len(s.data.Vacancies)
	}
	out := make([]Vacancy, limit)
	copy(out, s.data.Vacancies[:limit])
	return out
}

func (s *Store) WasSent(chatID int64, msgID int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensure()
	return s.data.Sent[sentKey(chatID, msgID)]
}

func (s *Store) MarkSent(chatID int64, msgID int) error {
	s.mu.Lock()
	s.data.Sent[sentKey(chatID, msgID)] = true
	s.mu.Unlock()
	return s.save()
}
