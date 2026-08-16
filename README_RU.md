# Xray Checker

[![GitHub Release](https://img.shields.io/github/v/release/kutovoys/xray-checker?style=flat&color=blue)](https://github.com/kutovoys/xray-checker/releases/latest)
[![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/kutovoys/xray-checker/build-publish.yml)](https://github.com/kutovoys/xray-checker/actions/workflows/build-publish.yml)
[![DockerHub](https://img.shields.io/badge/DockerHub-kutovoys%2Fxray--checker-blue)](https://hub.docker.com/r/kutovoys/xray-checker/)
[![Documentation](https://img.shields.io/badge/docs-xray--checker.kutovoy.dev-blue)](https://xray-checker.kutovoy.dev/)
[![Live Demo](https://img.shields.io/badge/demo-live-brightgreen)](https://demo-xray-checker.kutovoy.dev/)
[![Telegram Chat](https://img.shields.io/badge/Telegram-Chat-blue?logo=telegram)](https://t.me/+uZCGx_FRY0tiOGIy)
[![GitHub License](https://img.shields.io/github/license/kutovoys/xray-checker?color=greeen)](https://github.com/kutovoys/xray-checker/blob/main/LICENSE)
[![ru](https://img.shields.io/badge/lang-ru-blue)](https://github.com/kutovoys/xray-checker/blob/main/README_RU.md)
[![en](https://img.shields.io/badge/lang-en-red)](https://github.com/kutovoys/xray-checker/blob/main/README.md)

Xray Checker - это инструмент для мониторинга доступности прокси-серверов с поддержкой протоколов VLESS, VMess, Trojan и Shadowsocks. Он автоматически тестирует соединения через Xray Core и предоставляет метрики для Prometheus, а также API-эндпоинты для интеграции с системами мониторинга.

<div align="center">
  <img src=".github/screen/xray-checker.webp" alt="Dashboard Screenshot">
</div>

> [!TIP]
> **Попробуйте демо:** Посмотрите Xray Checker в действии на [demo-xray-checker.kutovoy.dev](https://demo-xray-checker.kutovoy.dev/)

## 🚀 Основные возможности

- 🔍 Мониторинг работоспособности Xray-прокси серверов (VLESS, VMess, Trojan, Shadowsocks)
- 🔄 Автоматическое обновление конфигурации из подписки (поддержка нескольких подписок)
- 📊 Экспорт метрик в формате Prometheus с поддержкой Pushgateway
- 🌐 REST API с документацией OpenAPI/Swagger
- 🌓 Веб-интерфейс с темной/светлой темой
- 🎨 Полная кастомизация веб-интерфейса (свой логотип, стили или весь шаблон)
- 📄 Публичная страница статуса для VPN-сервисов (без аутентификации)
- 📥 Эндпоинты для интеграции с системами мониторинга (Uptime Kuma и др.)
- 🔒 Защита метрик и веб-интерфейса с помощью Basic Auth
- 🐳 Поддержка Docker и Docker Compose
- 🌍 Автоматическое управление geo-файлами (geoip.dat, geosite.dat)
- 📝 Гибкая загрузка конфигурации:
  - URL-подписки (base64, JSON)
  - Share-ссылки (vless://, vmess://, trojan://, ss://)
  - JSON-файлы конфигурации
  - Папки с конфигурациями

Полный список возможностей доступен в [документации](https://xray-checker.kutovoy.dev/ru/intro/features).

## 🚀 Быстрый старт

### Docker

```bash
docker run -d \
  -e SUBSCRIPTION_URL=https://your-subscription-url/sub \
  -p 2112:2112 \
  kutovoys/xray-checker
```

### Docker Compose

```yaml
services:
  xray-checker:
    image: kutovoys/xray-checker
    environment:
      - SUBSCRIPTION_URL=https://your-subscription-url/sub
      - TELEGRAM_BOT_TOKEN=123456:your-bot-token
      - TELEGRAM_CHAT_IDS=123456789,-1001234567890_10
    ports:
      - "2112:2112"
```

Подробная документация по установке и настройке доступна на [xray-checker.kutovoy.dev](https://xray-checker.kutovoy.dev/ru/intro/quick-start)

## 📈 Статистика проекта

<a href="https://star-history.com/#kutovoys/xray-checker&Date">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=kutovoys/xray-checker&type=Date&theme=dark" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=kutovoys/xray-checker&type=Date" />
   <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=kutovoys/xray-checker&type=Date" />
 </picture>
</a>

## 🤝 Участие в разработке

Мы рады любому вкладу в развитие Xray Checker! Если вы хотите помочь:

1. Сделайте форк репозитория
2. Создайте ветку для ваших изменений
3. Внесите изменения и протестируйте их
4. Создайте Pull Request

Подробнее о том, как внести свой вклад, читайте в [руководстве для контрибьюторов](https://xray-checker.kutovoy.dev/ru/contributing/development-guide).

<p align="center">
Спасибо всем контрибьюторам, которые помогли улучшить Xray Checker:
</p>
<p align="center">
<a href="https://github.com/kutovoys/xray-checker/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=kutovoys/xray-checker" />
</a>
</p>
<p align="center">
  Сделано с помощью <a rel="noopener noreferrer" target="_blank" href="https://contrib.rocks">contrib.rocks</a>
</p>

---

## Рекомендация VPN

Для безопасного и надежного доступа в интернет мы рекомендуем [BlancVPN](https://getblancvpn.com/pricing?promo=klugscl&ref=xc-readme). Используйте промокод `KLUGSCL` для получения скидки 15% на вашу подписку.
