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
	PaymentID   string    `json:"payment_id"`
	UserID      int64     `json:"user_id"`
	PackageType string    `json:"package_type"`
	Price       int       `json:"price"`
	Status      string    `json:"status"` // pending, succeeded, canceled
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Generation struct {
	UserID    int64     `json:"user_id"`
	Keywords  string    `json:"keywords"`
	Timestamp time.Time `json:"timestamp"`
}

type Database struct {
	users            map[int64]*User
	purchases        []Purchase
	pendingPurchases map[string]*Purchase
	generations      []Generation
	file             string
	mu               sync.RWMutex
}

func NewDatabase(filename string) *Database {
	db := &Database{
		users:            make(map[int64]*User),
		purchases:        make([]Purchase, 0),
		pendingPurchases: make(map[string]*Purchase),
		generations:      make([]Generation, 0),
		file:             filename,
	}

	// Загружаем ожидающие покупки при создании
	db.loadPendingPurchases()

	return db
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

func (db *Database) loadPendingPurchases() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	data, err := os.ReadFile("pending_purchases.json")
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("ошибка чтения файла ожидающих покупок: %w", err)
	}

	if len(data) == 0 {
		return nil
	}

	if err := json.Unmarshal(data, &db.pendingPurchases); err != nil {
		return fmt.Errorf("ошибка парсинга JSON ожидающих покупок: %w", err)
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

	// Сохраняем ожидающие покупки
	if err := db.savePendingPurchases(); err != nil {
		return err
	}

	log.Printf("[DB] ✅ Данные успешно сохранены")
	return nil
}

func (db *Database) savePendingPurchases() error {
	data, err := json.MarshalIndent(db.pendingPurchases, "", "  ")
	if err != nil {
		return fmt.Errorf("ошибка маршалинга ожидающих покупок: %w", err)
	}

	tempFile := "pending_purchases.json.tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("ошибка записи временного файла: %w", err)
	}

	if err := os.Rename(tempFile, "pending_purchases.json"); err != nil {
		return fmt.Errorf("ошибка переименования файла: %w", err)
	}

	return nil
}

func (db *Database) AddPendingPurchase(purchase *Purchase) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.pendingPurchases[purchase.PaymentID] = purchase
	return db.savePendingPurchases()
}

func (db *Database) GetPendingPurchase(paymentID string) *Purchase {
	db.mu.RLock()
	defer db.mu.RUnlock()

	return db.pendingPurchases[paymentID]
}

func (db *Database) UpdatePurchaseStatus(paymentID, status string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	purchase, exists := db.pendingPurchases[paymentID]
	if !exists {
		return fmt.Errorf("покупка не найдена")
	}

	purchase.Status = status
	purchase.UpdatedAt = time.Now()

	// Если покупка завершена успешно, перемещаем ее в основную историю
	if status == "succeeded" {
		db.purchases = append(db.purchases, *purchase)
		delete(db.pendingPurchases, paymentID)
	}

	// Сохраняем оба файла
	if err := db.save(); err != nil {
		return err
	}

	return db.savePendingPurchases()
}

func (db *Database) GetUserPurchases(userID int64) []*Purchase {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var userPurchases []*Purchase
	for _, purchase := range db.pendingPurchases {
		if purchase.UserID == userID {
			userPurchases = append(userPurchases, purchase)
		}
	}
	return userPurchases
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

func (db *Database) GetAllUsers() []int64 {
	db.mu.RLock()
	defer db.mu.RUnlock()

	userIDs := make([]int64, 0, len(db.users))
	for userID := range db.users {
		userIDs = append(userIDs, userID)
	}
	return userIDs
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
		PaymentID:   fmt.Sprintf("manual_%d_%d", userID, time.Now().Unix()),
		UserID:      userID,
		PackageType: packageType,
		Price:       price,
		Status:      "succeeded",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
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
		// Создаем нового пользователя, если его нет
		user = &User{
			UserID:               userID,
			AvailableGenerations: 10 + count, // 10 бесплатных + добавленные
			TotalGenerations:     0,
			CreatedAt:            time.Now(),
			GenerationsCount:     0,
		}
		db.users[userID] = user
	} else {
		user.AvailableGenerations += count
	}

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

// Исправленная функция статистики
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
		"all_time":          db.calcPeriodStats(time.Time{}, now),
		"last_month":        db.calcPeriodStats(monthAgo, now),
		"last_24h":          db.calcPeriodStats(dayAgo, now),
		"total_users":       len(db.users),
		"pending_purchases": len(db.pendingPurchases),
	}

	return stats
}

func (db *Database) calcPeriodStats(from, to time.Time) map[string]interface{} {
	stats := map[string]interface{}{
		"users":         0,
		"new_users":     0,
		"generations":   0,
		"purchases_10":  0,
		"purchases_25":  0,
		"purchases_100": 0,
		"revenue_10":    0,
		"revenue_25":    0,
		"revenue_100":   0,
		"total_revenue": 0,
	}

	// Подсчет пользователей
	allUsersCount := 0
	newUsersCount := 0

	for _, user := range db.users {
		allUsersCount++
		if (from.IsZero() || user.CreatedAt.After(from)) && (to.IsZero() || user.CreatedAt.Before(to)) {
			newUsersCount++
		}
	}

	stats["users"] = allUsersCount
	stats["new_users"] = newUsersCount

	// Подсчет покупок (только успешные)
	for _, purchase := range db.purchases {
		if purchase.Status == "succeeded" && purchase.CreatedAt.After(from) && (to.IsZero() || purchase.CreatedAt.Before(to)) {
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

func (db *Database) CancelAllPendingPurchases(userID int64) {
	db.mu.Lock()
	defer db.mu.Unlock()

	for paymentID, purchase := range db.pendingPurchases {
		if purchase.UserID == userID && purchase.Status == "pending" {
			purchase.Status = "canceled"
			purchase.UpdatedAt = time.Now()
			db.pendingPurchases[paymentID] = purchase
		}
	}
	db.savePendingPurchases()
}
