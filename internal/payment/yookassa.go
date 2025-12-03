package payment

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
)

// YooMoneyClient клиент для работы с API ЮKassa
type YooMoneyClient struct {
	shopID     string
	secretKey  string
	baseURL    string
	httpClient *http.Client
}

// PaymentRequest запрос на создание платежа
type PaymentRequest struct {
	Amount struct {
		Value    string `json:"value"`
		Currency string `json:"currency"`
	} `json:"amount"`
	Capture      bool   `json:"capture"`
	Description  string `json:"description"`
	Confirmation struct {
		Type      string `json:"type"`
		ReturnURL string `json:"return_url"`
	} `json:"confirmation"`
	Metadata map[string]interface{} `json:"metadata"`
	Receipt  *Receipt               `json:"receipt,omitempty"`
}

// Receipt структура для фискального чека (54-ФЗ)
type Receipt struct {
	Customer struct {
		Email string `json:"email,omitempty"`
	} `json:"customer"`
	Items []ReceiptItem `json:"items"`
}

// ReceiptItem элемент чека
type ReceiptItem struct {
	Description string `json:"description"`
	Quantity    string `json:"quantity"`
	Amount      struct {
		Value    string `json:"value"`
		Currency string `json:"currency"`
	} `json:"amount"`
	VatCode        int    `json:"vat_code"`
	PaymentSubject string `json:"payment_subject,omitempty"`
	PaymentMode    string `json:"payment_mode,omitempty"`
}

// PaymentResponse ответ от API ЮKassa
type PaymentResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Amount struct {
		Value    string `json:"value"`
		Currency string `json:"currency"`
	} `json:"amount"`
	Description  string `json:"description"`
	Confirmation struct {
		ConfirmationURL string `json:"confirmation_url"`
	} `json:"confirmation"`
	Metadata map[string]interface{} `json:"metadata"`
	Paid     bool                   `json:"paid"`
}

// NewYooMoneyClient создает новый клиент ЮKassa
func NewYooMoneyClient() (*YooMoneyClient, error) {
	shopID := os.Getenv("YOOMONEY_SHOP_ID")
	secretKey := os.Getenv("YOOMONEY_SECRET_KEY")

	if shopID == "" {
		log.Println("[YOOMONEY] ⚠️ YOOMONEY_SHOP_ID не установлен")
	}
	if secretKey == "" {
		log.Println("[YOOMONEY] ⚠️ YOOMONEY_SECRET_KEY не установлен")
	}

	if shopID == "" || secretKey == "" {
		return nil, fmt.Errorf("YOOMONEY_SHOP_ID или YOOMONEY_SECRET_KEY не установлены")
	}

	log.Printf("[YOOMONEY] Клиент создан с shopID: %s", shopID)

	return &YooMoneyClient{
		shopID:    shopID,
		secretKey: secretKey,
		baseURL:   "https://api.yookassa.ru/v3/",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// CreatePayment создает новый платеж
func (c *YooMoneyClient) CreatePayment(amount float64, description string, userID int64, packageType string, count int) (*PaymentResponse, error) {
	url := c.baseURL + "payments"
	log.Printf("[YOOMONEY] Создание платежа: %.2f RUB, описание: %s", amount, description)

	// Генерируем уникальный ключ идемпотентности
	idempotenceKey := uuid.New().String()
	log.Printf("[YOOMONEY] Idempotence-Key: %s", idempotenceKey)

	// Создаем запрос
	paymentReq := PaymentRequest{}
	paymentReq.Amount.Value = fmt.Sprintf("%.2f", amount)
	paymentReq.Amount.Currency = "RUB"
	paymentReq.Capture = true
	paymentReq.Description = description
	paymentReq.Confirmation.Type = "redirect"
	paymentReq.Confirmation.ReturnURL = os.Getenv("YOOMONEY_RETURN_URL")

	// Устанавливаем возвратный URL
	if paymentReq.Confirmation.ReturnURL == "" {
		paymentReq.Confirmation.ReturnURL = "https://t.me/"
		log.Printf("[YOOMONEY] Return URL не установлен, используется: %s", paymentReq.Confirmation.ReturnURL)
	}

	// Устанавливаем метаданные
	paymentReq.Metadata = map[string]interface{}{
		"user_id":      userID,
		"package_type": packageType,
		"count":        count,
		"created_at":   time.Now().Format(time.RFC3339),
	}

	// Добавляем фискальный чек (обязательно для РФ)
	paymentReq.Receipt = &Receipt{
		Customer: struct {
			Email string `json:"email,omitempty"`
		}{
			Email: "noreply@example.com", // Требуется email, можно использовать заглушку
		},
		Items: []ReceiptItem{
			{
				Description: description,
				Quantity:    "1",
				Amount: struct {
					Value    string `json:"value"`
					Currency string `json:"currency"`
				}{
					Value:    fmt.Sprintf("%.2f", amount),
					Currency: "RUB",
				},
				VatCode:        4,              // 20% НДС (код 4)
				PaymentSubject: "service",      // Услуга
				PaymentMode:    "full_payment", // Полная предоплата
			},
		},
	}

	jsonData, err := json.Marshal(paymentReq)
	if err != nil {
		log.Printf("[YOOMONEY] ❌ Ошибка маршалинга запроса: %v", err)
		return nil, fmt.Errorf("ошибка маршалинга: %w", err)
	}

	log.Printf("[YOOMONEY] JSON запрос: %s", string(jsonData))

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("[YOOMONEY] ❌ Ошибка создания запроса: %v", err)
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}

	// Базовая аутентификация
	req.SetBasicAuth(c.shopID, c.secretKey)
	req.Header.Set("Idempotence-Key", idempotenceKey)
	req.Header.Set("Content-Type", "application/json")

	log.Printf("[YOOMONEY] Отправка запроса на %s", url)
	log.Printf("[YOOMONEY] Auth header: Basic %s:****", c.shopID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[YOOMONEY] ❌ Ошибка отправки запроса: %v", err)
		return nil, fmt.Errorf("ошибка отправки запроса: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[YOOMONEY] ❌ Ошибка чтения ответа: %v", err)
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	log.Printf("[YOOMONEY] Ответ от API: статус %d", resp.StatusCode)
	log.Printf("[YOOMONEY] Тело ответа: %s", string(body))

	if resp.StatusCode != http.StatusOK {
		log.Printf("[YOOMONEY] ❌ Ошибка API: статус %d", resp.StatusCode)

		// Пробуем распарсить ошибку
		var errorResp struct {
			Type        string `json:"type"`
			ID          string `json:"id"`
			Code        string `json:"code"`
			Description string `json:"description"`
			Parameter   string `json:"parameter"`
		}

		if err := json.Unmarshal(body, &errorResp); err == nil && errorResp.Description != "" {
			log.Printf("[YOOMONEY] Ошибка ЮKassa: %s (код: %s)", errorResp.Description, errorResp.Code)
			return nil, fmt.Errorf("ошибка ЮKassa: %s", errorResp.Description)
		}

		return nil, fmt.Errorf("ошибка API: статус %d", resp.StatusCode)
	}

	var paymentResp PaymentResponse
	if err := json.Unmarshal(body, &paymentResp); err != nil {
		log.Printf("[YOOMONEY] ❌ Ошибка парсинга ответа: %v", err)
		return nil, fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	log.Printf("[YOOMONEY] ✅ Платеж создан: ID=%s, статус=%s", paymentResp.ID, paymentResp.Status)
	if paymentResp.Confirmation.ConfirmationURL != "" {
		log.Printf("[YOOMONEY] URL для оплаты: %s", paymentResp.Confirmation.ConfirmationURL)
	}

	return &paymentResp, nil
}

// CheckPayment проверяет статус платежа
func (c *YooMoneyClient) CheckPayment(paymentID string) (*PaymentResponse, error) {
	url := c.baseURL + "payments/" + paymentID
	log.Printf("[YOOMONEY] Проверка статуса платежа: %s", paymentID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("[YOOMONEY] ❌ Ошибка создания запроса: %v", err)
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.SetBasicAuth(c.shopID, c.secretKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[YOOMONEY] ❌ Ошибка отправки запроса: %v", err)
		return nil, fmt.Errorf("ошибка отправки запроса: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[YOOMONEY] ❌ Ошибка чтения ответа: %v", err)
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("[YOOMONEY] ❌ Ошибка API при проверке: статус %d, тело: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("ошибка API: статус %d", resp.StatusCode)
	}

	var paymentResp PaymentResponse
	if err := json.Unmarshal(body, &paymentResp); err != nil {
		log.Printf("[YOOMONEY] ❌ Ошибка парсинга ответа: %v", err)
		return nil, fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	log.Printf("[YOOMONEY] Статус платежа %s: %s", paymentID, paymentResp.Status)
	return &paymentResp, nil
}

// CancelPayment отменяет платеж
func (c *YooMoneyClient) CancelPayment(paymentID string) error {
	url := c.baseURL + "payments/" + paymentID + "/cancel"
	log.Printf("[YOOMONEY] Отмена платежа: %s", paymentID)

	// Генерируем новый ключ идемпотентности для отмены
	idempotenceKey := uuid.New().String()

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		log.Printf("[YOOMONEY] ❌ Ошибка создания запроса: %v", err)
		return fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.SetBasicAuth(c.shopID, c.secretKey)
	req.Header.Set("Idempotence-Key", idempotenceKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[YOOMONEY] ❌ Ошибка отправки запроса: %v", err)
		return fmt.Errorf("ошибка отправки запроса: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[YOOMONEY] ❌ Ошибка API при отмене: статус %d, тело: %s", resp.StatusCode, string(body))
		return fmt.Errorf("ошибка API: статус %d", resp.StatusCode)
	}

	log.Printf("[YOOMONEY] ✅ Платеж %s отменен", paymentID)
	return nil
}
