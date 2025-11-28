package main

import (
	"log"
	"os"

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

}
