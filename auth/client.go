package auth

import (
	"context"
	"fmt"
	"log"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

// AuthFlow реализует полный интерфейс UserAuthenticator
type AuthFlow struct{}

// Phone запрашивает номер телефона у пользователя
func (a AuthFlow) Phone(ctx context.Context) (string, error) {
	fmt.Print("Введите номер телефона в международном формате (+79991234567): ")
	var phone string
	if _, err := fmt.Scanln(&phone); err != nil {
		return "", fmt.Errorf("ошибка ввода номера: %w", err)
	}
	log.Printf("Введен номер: %s", phone)
	return phone, nil
}

// Password запрашивает пароль двухфакторной аутентификации
func (a AuthFlow) Password(ctx context.Context) (string, error) {
	fmt.Print("Введите пароль двухфакторной аутентификации: ")
	var password string
	if _, err := fmt.Scanln(&password); err != nil {
		return "", fmt.Errorf("ошибка ввода пароля: %w", err)
	}
	log.Printf("Введен пароль 2FA")
	return password, nil
}

// Code запрашивает код подтверждения из Telegram
func (a AuthFlow) Code(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
	fmt.Print("Введите код из Telegram: ")
	var code string
	if _, err := fmt.Scanln(&code); err != nil {
		return "", fmt.Errorf("ошибка ввода кода: %w", err)
	}
	log.Printf("Введен код подтверждения")
	return code, nil
}

// AcceptTermsOfService обрабатывает принятие условий использования
func (a AuthFlow) AcceptTermsOfService(ctx context.Context, tos tg.HelpTermsOfService) error {
	log.Printf("Принимаем условия использования Telegram")
	// Автоматически принимаем условия
	return nil
}

// SignUp не используется в нашем случае, но требуется интерфейсом
func (a AuthFlow) SignUp(ctx context.Context) (auth.UserInfo, error) {
	log.Printf("SignUp вызван (не используется)")
	return auth.UserInfo{}, fmt.Errorf("регистрация не поддерживается")
}

// AcceptLoginToken не используется в нашем случае, но требуется интерфейсом
func (a AuthFlow) AcceptLoginToken(ctx context.Context, token tg.AuthLoginToken) error {
	log.Printf("AcceptLoginToken вызван (не используется)")
	return fmt.Errorf("логин по токену не поддерживается")
}

// Authenticate выполняет полный процесс аутентификации
func Authenticate(ctx context.Context, client *telegram.Client) error {
	// Проверяем текущий статус аутентификации
	status, err := client.Auth().Status(ctx)
	if err != nil {
		return fmt.Errorf("не удалось проверить статус аутентификации: %w", err)
	}

	if !status.Authorized {
		log.Printf("Начинаем процесс аутентификации...")
		fmt.Println("Начинаем аутентификацию в Telegram...")

		// Запускаем процесс аутентификации
		if err := client.Auth().IfNecessary(ctx, auth.NewFlow(
			AuthFlow{},
			auth.SendCodeOptions{},
		)); err != nil {
			return fmt.Errorf("ошибка аутентификации: %w", err)
		}

		log.Printf("Аутентификация успешно завершена")
		fmt.Println("Аутентификация успешно завершена!")
	} else {
		log.Printf("Уже аутентифицирован")
		fmt.Println("Уже аутентифицирован!")
	}

	// Получаем информацию о текущем пользователе
	me, err := client.Self(ctx)
	if err != nil {
		return fmt.Errorf("не удалось получить информацию о пользователе: %w", err)
	}

	// Форматируем информацию о пользователе
	userInfo := fmt.Sprintf("%s", me.FirstName)
	if me.LastName != "" {
		userInfo += " " + me.LastName
	}
	if me.Username != "" {
		userInfo += " (@" + me.Username + ")"
	}

	log.Printf("✅ Успешный вход как: %s", userInfo)
	fmt.Printf("👤 Вы вошли как: %s\n", userInfo)

	return nil
}
