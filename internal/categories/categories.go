package categories

import "strings"

// Category представляет категорию контента
type Category struct {
	Name      string
	Keywords  []string
	Subtopics []string
	Sources   []string // Предпочтительные источники
}

// GetCategories возвращает все категории с их ключевыми словами
func GetCategories() map[string]Category {
	return map[string]Category{
		"IT и технологии": {
			Name: "IT и технологии",
			Keywords: []string{
				"программирование", "разработка", "software", "код", "алгоритм",
				"искусственный интеллект", "AI", "машинное обучение", "нейросеть",
				"кибербезопасность", "хакер", "вирус", "защита",
				"базы данных", "SQL", "NoSQL", "облако", "cloud",
				"мобильная разработка", "iOS", "Android", "React Native", "Flutter",
				"веб-разработка", "frontend", "backend", "JavaScript", "TypeScript",
				"DevOps", "контейнеризация", "Docker", "Kubernetes",
				"блокчейн", "криптовалюта", "NFT", "Web3",
				"игры", "геймдев", "Unity", "Unreal Engine",
			},
			Subtopics: []string{
				"Веб-разработка",
				"Мобильная разработка",
				"Искусственный интеллект",
				"Кибербезопасность",
				"Облачные технологии",
				"DevOps",
				"Блокчейн и крипто",
				"Геймдев",
				"Data Science",
				"Интернет вещей",
			},
			Sources: []string{"Хабрахабр", "VC.ru", "CNews", "3DNews", "IXBT"},
		},
		"Бизнес и стартапы": {
			Name: "Бизнес и стартапы",
			Keywords: []string{
				"стартап", "бизнес", "предпринимательство", "инвестиции", "венчур",
				"финансы", "экономика", "рынок", "акции", "трейдинг",
				"маркетинг", "реклама", "SEO", "SMM", "контент-маркетинг",
				"управление", "менеджмент", "лидерство", "команда", "HR",
				"продажи", "клиенты", "CRM", "бизнес-модель", "монетизация",
				"фриланс", "удаленная работа", "карьера", "трудоустройство",
			},
			Subtopics: []string{
				"Стартапы и инвестиции",
				"Финансы и трейдинг",
				"Маркетинг и реклама",
				"Управление и менеджмент",
				"Продажи и клиенты",
				"Карьера и работа",
			},
			Sources: []string{"РБК", "Коммерсант", "Forbes", "Ведомости", "VC.ru"},
		},
		"Наука и образование": {
			Name: "Наука и образование",
			Keywords: []string{
				"наука", "исследование", "открытие", "ученый", "лаборатория",
				"образование", "обучение", "курсы", "университет", "студент",
				"математика", "физика", "химия", "биология", "медицина",
				"технологии", "инновации", "изобретение", "патент",
				"космос", "астрономия", "NASA", "космонавтика",
				"психология", "социология", "исследование",
			},
			Subtopics: []string{
				"Научные открытия",
				"Образовательные технологии",
				"Медицина и здоровье",
				"Космос и астрономия",
				"Психология и социология",
			},
			Sources: []string{"N+1", "Индикатор", "Элементы", "Биомолекула"},
		},
		"Гаджеты и техника": {
			Name: "Гаджеты и техника",
			Keywords: []string{
				"смартфон", "телефон", "iPhone", "Samsung", "Xiaomi",
				"ноутбук", "компьютер", "процессор", "видеокарта", "память",
				"планшет", "умные часы", "фитнес-браслет", "гаджет", "девайс",
				"игры", "консоль", "PlayStation", "Xbox", "Nintendo",
				"автомобили", "тесла", "электромобиль", "беспилотник",
				"умный дом", "IoT", "робот", "дрон",
			},
			Subtopics: []string{
				"Смартфоны и планшеты",
				"Ноутбуки и компьютеры",
				"Игровые консоли",
				"Автомобили и транспорт",
				"Умный дом и гаджеты",
			},
			Sources: []string{"IXBT", "3DNews", "Ferra", "CNews"},
		},
	}
}

// DetectCategory определяет категорию по тексту
func DetectCategory(text string) string {
	categories := GetCategories()
	textLower := strings.ToLower(text)

	bestCategory := "Общее"
	maxMatches := 0

	for _, category := range categories {
		matches := 0
		for _, keyword := range category.Keywords {
			if strings.Contains(textLower, strings.ToLower(keyword)) {
				matches++
			}
		}
		if matches > maxMatches {
			maxMatches = matches
			bestCategory = category.Name
		}
	}

	return bestCategory
}

// GetCategory возвращает категорию по имени
func GetCategory(name string) (Category, bool) {
	categories := GetCategories()
	category, exists := categories[name]
	return category, exists
}
