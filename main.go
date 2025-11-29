package main

import (
	"AIGenerator/internal/ai"
	"AIGenerator/internal/analyzer"
	"AIGenerator/internal/auth"
	"AIGenerator/internal/bot"
	"AIGenerator/internal/news"
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

		// === ИНИЦИАЛИЗАЦИЯ YANDEXGPT КЛИЕНТА ===
		fmt.Println("\n🔧 Инициализируем YandexGPT клиент...")
		gptClient, err := ai.NewYandexGPTClient()
		if err != nil {
			log.Printf("❌ YandexGPT клиент не создан: %v", err)
			fmt.Println("❌ YandexGPT клиент не создан. Проверьте переменные в .env:")
			fmt.Println("   - YANDEX_GPT_API_KEY")
			fmt.Println("   - YANDEX_FOLDER_ID")
			log.Fatal("Приложение остановлено")
		}

		// Тестируем подключение к YandexGPT
		fmt.Println("🧪 Тестируем подключение к YandexGPT...")
		if err := gptClient.TestConnection(context.Background()); err != nil {
			log.Printf("❌ Ошибка подключения к YandexGPT: %v", err)
			fmt.Println("❌ YandexGPT не доступен. Проверьте:")
			fmt.Println("1. Правильность API ключа и Folder ID в .env")
			fmt.Println("2. Доступ к интернету")
			fmt.Println("3. Активность аккаунта Yandex Cloud")
			fmt.Println("4. Активирован ли YandexGPT API в консоли")
			log.Fatal("Приложение остановлено")
		}

		fmt.Println("✅ YandexGPT подключен успешно!")

		// === СОЗДАЕМ АНАЛИЗАТОР КАНАЛОВ И НОВОСТНОЙ АГРЕГАТОР ===
		fmt.Println("\n🔧 Инициализируем анализатор каналов...")

		// Создаем анализатор каналов с nil клиентом (будем использовать тестовые данные)
		channelAnalyzer := analyzer.NewChannelAnalyzer(nil, gptClient)

		fmt.Println("🔧 Инициализируем новостной агрегатор...")
		newsAggregator := news.NewNewsAggregator(gptClient)
		newsAggregator.AddDefaultSources()

		// === ЗАПУСК TELEGRAM БОТА ===
		fmt.Println("\n🤖 Запускаем Telegram бота...")

		botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
		if botToken == "" {
			log.Printf("❌ TELEGRAM_BOT_TOKEN не установлен в .env")
			fmt.Println("❌ TELEGRAM_BOT_TOKEN не установлен. Добавьте в .env:")
			fmt.Println("   TELEGRAM_BOT_TOKEN=ваш_токен_бота")
			log.Fatal("Приложение остановлено")
		}

		// Создаем бота
		telegramBot, err := bot.New(botToken, channelAnalyzer, newsAggregator, gptClient)
		if err != nil {
			log.Printf("❌ Ошибка создания бота: %v", err)
			fmt.Println("❌ Ошибка создания бота:", err)
			log.Fatal("Приложение остановлено")
		}

		fmt.Println("✅ Бот успешно создан!")

		// Создаем контекст с отменой для graceful shutdown
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Запускаем бота в отдельной горутине
		go func() {
			log.Printf("🤖 Запуск Telegram бота...")
			telegramBot.Start(ctx)
		}()

		fmt.Println("\n🎉 Система полностью готова к работе!")
		fmt.Println("📱 Бот запущен и ожидает команд:")
		fmt.Println("   /start - начать работу")
		fmt.Println("   /help - справка по командам")
		fmt.Println("   /generate @username - создать пост для канала")

		select {}

		return nil
	}); err != nil {
		log.Fatalf("Ошибка запуска клиента: %v", err)
	}
}
