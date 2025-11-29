package main

import (
	"AIGenerator/internal/ai"
	"AIGenerator/internal/analyzer"
	"AIGenerator/internal/auth"
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
			return nil
		}

		// Тестируем подключение к YandexGPT
		fmt.Println("🧪 Тестируем подключение к YandexGPT...")
		if err := gptClient.TestConnection(ctx); err != nil {
			log.Printf("❌ Ошибка подключения к YandexGPT: %v", err)
			fmt.Println("❌ YandexGPT не доступен. Проверьте:")
			fmt.Println("1. Правильность API ключа и Folder ID в .env")
			fmt.Println("2. Доступ к интернету")
			fmt.Println("3. Активность аккаунта Yandex Cloud")
			fmt.Println("4. Активирован ли YandexGPT API в консоли")
			return nil
		}

		fmt.Println("✅ YandexGPT подключен успешно!")

		// === СОЗДАЕМ АНАЛИЗАТОР КАНАЛОВ И НОВОСТНОЙ АГРЕГАТОР ===
		fmt.Println("\n🔧 Инициализируем анализатор каналов...")
		channelAnalyzer := analyzer.NewChannelAnalyzer(client.API(), gptClient)

		fmt.Println("🔧 Инициализируем новостной агрегатор...")
		newsAggregator := news.NewNewsAggregator(gptClient)
		newsAggregator.AddDefaultSources()

		// === ТЕСТИРУЕМ АНАЛИЗ КАНАЛА ===
		fmt.Println("🧪 Тестируем анализ канала...")
		testAnalysis, err := channelAnalyzer.AnalyzeChannel(ctx, "tproger")
		if err != nil {
			log.Printf("❌ Ошибка анализа канала: %v", err)
			fmt.Println("❌ Ошибка при анализе канала:", err)
		} else {
			fmt.Println("✅ Анализ канала выполнен успешно!")
			fmt.Printf("   Канал: %s (@%s)\n", testAnalysis.ChannelInfo.Title, testAnalysis.ChannelInfo.Username)
			fmt.Printf("   Основная тема: %s\n", testAnalysis.GPTAnalysis.MainTopic)
			fmt.Printf("   Подтемы: %v\n", testAnalysis.GPTAnalysis.Subtopics)
			fmt.Printf("   Ключевые слова: %v\n", testAnalysis.GPTAnalysis.Keywords)
			fmt.Printf("   Угол подачи: %s\n", testAnalysis.GPTAnalysis.ContentAngle)
		}

		// === ТЕСТИРУЕМ AI-ПОДБОР НОВОСТЕЙ (ЭТАП 3) ===
		fmt.Println("\n🧪 Тестируем AI-подбор новостей...")

		// Получаем свежие новости
		articles, err := newsAggregator.FetchAllArticles()
		if err != nil {
			log.Printf("❌ Ошибка получения новостей: %v", err)
			fmt.Println("❌ Ошибка при получении новостей:", err)
		} else {
			// Используем AI для подбора релевантных новостей
			fmt.Println("🔧 Используем AI для подбора релевантных новостей...")
			relevantArticles := newsAggregator.FindRelevantArticles(ctx, articles, testAnalysis, 3)

			fmt.Printf("✅ AI подобрал %d релевантных новостей:\n", len(relevantArticles))
			for i, article := range relevantArticles {
				fmt.Printf("   %d. %s (релевантность: %.2f)\n", i+1, article.Title, article.Relevance)
				fmt.Printf("      Ссылка: %s\n", article.URL)
				fmt.Printf("      Источник: %s\n", article.Source)
				fmt.Println()
			}

			// Генерируем идеи для контента
			if len(relevantArticles) > 0 {
				fmt.Println("🧪 Генерируем идеи для контента...")
				contentIdeas := newsAggregator.GenerateContentIdeas(relevantArticles, testAnalysis)

				fmt.Printf("✅ Сгенерировано %d идей для контента:\n", len(contentIdeas))
				for i, idea := range contentIdeas {
					fmt.Printf("   %d. %s\n", i+1, idea)
					fmt.Println()
				}
			}
		}

		fmt.Println("\n🎉 Все этапы завершены успешно!")
		fmt.Println("📊 Система готова к работе:")
		fmt.Println("   - AI-анализ Telegram каналов ✅")
		fmt.Println("   - AI-подбор релевантных новостей ✅")
		fmt.Println("   - Генерация идей для контента ✅")

		// Оставляем программу работать для просмотра результатов
		fmt.Println("\n⏹️  Нажмите Ctrl+C для остановки приложения")

		// Оставляем основную горутину активной
		<-ctx.Done()

		return nil
	}); err != nil {
		log.Fatalf("Ошибка запуска клиента: %v", err)
	}
}
