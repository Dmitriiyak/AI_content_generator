package database

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

type User struct {
	UserID               int64     `json:"user_id"`
	Username             string    `json:"username"`
	AvailableGenerations int       `json:"available_generations"`
	TotalGenerations     int       `json:"total_generations"`
	CreatedAt            time.Time `json:"created_at"`
	LastGenerate         time.Time `json:"last_generate"`
	PendingFeedback      bool      `json:"pending_feedback,omitempty"`
	GenerationsCount     int       `json:"generations_count,omitempty"`
	LastFeedbackReminder time.Time `json:"last_feedback_reminder,omitempty"`
}

type Purchase struct {
	UserID      int64     `json:"user_id"`
	PackageType string    `json:"package_type"`
	Price       int       `json:"price"`
	Timestamp   time.Time `json:"timestamp"`
}

type Generation struct {
	UserID    int64     `json:"user_id"`
	Keywords  string    `json:"keywords"`
	Timestamp time.Time `json:"timestamp"`
}

type Database struct {
	users       map[int64]*User
	purchases   []Purchase
	generations []Generation
	file        string
	mu          sync.RWMutex
}

func NewDatabase(filename string) *Database {
	return &Database{
		users:       make(map[int64]*User),
		purchases:   make([]Purchase, 0),
		generations: make([]Generation, 0),
		file:        filename,
	}
}

func (db *Database) Load() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	data, err := os.ReadFile(db.file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("ошибка чтения файла: %w", err)
	}

	if len(data) == 0 {
		return nil
	}

	if err := json.Unmarshal(data, &db.users); err != nil {
		return fmt.Errorf("ошибка парсинга JSON: %w", err)
	}

	// Загружаем покупки
	purchaseData, err := os.ReadFile("purchases.json")
	if err == nil && len(purchaseData) > 0 {
		json.Unmarshal(purchaseData, &db.purchases)
	}

	// Загружаем историю генераций
	generationData, err := os.ReadFile("generations.json")
	if err == nil && len(generationData) > 0 {
		json.Unmarshal(generationData, &db.generations)
	}

	return nil
}

func (db *Database) save() error {
	// Сохраняем пользователей
	userData, err := json.MarshalIndent(db.users, "", "  ")
	if err != nil {
		log.Printf("[DB] ❌ Ошибка маршалинга пользователей: %v", err)
		return fmt.Errorf("ошибка маршалинга пользователей: %w", err)
	}

	tempFile := db.file + ".tmp"
	if err := os.WriteFile(tempFile, userData, 0644); err != nil {
		log.Printf("[DB] ❌ Ошибка записи временного файла: %v", err)
		return fmt.Errorf("ошибка записи временного файла: %w", err)
	}

	if err := os.Rename(tempFile, db.file); err != nil {
		log.Printf("[DB] ❌ Ошибка переименования файла: %v", err)
		return fmt.Errorf("ошибка переименования файла: %w", err)
	}

	// Сохраняем покупки
	purchaseData, err := json.MarshalIndent(db.purchases, "", "  ")
	if err != nil {
		log.Printf("[DB] ❌ Ошибка маршалинга покупок: %v", err)
		return fmt.Errorf("ошибка маршалинга покупок: %w", err)
	}

	if err := os.WriteFile("purchases.json", purchaseData, 0644); err != nil {
		log.Printf("[DB] ❌ Ошибка записи файла покупок: %v", err)
		return fmt.Errorf("ошибка записи файла покупок: %w", err)
	}

	// Сохраняем историю генераций
	generationData, err := json.MarshalIndent(db.generations, "", "  ")
	if err != nil {
		log.Printf("[DB] ❌ Ошибка маршалинга истории генераций: %v", err)
		return fmt.Errorf("ошибка маршалинга истории генераций: %w", err)
	}

	if err := os.WriteFile("generations.json", generationData, 0644); err != nil {
		log.Printf("[DB] ❌ Ошибка записи файла истории генераций: %v", err)
		return fmt.Errorf("ошибка записи файла истории генераций: %w", err)
	}

	log.Printf("[DB] ✅ Данные успешно сохранены")
	return nil
}

func (db *Database) AddGeneration(userID int64, keywords string) {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.generations = append(db.generations, Generation{
		UserID:    userID,
		Keywords:  keywords,
		Timestamp: time.Now(),
	})
}

func (db *Database) GetUser(userID int64) *User {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if user, exists := db.users[userID]; exists {
		return &User{
			UserID:               user.UserID,
			Username:             user.Username,
			AvailableGenerations: user.AvailableGenerations,
			TotalGenerations:     user.TotalGenerations,
			CreatedAt:            user.CreatedAt,
			LastGenerate:         user.LastGenerate,
			PendingFeedback:      user.PendingFeedback,
			GenerationsCount:     user.GenerationsCount,
			LastFeedbackReminder: user.LastFeedbackReminder,
		}
	}

	// Возвращаем нового пользователя, но не сохраняем его в базу до первого действия
	return &User{
		UserID:               userID,
		AvailableGenerations: 10,
		TotalGenerations:     0,
		CreatedAt:            time.Now(),
		GenerationsCount:     0,
	}
}

func (db *Database) UpdateUser(user *User) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.users[user.UserID] = user
	return db.save()
}

func (db *Database) UseGeneration(userID int64) (bool, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	log.Printf("[DB] UseGeneration для пользователя %d", userID)

	user, exists := db.users[userID]
	if !exists {
		log.Printf("[DB] Создаю нового пользователя %d", userID)
		user = &User{
			UserID:               userID,
			AvailableGenerations: 10,
			TotalGenerations:     0,
			CreatedAt:            time.Now(),
			GenerationsCount:     0,
		}
		db.users[userID] = user
	}

	log.Printf("[DB] Пользователь %d: доступно %d генераций", userID, user.AvailableGenerations)

	if user.AvailableGenerations <= 0 {
		log.Printf("[DB] У пользователя %d нет доступных генераций", userID)
		return false, nil
	}

	user.AvailableGenerations--
	user.TotalGenerations++
	user.LastGenerate = time.Now()

	log.Printf("[DB] После списания: доступно %d, всего использовано %d",
		user.AvailableGenerations, user.TotalGenerations)

	if err := db.save(); err != nil {
		log.Printf("[DB] ❌ Ошибка сохранения: %v", err)
		return false, err
	}

	log.Printf("[DB] ✅ Генерация успешно использована для пользователя %d", userID)
	return true, nil
}

func (db *Database) IncrementGenerationsCount(userID int64) {
	db.mu.Lock()
	defer db.mu.Unlock()

	user, exists := db.users[userID]
	if !exists {
		return
	}

	user.GenerationsCount++
	db.save()
}

func (db *Database) ResetGenerationsCount(userID int64) {
	db.mu.Lock()
	defer db.mu.Unlock()

	user, exists := db.users[userID]
	if !exists {
		return
	}

	user.GenerationsCount = 0
	db.save()
}

func (db *Database) SetPendingFeedback(userID int64, pending bool) {
	db.mu.Lock()
	defer db.mu.Unlock()

	user, exists := db.users[userID]
	if !exists {
		user = &User{
			UserID:               userID,
			AvailableGenerations: 10,
			TotalGenerations:     0,
			CreatedAt:            time.Now(),
			GenerationsCount:     0,
		}
		db.users[userID] = user
	}

	user.PendingFeedback = pending
	db.save()
}

func (db *Database) IsUserPendingFeedback(userID int64) bool {
	db.mu.RLock()
	defer db.mu.RUnlock()

	user, exists := db.users[userID]
	if !exists {
		return false
	}

	return user.PendingFeedback
}

func (db *Database) ShouldRemindFeedback(userID int64) bool {
	db.mu.RLock()
	defer db.mu.RUnlock()

	user, exists := db.users[userID]
	if !exists {
		return false
	}

	// Напоминаем каждые 3 генерации
	if user.GenerationsCount >= 3 && !user.PendingFeedback {
		// Проверяем, когда последний раз напоминали
		if time.Since(user.LastFeedbackReminder) > 24*time.Hour {
			user.LastFeedbackReminder = time.Now()
			return true
		}
	}

	return false
}

func (db *Database) AddPurchase(userID int64, packageType string, price int) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	log.Printf("[DB] Добавление покупки для пользователя %d: пакет %s, цена %d",
		userID, packageType, price)

	// Добавляем покупку в историю
	db.purchases = append(db.purchases, Purchase{
		UserID:      userID,
		PackageType: packageType,
		Price:       price,
		Timestamp:   time.Now(),
	})

	// Получаем или создаем пользователя
	user, exists := db.users[userID]
	if !exists {
		user = &User{
			UserID:               userID,
			AvailableGenerations: 10,
			TotalGenerations:     0,
			CreatedAt:            time.Now(),
			GenerationsCount:     0,
		}
		db.users[userID] = user
	}

	// Добавляем генерации в зависимости от пакета
	var generations int
	switch packageType {
	case "10":
		generations = 10
	case "25":
		generations = 25
	case "100":
		generations = 100
	default:
		generations = 10
	}

	user.AvailableGenerations += generations
	log.Printf("[DB] Пользователю %d добавлено %d генераций, теперь доступно %d",
		userID, generations, user.AvailableGenerations)

	// Сохраняем изменения
	if err := db.save(); err != nil {
		log.Printf("[DB] ❌ Ошибка сохранения покупки: %v", err)
		return err
	}

	log.Printf("[DB] ✅ Покупка успешно добавлена для пользователя %d", userID)
	return nil
}

func (db *Database) AddGenerations(userID int64, count int) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	log.Printf("[DB] Добавление %d генераций пользователю %d", count, userID)

	user, exists := db.users[userID]
	if !exists {
		user = &User{
			UserID:               userID,
			AvailableGenerations: 10,
			TotalGenerations:     0,
			CreatedAt:            time.Now(),
			GenerationsCount:     0,
		}
		db.users[userID] = user
	}

	user.AvailableGenerations += count
	log.Printf("[DB] Теперь у пользователя %d доступно %d генераций",
		userID, user.AvailableGenerations)

	if err := db.save(); err != nil {
		log.Printf("[DB] ❌ Ошибка сохранения: %v", err)
		return err
	}

	return nil
}

func (db *Database) GetPricing() map[string]int {
	return map[string]int{
		"10":  99,
		"25":  199,
		"100": 499,
	}
}

func (db *Database) GetStatistics(password string) map[string]interface{} {
	db.mu.RLock()
	defer db.mu.RUnlock()

	adminPassword := os.Getenv("STATISTICS_PASSWORD")
	if adminPassword == "" {
		adminPassword = "admin123"
	}

	if password != adminPassword {
		return nil
	}

	now := time.Now()
	dayAgo := now.Add(-24 * time.Hour)
	monthAgo := now.Add(-30 * 24 * time.Hour)

	stats := map[string]interface{}{
		"all_time":    db.calcPeriodStats(time.Time{}, now),
		"last_month":  db.calcPeriodStats(monthAgo, now),
		"last_24h":    db.calcPeriodStats(dayAgo, now),
		"total_users": len(db.users),
	}

	return stats
}

func (db *Database) calcPeriodStats(from, to time.Time) map[string]interface{} {
	stats := map[string]interface{}{
		"users":         0,
		"new_users":     0,
		"generations":   0, // Добавлено
		"purchases_10":  0,
		"purchases_25":  0,
		"purchases_100": 0,
		"revenue_10":    0,
		"revenue_25":    0,
		"revenue_100":   0,
		"total_revenue": 0,
	}

	// Подсчет покупок
	for _, purchase := range db.purchases {
		if purchase.Timestamp.After(from) && (to.IsZero() || purchase.Timestamp.Before(to)) {
			switch purchase.PackageType {
			case "10":
				stats["purchases_10"] = stats["purchases_10"].(int) + 1
				stats["revenue_10"] = stats["revenue_10"].(int) + purchase.Price
			case "25":
				stats["purchases_25"] = stats["purchases_25"].(int) + 1
				stats["revenue_25"] = stats["revenue_25"].(int) + purchase.Price
			case "100":
				stats["purchases_100"] = stats["purchases_100"].(int) + 1
				stats["revenue_100"] = stats["revenue_100"].(int) + purchase.Price
			}
		}
	}

	// Подсчет генераций
	for _, generation := range db.generations {
		if generation.Timestamp.After(from) && (to.IsZero() || generation.Timestamp.Before(to)) {
			stats["generations"] = stats["generations"].(int) + 1
		}
	}

	// Итоговая выручка
	totalRevenue := stats["revenue_10"].(int) + stats["revenue_25"].(int) + stats["revenue_100"].(int)
	stats["total_revenue"] = totalRevenue

	return stats
}

func (db *Database) GetTopGenerationTopics(from, to time.Time, limit int) map[string]int {
	db.mu.RLock()
	defer db.mu.RUnlock()

	topics := make(map[string]int)

	for _, generation := range db.generations {
		if generation.Timestamp.After(from) && (to.IsZero() || generation.Timestamp.Before(to)) {
			// Очищаем ключевые слова и приводим к нижнему регистру
			keywords := strings.ToLower(strings.TrimSpace(generation.Keywords))
			if keywords != "" {
				topics[keywords]++
			}
		}
	}

	return topics
}
