---
title: Руководство по разработке
description: Руководство по разработке
---

### Настройка среды разработки

1. Требования:

   - Go 1.27.3 или новее
   - Git
   - Make (опционально, для использования Makefile)

2. Клонирование репозитория:

```bash
git clone https://github.com/kutovoys/xray-checker.git
cd xray-checker
```

3. Установка зависимостей:

```bash
go mod download
```

4. Сборка проекта:

```bash
make build
# или
go build -o xray-checker
```

### Структура проекта

```
.
├── checker/       # Логика проверки прокси
├── config/        # Обработка конфигурации
├── geo/           # Geo-файлы (geoip.dat, geosite.dat)
├── logger/        # Структурированное логирование
├── metrics/       # Метрики Prometheus
├── models/        # Модели данных
├── subscription/  # Парсинг и управление подписками
├── web/           # Веб-интерфейс, API и ассеты
├── xray/          # Интеграция с Xray и раннер
├── go.mod         # Файл модулей Go
└── main.go        # Точка входа в приложение
```

### Внесение изменений

1. Создайте новую ветку:

```bash
git checkout -b feature/ваше-название-функции
```

2. Внесите изменения
3. Запустите тесты
4. Обновите документацию при необходимости
5. Отправьте pull request

### Локальное тестирование

1. Настройте тестовую конфигурацию:

```bash
export SUBSCRIPTION_URL="ваша_тестовая_подписка"
```

2. Запустите в режиме разработки:

```bash
go run main.go
```

3. Запустите с определенными функциями:

```bash
go run main.go --proxy-check-method=status --metrics-protected=true
```
