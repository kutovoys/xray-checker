# Xray Checker (личный форк со Speedtest)

Это форк [kutovoys/xray-checker](https://github.com/kutovoys/xray-checker) — инструмента для мониторинга доступности прокси-серверов (VLESS, VMess, Trojan, Shadowsocks) с метриками для Prometheus и веб-дашбордом.

**Чем этот форк отличается от оригинала:** добавлен периодический замер скорости (download/upload) через каждый прокси, результаты видны в дашборде и в метриках. Подробности — в разделе [Speedtest](#-speedtest) ниже. Всё остальное работает так же, как в оригинале — полную документацию по фичам оригинального проекта смотри на [xray-checker.kutovoy.dev](https://xray-checker.kutovoy.dev/).

## 🚀 Быстрый старт (Docker Compose)

Понадобится сервер с установленным Docker и Docker Compose. Если Docker не установлен:

```bash
curl -fsSL https://get.docker.com | sh
```

Дальше:

```bash
# 1. Клонируем форк
git clone -b claude/vigilant-faraday-iov93w https://github.com/PepHap/xray-checker.git
cd xray-checker

# 2. Готовим конфиг
cp .env.example .env
cp docker-compose.example.yml docker-compose.yml
nano .env   # вставь свой SUBSCRIPTION_URL, при желании поменяй остальное

# 3. Собираем образ и запускаем
docker compose up -d --build
```

Дашборд откроется на `http://<ip-сервера>:2112`. Метрики Prometheus — на `http://<ip-сервера>:2112/metrics`.

> .env и docker-compose.yml в .gitignore — твои настройки и ссылка на подписку не попадут в git по ошибке.

## ⚙️ Основные переменные окружения

Полный список — в `.env.example` и в [документации оригинала](https://xray-checker.kutovoy.dev/configuration/variables). Самые важные:

| Переменная | По умолчанию | Что делает |
| --- | --- | --- |
| `SUBSCRIPTION_URL` | — | URL подписки (или share-ссылка). Обязательная. |
| `METRICS_PROTECTED` | `false` | Закрыть дашборд и `/metrics` Basic Auth |
| `METRICS_USERNAME` / `METRICS_PASSWORD` | `metricsUser` / `MetricsVeryHardPassword` | Логин/пароль для Basic Auth |
| `WEB_PUBLIC` | `false` | Публичная страница статуса без авторизации (требует `METRICS_PROTECTED=true`) |
| `WEB_SHOW_DETAILS` | `false` | Показывать IP:порт серверов в дашборде |
| `PROXY_CHECK_INTERVAL` | `300` | Как часто проверять прокси, сек |
| `SPEEDTEST_ENABLED` | `false` | Включить периодический speedtest |
| `SPEEDTEST_INTERVAL` | `240` | Интервал speedtest, мин |

## 📶 Speedtest

Фича этого форка. Раз в `SPEEDTEST_INTERVAL` минут по очереди (не параллельно, чтобы не давать ложно высокую скорость от перегрузки сети) через SOCKS5-порт каждого прокси прогоняется тест скорости. Результат виден:

- в дашборде — рядом с задержкой (latency) каждого прокси;
- в метриках — `xray_proxy_speedtest_download_bps` и `xray_proxy_speedtest_upload_bps`;
- в REST API — поля `downloadMbps` / `uploadMbps` / `speedtestTested`.

Включить:

```env
SPEEDTEST_ENABLED=true
SPEEDTEST_INTERVAL=240
```

## 🔒 Защита дашборда

Если сервер смотрит в интернет без nginx/firewall перед ним — включи `METRICS_PROTECTED=true` и задай свои `METRICS_USERNAME`/`METRICS_PASSWORD`. Без этого дашборд и метрики доступны всем, кто знает адрес.

Для публичной страницы статуса (без IP/портов, без авторизации, можно делиться с пользователями VPN) — `WEB_PUBLIC=true` плюс `METRICS_PROTECTED=true` (защищает только админский `/metrics` и `/api`).

## 🔁 Обновление

```bash
cd xray-checker
git pull
docker compose build --no-cache
docker compose up -d --force-recreate
```

После обновления сделай в браузере жёсткий рефреш страницы (`Ctrl+Shift+R`) или открой её в режиме инкогнито — дашборд рендерится на сервере, и браузер может закэшировать старую версию.

## 🌐 Доступ через свой nginx (опционально)

Если уже есть nginx и хочется отдать дашборд на отдельном пути существующего домена (например, `https://example.com/xray-checker/`), а не на отдельном порту:

```nginx
location /xray-checker/ {
    proxy_pass http://127.0.0.1:2112/xray-checker/;
    proxy_set_header Host $host;
}
```

И в `.env`:

```env
METRICS_BASE_PATH=/xray-checker
METRICS_HOST=0.0.0.0
```

`METRICS_BASE_PATH` обязателен — приложение само ожидает префикс в пути и само его убирает внутри себя, поэтому в nginx путь оставляем как есть, не делаем rewrite/strip префикса.

## 🛟 Типичные проблемы

**`Bind for 0.0.0.0:2112 failed: port is already allocated`**
Порт уже занят старым контейнером или другим процессом:
```bash
docker ps                 # найти старый контейнер
docker stop <id> && docker rm <id>
# либо для процесса не из Docker:
ss -tulnp | grep 2112
kill <pid>
```

**Дашборд открывается, метрики приходят, но в консоли браузера ошибки Alpine.js / карточки прокси не отображаются**
Обычно — закэшированная браузером старая версия страницы. Сделай жёсткий рефреш (`Ctrl+Shift+R`) или открой в инкогнито.

**После `docker compose up -d --build` код не обновился**
Слои Docker могли закэшироваться. Пересобери без кэша: `docker compose build --no-cache && docker compose up -d --force-recreate`.

## 📚 Документация оригинала

Все остальные возможности (несколько подписок, кастомизация веб-интерфейса, Pushgateway, интеграция с Uptime Kuma и т.д.) описаны в документации оригинального проекта: [xray-checker.kutovoy.dev](https://xray-checker.kutovoy.dev/).
