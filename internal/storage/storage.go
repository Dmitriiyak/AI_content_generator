package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

type User struct {
	UserID               int64  `json:"user_id"`
	Username             string `json:"username"`
	AvailableGenerations int    `json:"available_generations"`
	TotalGenerations     int    `json:"total_generations"`
	IsPremium            bool   `json:"is_premium"`
}

type Storage struct {
	users map[int64]*User
	file  string
	mu    sync.RWMutex
}

func NewStorage(filename string) *Storage {
	return &Storage{
		users: make(map[int64]*User),
		file:  filename,
	}
}

func (s *Storage) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("ошибка чтения файла: %w", err)
	}

	if len(data) == 0 {
		return nil
	}

	if err := json.Unmarshal(data, &s.users); err != nil {
		return fmt.Errorf("ошибка парсинга JSON: %w", err)
	}

	return nil
}

func (s *Storage) save() error {
	data, err := json.MarshalIndent(s.users, "", "  ")
	if err != nil {
		return fmt.Errorf("ошибка маршалинга JSON: %w", err)
	}

	if err := os.WriteFile(s.file, data, 0644); err != nil {
		return fmt.Errorf("ошибка записи файла: %w", err)
	}

	return nil
}

func (s *Storage) GetUser(userID int64) *User {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if user, exists := s.users[userID]; exists {
		return &User{
			UserID:               user.UserID,
			Username:             user.Username,
			AvailableGenerations: user.AvailableGenerations,
			TotalGenerations:     user.TotalGenerations,
			IsPremium:            user.IsPremium,
		}
	}

	return &User{
		UserID:               userID,
		AvailableGenerations: 10,
		TotalGenerations:     0,
		IsPremium:            false,
	}
}

func (s *Storage) UpdateUser(user *User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.users[user.UserID] = user
	return s.save()
}

func (s *Storage) UseGeneration(userID int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, exists := s.users[userID]
	if !exists {
		user = &User{
			UserID:               userID,
			AvailableGenerations: 10,
			TotalGenerations:     0,
			IsPremium:            false,
		}
		s.users[userID] = user
	}

	if user.AvailableGenerations <= 0 {
		return false, nil
	}

	user.AvailableGenerations--
	user.TotalGenerations++

	if err := s.save(); err != nil {
		return false, err
	}

	return true, nil
}

func (s *Storage) AddGenerations(userID int64, count int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, exists := s.users[userID]
	if !exists {
		user = &User{
			UserID:               userID,
			AvailableGenerations: 10,
			TotalGenerations:     0,
			IsPremium:            false,
		}
		s.users[userID] = user
	}

	user.AvailableGenerations += count

	if count >= 25 {
		user.IsPremium = true
	}

	return s.save()
}

func (s *Storage) GetPricing() map[string]int {
	return map[string]int{
		"10 генераций":  99,
		"25 генераций":  199,
		"100 генераций": 499,
	}
}
