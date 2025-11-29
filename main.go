package main

import (
	"AIGenerator/internal/ai"
	"AIGenerator/internal/analyzer"
	"AIGenerator/internal/auth"
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/joho/godotenv"
)

func Setup_logger() *os.File {
	// Настраивает файл для логов
	file, err := os.OpenFile("logs.txt", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal("Ошибка настройки логгера (см. Setup_logger в main.go)")
	}
	return file
}

func main() {

	// Настройка логгера (запись в файл logs.txt)
	log_file := Setup_logger()
	defer log_file.Close()
	log.SetOutput(log_file)
	log.Printf("Логгер успешно запущен!\n")

	// Загружаем переменные окружения
	if err := godotenv.Load(); err != nil {
		log.Fatal("Ошибка загрузки переменных окружения (см. main.go))")
	}

	log.Printf("Переменные окружения загружены успешно")

	// Парсим переменные окружения
	apiID, err := strconv.Atoi(os.Getenv("API_ID"))
	if err != nil {
		log.Fatal("Неверный API_ID (см. main.go): ", err)
	}

	apiHash := os.Getenv("API_HASH")
	if apiHash == "" {
		log.Fatal("API_HASH не установлен (см. main.go)")
	}

	log.Printf("Успешный парсинг переменных окружения")

	// Создаем папку для сессии если её нет
	if err := os.MkdirAll("tdsession", 0700); err != nil {
		log.Fatal("Ошибка создания папки сессии: ", err)
	}

	// Создаем клиент Telegram с хранилищем сессии
	client := telegram.NewClient(apiID, apiHash, telegram.Options{
		SessionStorage: &session.FileStorage{
			Path: "tdsession/session.json",
		},
	})

	ctx := context.Background()

	// Запускаем клиент и аутентификацию
	log.Printf("Запускаем Telegram клиент...")
	if err := client.Run(ctx, func(ctx context.Context) error {
		if err := auth.Authenticate(ctx, client); err != nil {
			return fmt.Errorf("аутентификация не удалась: %w", err)
		}

		log.Printf("Аутентификация завершена успешно")
		fmt.Println("Аутентификация завершена успешно!")

		// === ТЕСТИРОВАНИЕ YANDEXGPT КЛИЕНТА ===
		fmt.Println("\n🔧 Инициализируем YandexGPT клиент...")
		gptClient, err := ai.NewYandexGPTClient()
		if err != nil {
			log.Printf("❌ YandexGPT клиент не создан: %v", err)
			fmt.Println("❌ YandexGPT клиент не создан. Проверьте переменные в .env:")
			fmt.Println("   - YANDEX_GPT_API_KEY")
			fmt.Println("   - YANDEX_FOLDER_ID")
			return nil
		}

		channelAnalyzer := analyzer.NewChannelAnalyzer(client.API(), gptClient)

		// Тестовый анализ
		testAnalysis, err := channelAnalyzer.AnalyzeChannel(ctx, "tproger")
		if err != nil {
			log.Printf("❌ Ошибка анализа канала: %v", err)
		} else {
			fmt.Printf("✅ AI-анализ завершен!\n")
			fmt.Printf("   Канал: %s (@%s)\n", testAnalysis.ChannelInfo.Title, testAnalysis.ChannelInfo.Username)
			fmt.Printf("   Основная тема: %s\n", testAnalysis.GPTAnalysis.MainTopic)
			fmt.Printf("   Подтемы: %v\n", testAnalysis.GPTAnalysis.Subtopics)
		}

		// Оставляем программу работать
		<-ctx.Done()

		return nil
	}); err != nil {
		log.Fatalf("Ошибка запуска клиента: %v", err)
	}
}
