package main

import (
	"AIGenerator/internal/ai"
	"AIGenerator/internal/bot"
	"AIGenerator/internal/database"
	"AIGenerator/internal/news"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	// Настройка логирования
	logFile, err := os.OpenFile("logs.txt", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("❌ Ошибка создания лог-файла: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	// Консольный вывод процесса запуска
	fmt.Println("=========================================")
	fmt.Println("🚀 ЗАПУСК AI CONTENT GENERATOR")
	fmt.Println("=========================================")

	// 1. Загрузка переменных окружения
	fmt.Println("[1/6] Загрузка .env файла...")
	if err := godotenv.Load(); err != nil {
		fmt.Println("⚠️  .env файл не найден, проверяю системные переменные")
	}

	// 2. Инициализация базы данных
	fmt.Println("[2/6] Инициализация базы данных...")
	db := database.NewDatabase("users.json")
	if err := db.Load(); err != nil {
		fmt.Printf("⚠️  Ошибка загрузки базы: %v\n", err)
		fmt.Println("📁 Создана новая база данных")
	} else {
		fmt.Println("✅ База данных загружена")
	}

	// 3. Инициализация YandexGPT
	fmt.Println("[3/6] Инициализация YandexGPT...")
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	yandexAPIKey := os.Getenv("YANDEX_GPT_API_KEY")
	yandexFolderID := os.Getenv("YANDEX_FOLDER_ID")

	// Проверка обязательных переменных
	if botToken == "" {
		fmt.Println("❌ ОШИБКА: TELEGRAM_BOT_TOKEN не установлен")
		fmt.Println("Добавьте в .env файл: TELEGRAM_BOT_TOKEN=ваш_токен_бота")
		os.Exit(1)
	}

	if yandexAPIKey == "" || yandexFolderID == "" {
		fmt.Println("❌ ОШИБКА: Переменные YandexGPT не установлены")
		fmt.Println("Добавьте в .env файл:")
		fmt.Println("YANDEX_GPT_API_KEY=ваш_api_ключ")
		fmt.Println("YANDEX_FOLDER_ID=ваш_folder_id")
		os.Exit(1)
	}

	gptClient, err := ai.NewYandexGPTClient()
	if err != nil {
		fmt.Printf("❌ ОШИБКА: Не удалось создать клиент YandexGPT: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ YandexGPT клиент создан")

	// 4. Инициализация новостного агрегатора
	fmt.Println("[4/6] Инициализация новостного агрегатора...")
	newsAggregator := news.NewNewsAggregator()
	newsAggregator.AddDefaultSources()
	fmt.Println("✅ Новостной агрегатор создан")

	// 5. Создание бота
	fmt.Println("[5/6] Создание Telegram бота...")
	telegramBot, err := bot.New(botToken, newsAggregator, gptClient, db)
	if err != nil {
		fmt.Printf("❌ ОШИБКА: Не удалось создать бота: %v\n", err)
		os.Exit(1)
	}

	// 6. Настройка graceful shutdown
	fmt.Println("[6/6] Настройка graceful shutdown...")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Обработка сигналов завершения
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Запуск бота в отдельной горутине
	go func() {
		fmt.Println("=========================================")
		fmt.Println("✅ ВСЕ СИСТЕМЫ ЗАПУЩЕНЫ УСПЕШНО!")
		fmt.Println("✨ Ожидание команд...")
		fmt.Println("=========================================")
		log.Println("[STARTUP] Бот успешно запущен")
		telegramBot.Start(ctx)
	}()

	// Ожидание сигнала завершения
	<-sigChan
	fmt.Println("\n🔄 Получен сигнал завершения...")
	cancel()
	time.Sleep(2 * time.Second)
	fmt.Println("👋 Бот завершил работу")
}
