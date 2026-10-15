package catalogtest

import "time"

const (
	contentSource   = "catalogtest"
	contentChecksum = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
)

var contentImportedAt = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

// Data returns a fresh copy of the fixture content, without ids. Each call
// builds new maps, so callers may mutate the result.
func Data() Fixture {
	return Fixture{
		Regions: []Region{galicia(), asturias()},
		Dishes:  []Dish{tortilla(), cocido()},
		ContentVersion: ContentVersion{
			ImportedAt: contentImportedAt,
			Source:     contentSource,
			Checksum:   contentChecksum,
		},
	}
}

func unit(u string) *string { return &u }

func photo(key string) *string { return &key }

func galicia() Region {
	return Region{
		Key:       "gal",
		SortOrder: 1,
		TileColor: "#9A9A5E",
		Name:      Text{"en": "Galicia", "es": "Galicia", "ru": "Галисия"},
		MapLabel:  Text{"en": "Galicia", "es": "Galicia", "ru": "Галисия"},
		Description: Text{
			"en": "Atlantic seafood, pilgrim fare and Spain’s best-loved octopus.",
			"es": "Marisco atlántico, cocina de peregrinos y el pulpo más querido de España.",
			"ru": "Атлантические морепродукты, еда паломников и самый любимый осьминог Испании.",
		},
		RegionalDishes: []string{"Pulpo á feira", "Empanada gallega", "Lacón con grelos"},
	}
}

func asturias() Region {
	return Region{
		Key:       "ast",
		SortOrder: 2,
		TileColor: "#C97B4A",
		Name:      Text{"en": "Asturias", "es": "Asturias", "ru": "Астурия"},
		MapLabel:  Text{"en": "Asturias", "es": "Asturias", "ru": "Астурия"},
		Description: Text{
			"en": "Bean stews, cider houses and mountain cheeses from the green north.",
			"es": "Potes de fabes, sidrerías y quesos de montaña del norte verde.",
			"ru": "Фасолевые рагу, сидрерии и горные сыры зелёного севера.",
		},
		RegionalDishes: []string{"Fabada asturiana", "Cachopo", "Arroz con leche"},
	}
}

func tortilla() Dish {
	return Dish{
		Key:        "tortilla",
		SortOrder:  1,
		Course:     "tapas",
		Season:     "all",
		Vegetarian: true,
		Quick:      true,
		Name:       Text{"en": "Tortilla de patatas", "es": "Tortilla de patatas", "ru": "Тортилья"},
		Description: Text{
			"en": "One dish, two centuries of argument. Onion or none is only where it starts.",
			"es": "Un plato, dos siglos de discusión. Con o sin cebolla es solo el principio.",
			"ru": "Одно блюдо — два века споров. С луком или без — это только начало.",
		},
		Photos: Photos{
			Thumb: photo("dishes/tortilla/thumb-1a2b3c4d"),
			Card:  photo("dishes/tortilla/card-5e6f7a8b"),
			Hero:  photo("dishes/tortilla/hero-9c0d1e2f"),
		},
		Variations: []Variation{tortillaBetanzos(), tortillaChorizo()},
	}
}

func cocido() Dish {
	return Dish{
		Key:        "cocido",
		SortOrder:  3,
		Course:     "mains",
		Season:     "winter",
		Pork:       true,
		SlowCooked: true,
		Name:       Text{"en": "Cocido & stews", "es": "Cocidos y guisos", "ru": "Косидо и рагу"},
		Description: Text{
			"en": "The Sunday pot. Chickpeas, beans or cabbage depending on the latitude.",
			"es": "La olla del domingo. Garbanzos, alubias o berza según la latitud.",
			"ru": "Воскресный котёл. Нут, фасоль или капуста — в зависимости от широты.",
		},
		Variations: []Variation{fabadaAsturiana()},
	}
}

func tortillaBetanzos() Variation {
	return Variation{
		RegionKey: "gal",
		RecipeKey: "tortilla-betanzos",
		Name:      Text{"en": "Betanzos-style runny", "es": "Estilo Betanzos, jugosa", "ru": "По-бетансосски, жидкая внутри"},
		Description: Text{
			"en": "Barely-set centre, no onion, eaten with bread",
			"es": "Centro apenas cuajado, sin cebolla, con pan",
			"ru": "Едва схваченная серединка, без лука, едят с хлебом",
		},
		Photos: Photos{
			Thumb: photo("dishes/tortilla/gal/thumb-3a4b5c6d"),
			Card:  photo("dishes/tortilla/gal/card-7e8f9a0b"),
			Hero:  photo("dishes/tortilla/gal/hero-1c2d3e4f"),
		},
		Recipe: betanzosRecipe(),
	}
}

func betanzosRecipe() Recipe {
	return Recipe{
		TimeMinutes:  25,
		BaseServings: 2,
		Ingredients: []Ingredient{
			{Quantity: 4, Name: Text{"en": "Eggs", "es": "Huevos", "ru": "Яйца"}},
			{Quantity: 400, Unit: unit("g"), Name: Text{"en": "Waxy potatoes", "es": "Patatas de carne firme", "ru": "Плотный картофель"}},
			{Quantity: 250, Unit: unit("ml"), Name: Text{"en": "Olive oil, for confit", "es": "Aceite de oliva, para confitar", "ru": "Оливковое масло для конфи"}},
			{Quantity: 1, Unit: unit("tsp"), Name: Text{"en": "Salt", "es": "Sal", "ru": "Соль"}},
		},
		Steps: []Text{
			{"en": "Peel the potatoes and slice them very thin.", "es": "Pela las patatas y córtalas muy finas.", "ru": "Очистите картофель и нарежьте очень тонко."},
			{"en": "Confit in the oil over low heat for 15 minutes, until soft but not coloured. Drain and keep the oil.", "es": "Confítalas en el aceite a fuego bajo 15 minutos, tiernas pero sin dorar. Escúrrelas y guarda el aceite.", "ru": "Томите в масле на слабом огне 15 минут, до мягкости, не обжаривая. Откиньте картофель, масло сохраните."},
			{"en": "Beat the eggs with the salt, fold in the hot potatoes and rest for 5 minutes.", "es": "Bate los huevos con la sal, añade las patatas calientes y deja reposar 5 minutos.", "ru": "Взбейте яйца с солью, вмешайте горячий картофель и оставьте на 5 минут."},
			{"en": "Heat a spoon of the oil in a small pan until very hot. Pour in and cook for 40 seconds.", "es": "Calienta una cucharada del aceite en una sartén pequeña hasta que esté muy caliente. Vierte y cuaja 40 segundos.", "ru": "Сильно разогрейте ложку масла в маленькой сковороде. Вылейте смесь и жарьте 40 секунд."},
			{"en": "Flip with a plate, cook 20 seconds more and serve at once — the centre should run.", "es": "Dale la vuelta con un plato, 20 segundos más y sirve enseguida: el centro debe quedar jugoso.", "ru": "Переверните с помощью тарелки, жарьте ещё 20 секунд и сразу подавайте — серединка должна остаться жидкой."},
		},
	}
}

// tortillaChorizo has no recipe in content.json; the recipe is written for
// the fixture.
func tortillaChorizo() Variation {
	return Variation{
		RegionKey: "ast",
		RecipeKey: "tortilla-chorizo",
		Name:      Text{"en": "With chorizo", "es": "Con chorizo", "ru": "С чорисо"},
		Description: Text{
			"en": "Chorizo fat colours the potato",
			"es": "La grasa del chorizo tiñe la patata",
			"ru": "Жир чорисо окрашивает картофель",
		},
		Recipe: chorizoRecipe(),
	}
}

func chorizoRecipe() Recipe {
	return Recipe{
		TimeMinutes:  35,
		BaseServings: 4,
		Ingredients: []Ingredient{
			{Quantity: 6, Name: Text{"en": "Eggs", "es": "Huevos", "ru": "Яйца"}},
			{Quantity: 600, Unit: unit("g"), Name: Text{"en": "Potatoes", "es": "Patatas", "ru": "Картофель"}},
			{Quantity: 150, Unit: unit("g"), Name: Text{"en": "Asturian chorizo", "es": "Chorizo asturiano", "ru": "Астурийский чорисо"}},
			{Quantity: 200, Unit: unit("ml"), Name: Text{"en": "Olive oil", "es": "Aceite de oliva", "ru": "Оливковое масло"}},
			{Quantity: 0.5, Unit: unit("tsp"), Name: Text{"en": "Salt", "es": "Sal", "ru": "Соль"}},
		},
		Steps: []Text{
			{"en": "Dice the chorizo and fry it gently until it gives up its fat.", "es": "Corta el chorizo en dados y fríelo suave hasta que suelte la grasa.", "ru": "Нарежьте чорисо кубиками и обжаривайте, пока не вытопится жир."},
			{"en": "Fry the sliced potatoes in the oil and chorizo fat until tender.", "es": "Fríe las patatas en láminas en el aceite y la grasa del chorizo hasta que estén tiernas.", "ru": "Обжарьте нарезанный картофель в масле с жиром чорисо до мягкости."},
			{"en": "Beat the eggs with the salt and fold in the potatoes and chorizo.", "es": "Bate los huevos con la sal y mezcla con las patatas y el chorizo.", "ru": "Взбейте яйца с солью и смешайте с картофелем и чорисо."},
			{"en": "Set in a hot pan, flip once and serve warm.", "es": "Cuaja en una sartén caliente, dale la vuelta una vez y sirve templada.", "ru": "Жарьте на горячей сковороде, переверните один раз и подавайте тёплой."},
		},
	}
}

// fabadaAsturiana: content.json gives "3 h + soaking", stored as 180 minutes
// plus a per-locale time note.
func fabadaAsturiana() Variation {
	return Variation{
		RegionKey: "ast",
		RecipeKey: "fabada-asturiana",
		Name:      Text{"en": "Fabada asturiana", "es": "Fabada asturiana", "ru": "Fabada asturiana"},
		Description: Text{
			"en": "Fabes de la Granja, no vegetables",
			"es": "Fabes de la Granja, sin verduras",
			"ru": "Фасоль фабес-де-ла-гранха, без овощей",
		},
		Recipe: fabadaRecipe(),
	}
}

func fabadaRecipe() Recipe {
	return Recipe{
		TimeMinutes:  180,
		TimeNote:     &Text{"en": "+ soaking", "es": "+ remojo", "ru": "+ замачивание"},
		BaseServings: 4,
		Ingredients: []Ingredient{
			{Quantity: 500, Unit: unit("g"), Name: Text{"en": "Fabes de la Granja", "es": "Fabes de la Granja", "ru": "Фасоль фабес-де-ла-гранха"}},
			{Quantity: 2, Name: Text{"en": "Asturian chorizo", "es": "Chorizo asturiano", "ru": "Астурийский чорисо"}},
			{Quantity: 2, Name: Text{"en": "Asturian morcilla", "es": "Morcilla asturiana", "ru": "Астурийская морсилья"}},
			{Quantity: 150, Unit: unit("g"), Name: Text{"en": "Panceta", "es": "Panceta", "ru": "Панчетта"}},
			{Quantity: 1, Name: Text{"en": "Pinch of saffron", "es": "Una pizca de azafrán", "ru": "Щепотка шафрана"}},
		},
		Steps: []Text{
			{"en": "Soak the beans overnight in cold water.", "es": "Pon las fabes en remojo en agua fría la noche anterior.", "ru": "Замочите фасоль на ночь в холодной воде."},
			{"en": "Cover beans and meats with fresh water and bring slowly to a boil. Skim.", "es": "Cubre las fabes y el compango con agua fresca y lleva a ebullición despacio. Espuma.", "ru": "Залейте фасоль и мясо свежей водой и медленно доведите до кипения. Снимите пену."},
			{"en": "Simmer very gently for 2½–3 hours, adding cold water to “scare” the beans when it boils.", "es": "Cuece muy suave 2½–3 horas, «asustando» las fabes con agua fría cuando hierva.", "ru": "Томите на очень слабом огне 2½–3 часа, «пугая» фасоль холодной водой, когда закипит."},
			{"en": "Add saffron halfway. Never stir — shake the pot instead.", "es": "Añade el azafrán a mitad de cocción. Nunca remuevas: mueve la olla.", "ru": "На полпути добавьте шафран. Никогда не мешайте — встряхивайте кастрюлю."},
			{"en": "Rest an hour before serving. Better the next day.", "es": "Reposa una hora antes de servir. Mejor al día siguiente.", "ru": "Дайте постоять час перед подачей. На следующий день ещё вкуснее."},
		},
	}
}
