---
title: Формат подписки
description: Варианты и примеры формата подписки
---

Xray Checker поддерживает перечисленные ниже источники конфигурации. Для настройки используйте [переменную окружения](/ru/configuration/envs#subscription_url) `SUBSCRIPTION_URL`. По умолчанию используется встроенный Xray; для [подписок Mihomo](#10-подписки-mihomo-и-h2mux) нужны `PROXY_ENGINE=mihomo` и отдельно установленное ядро Mihomo.

Подробнее о методах проверки прокси читайте в разделе [методы проверки](/ru/configuration/check-methods).

### 1. URL подписки (По умолчанию)

Стандартный URL подписки, возвращающий Base64-кодированный список прокси-ссылок.

Пример:

```bash
SUBSCRIPTION_URL=https://example.com/subscription
```

Требования:

- HTTPS URL
- Возвращает Base64-кодированное содержимое
- Содержимое - это прокси-URL, разделенные переносом строки
- Поддерживает стандартные заголовки User-Agent

Отправляемые заголовки:

```
Accept: */*
User-Agent: Xray-Checker
```

### 2. Строка Base64

Прямая Base64-кодированная строка, содержащая ссылки конфигурации прокси.

Пример:

```bash
SUBSCRIPTION_URL=dmxlc3M6Ly91dWlkQGV4YW1wbGUuY29tOjQ0MyVlbmNyeXB0aW9uPW5vbmUmc2VjdXJpdHk9dGxzI3Byb3h5MQ==
```

Формат содержимого (до кодирования):

```
vless://uuid@example.com:443?encryption=none&security=tls#proxy1
trojan://password@example.com:443?security=tls#proxy2
vmess://base64encodedconfig
ss://base64encodedconfig
hysteria2://password@example.com:443?sni=example.com#proxy5
```

:::note[Поддерживаемые протоколы]
Поддерживаются VLESS, VMess, Trojan, Shadowsocks и **Hysteria2**, включая такие транспорты, как Reality, xhttp, gRPC, WebSocket и mKCP. Также можно проверять обычные **форвард-прокси SOCKS / HTTP / HTTPS** — см. [SOCKS, HTTP и HTTPS прокси](#7-socks-http-и-https-прокси) ниже.
:::

### 3. JSON-файл V2Ray

Один JSON-файл конфигурации в формате V2Ray/Xray.

Пример:

```bash
SUBSCRIPTION_URL=file:///path/to/config.json
```

Формат файла:

```json
{
  "outbounds": [
    {
      "protocol": "vless",
      "settings": {
        "vnext": [
          {
            "address": "example.com",
            "port": 443,
            "users": [
              {
                "id": "uuid",
                "encryption": "none"
              }
            ]
          }
        ]
      },
      "streamSettings": {
        "network": "tcp",
        "security": "tls"
      }
    }
  ]
}
```

### 4. Xray JSON-массив (Мульти-конфиг)

JSON-массив, содержащий несколько конфигураций Xray с именами. Этот формат удобен при экспорте конфигураций из GUI-клиентов или управлении несколькими именованными конфигурациями в одном файле.

Пример:

```bash
SUBSCRIPTION_URL=file:///path/to/configs.json
```

Формат файла:

```json
[
  {
    "remarks": "US Server 1",
    "outbounds": [
      {
        "protocol": "vless",
        "settings": {
          "vnext": [
            {
              "address": "us1.example.com",
              "port": 443,
              "users": [{ "id": "uuid-1", "encryption": "none" }]
            }
          ]
        },
        "streamSettings": { "network": "tcp", "security": "tls" }
      }
    ]
  },
  {
    "remarks": "EU Server 1",
    "outbounds": [
      {
        "protocol": "trojan",
        "settings": {
          "servers": [
            {
              "address": "eu1.example.com",
              "port": 443,
              "password": "password123"
            }
          ]
        },
        "streamSettings": { "network": "tcp", "security": "tls" }
      }
    ]
  }
]
```

Поле `remarks` из каждой конфигурации будет использоваться как имя прокси в панели управления.

### 5. Папка с конфигурациями

Директория, содержащая несколько JSON-файлов конфигурации V2Ray/Xray.

Пример:

```bash
SUBSCRIPTION_URL=folder:///path/to/configs
```

Требования:

- Директория должна содержать .json файлы
- Каждый файл следует формату JSON V2Ray
- Файлы обрабатываются в алфавитном порядке
- Некорректные файлы пропускаются с предупреждением

### 6. JSON-подписка (балансировщики)

Некоторые панели (например, Remnawave) отдают полный JSON-конфиг Xray вместо Base64-списка ссылок и объединяют несколько серверов в один **балансировщик** (`balancer` / `leastPing`). При стандартном формате Base64 такая группа сворачивается в одну запись, поэтому невозможно понять, какой именно узел недоступен.

Включите [`SUBSCRIPTION_JSON_FORMAT`](/ru/configuration/envs#subscription_json_format), чтобы запросить JSON-форму и развернуть каждый outbound внутри балансировщика в **отдельно проверяемый прокси**:

```bash
SUBSCRIPTION_URL=https://panel.example.com/sub
SUBSCRIPTION_JSON_FORMAT=true
```

Узлы внутри группы именуются как `<group> | <node>` и имеют общий `group_name` (используется [сгруппированным дашбордом](/ru/configuration/status-page) и [меткой метрики `group_name`](/ru/integrations/metrics)). При включённом формате JSON запрос отправляется с похожим на приложение `User-Agent`; переопределите его через `SUBSCRIPTION_USER_AGENT`, если ваша панель ожидает определённый клиент.

### 7. SOCKS, HTTP и HTTPS прокси

Помимо протоколов Xray, можно проверять работоспособность обычных форвард-прокси. Добавляйте их как строки подписки (из любого источника — URL, `base64://`, `file://` или inline):

```
socks://base64(user:pass)@host:port#name
socks5://user:pass@host:port#name
http://user:pass@host:port#name
https://user:pass@host:port#name
```

- `socks://`, `socks5://` и `socks5h://` соответствуют SOCKS outbound. Учётные данные могут быть как обычными `user:pass`, так и (для `socks://`) стандартным Base64-кодированным токеном `user:pass`.
- `http://` — обычный HTTP CONNECT прокси.
- `https://` — HTTP-прокси, доступный через TLS.
- Фрагмент `#name` задаёт отображаемое имя (по умолчанию `host:port`).

Для `https://` прокси с самоподписанным или приватным сертификатом закрепите его вместо отключения проверки (xray-core больше не поддерживает `allowInsecure`):

```
https://user:pass@host:port?pinnedPeerCertSha256=<sha256-hex>#name
https://user:pass@host:port?sni=real.example.com&verifyPeerCertByName=real.example.com#name
```

| Параметр запроса | Псевдоним | Описание |
|-------------|-------|-------------|
| `pinnedPeerCertSha256` | `pcs` | Принять сертификат пира, чей SHA-256 (hex, двоеточия допускаются) совпадает — для самоподписанных/внутренних сертификатов |
| `verifyPeerCertByName` | `vcn` | Проверять сертификат по этому имени вместо хоста |
| `sni` | — | TLS Server Name (по умолчанию совпадает с хостом) |

### 8. WireGuard

Серверы WireGuard также можно проверять на работоспособность. Добавляйте их как строки подписки (из любого источника — URL, `base64://`, `file://` или inline) по схеме `wg://`, где полезная нагрузка — это **Base64 стандартного `.conf`-файла WireGuard**:

```
wg://<base64 of the .conf>#name
```

Декодированный `.conf` — это обычный конфиг WireGuard, который выдаёт ваш провайдер:

```ini
[Interface]
PrivateKey = <client private key>
Address = 10.9.0.2/32
DNS = 1.1.1.1            # optional, ignored by the checker
MTU = 1420               # optional (default 1420)

[Peer]
PublicKey = <server public key>
PresharedKey = <psk>     # optional
Endpoint = wg.example.com:51820
AllowedIPs = 0.0.0.0/0, ::/0
PersistentKeepalive = 25 # optional
```

- Проверяется `Endpoint` пира (`host:port`). Используется первый `[Peer]`.
- Фрагмент `#name` задаёт отображаемое имя (по умолчанию `wireguard-<host>`).
- Только стандартный, необфусцированный WireGuard. (AmneziaWG / `awg://` не поддерживается.)

WireGuard также работает внутри [JSON-подписки](#6-json-подписка-балансировщики) — outbound `wireguard` в JSON-конфиге Xray разбирается автоматически:

```json
{
  "protocol": "wireguard",
  "settings": {
    "secretKey": "<client private key>",
    "address": ["10.9.0.2/32"],
    "mtu": 1420,
    "peers": [
      {
        "publicKey": "<server public key>",
        "endpoint": "wg.example.com:51820",
        "allowedIPs": ["0.0.0.0/0", "::/0"],
        "keepAlive": 25
      }
    ]
  }
}
```

:::note[Режим TUN и производительность]
WireGuard работает в пространстве пользователя (модуль ядра не требуется). Сетевой уровень — это либо настоящий интерфейс **kernel TUN** (быстрый, масштабируется на множество туннелей), либо userspace-netstack **gVisor** (работает без привилегий, например на macOS, но тяжелее). Для kernel TUN нужны `/dev/net/tun` и `CAP_NET_ADMIN`; в Docker передайте `--cap-add NET_ADMIN --device /dev/net/tun`. Для подписок с большим количеством конфигов WireGuard рекомендуется kernel TUN (и задайте `PROXY_TIMEOUT` с запасом).
:::

### 9. Кастомные лейблы метрик

Любой outbound в JSON-подписке (разделы [3](#3-json-файл-v2ray), [4](#4-xray-json-массив-мульти-конфиг) и [6](#6-json-подписка-балансировщики)) может содержать объект `metricsLabels` со статическими лейблами, заданными оператором. Каждая запись становится дополнительным лейблом на метриках `xray_proxy_status` и `xray_proxy_latency_ms` этого прокси и возвращается в API в поле `metricsLabels`. Это позволяет фильтровать и агрегировать по таким атрибутам, как локация или хостер, прямо в PromQL и Grafana.

```json
{
  "protocol": "trojan",
  "tag": "proxy",
  "settings": { "servers": [{ "address": "1.1.1.1", "port": 443, "password": "..." }] },
  "metricsLabels": {
    "location": "Netherlands, Amsterdam",
    "hoster": "FreeVDS"
  }
}
```

Лейблы добавляются к метрике:

```text
xray_proxy_status{protocol="trojan",address="1.1.1.1:443",name="proxy",...,location="Netherlands, Amsterdam",hoster="FreeVDS"} 1
```

Примечания:

- Ключи приводятся к корректным именам лейблов Prometheus (например, `data center` → `data_center`); ключи, совпадающие со встроенными лейблами (`protocol`, `address`, `name`, `sub_name`, `stable_id`, `group_name`, `instance`), игнорируются.
- Лейблы доступны только в JSON-подписках — в share-ссылках (`vless://`, …) их негде передать.
- Изменение лейбла и обновление подписки применяются при следующем обновлении **без сброса серий других прокси в 0**. См. [`metricsLabels` на метриках](/ru/integrations/metrics#кастомные-лейблы).

### 10. Подписки Mihomo и h2mux

Установите `PROXY_ENGINE=mihomo`, чтобы проверять прокси с нативными параметрами
Mihomo, которые используют совместимые клиенты. В частности, включённый
`smux.protocol: h2mux` передаётся в Mihomo и используется при проверке, а не
теряется при преобразовании в Xray JSON.

#### Ядро и формат входных данных

Установите актуальный [релиз Mihomo](https://github.com/MetaCubeX/mihomo/releases)
для ОС и архитектуры машины, проверьте контрольную сумму релиза и зафиксируйте
версию в конфигурации развёртывания. Ядро должно поддерживать именованные SOCKS-
слушатели с параметром `proxy` и настройки из вашей подписки. Xray Checker не
включает, не скачивает и не обновляет исполняемый файл Mihomo.

При разработке используется тестовое ядро Mihomo **v1.19.33** для Linux.

| Переменная окружения | Параметр CLI | По умолчанию |
| --- | --- | --- |
| `PROXY_ENGINE` | `--proxy-engine` | `xray`; для этого режима выберите `mihomo` |
| `MIHOMO_BINARY` | `--mihomo-binary` | `mihomo`, поиск в `PATH` |
| `XRAY_START_PORT` | `--xray-start-port` | `10000`, используется обоими движками |

`SUBSCRIPTION_URL` может указывать на полную YAML-подписку Mihomo или документ
провайдера с непустым встроенным списком `proxies`. Для локального файла
используйте `file:///absolute/path/subscription.yaml`. JSON с той же структурой
`proxies` тоже поддерживается; `folder:///absolute/path/configs` в этом режиме
читает файлы `.yaml`, `.yml` и `.json`. Все источники одного чекера в режиме Mihomo
должны иметь этот формат; Xray JSON и списки share-ссылок предназначены для
движка Xray.

Каждая запись должна содержать непустые строки `name`, `type` и `server`, а также
скалярный `port` от 1 до 65535. Записи с перебором портов, содержащие только
`ports` без `port`, не поддерживаются.

По умолчанию URL-подписка запрашивается с `User-Agent: mihomo`.
`SUBSCRIPTION_USER_AGENT` и `SUBSCRIPTION_HEADERS` могут переопределить его;
обновите прежние настройки, специфичные для Xray или Happ. В панелях вроде
Remnawave можно использовать явный URL формата Mihomo — обычно это текущий URL
подписки с добавленным `/mihomo`, если панель разрешает такой путь. У аккаунта
мониторинга должен быть доступ к нужным inbound и хостам в формате Mihomo.
`SUBSCRIPTION_JSON_FORMAT=true` запрашивает Xray JSON и отклоняется в этом режиме.

Пример прокси VLESS/REALITY:

```yaml
proxies:
  - name: H2MUX example
    type: vless
    server: proxy.example.com
    port: 443
    uuid: YOUR_USER_UUID
    network: tcp
    tls: true
    servername: cover.example.com
    client-fingerprint: chrome
    reality-opts:
      public-key: YOUR_REALITY_PUBLIC_KEY
      short-id: YOUR_SHORT_ID
      support-x25519mlkem768: true
    smux:
      enabled: true
      protocol: h2mux
      padding: true
      min-streams: 8
      max-connections: 2
```

Используйте значения из своей панели. Параметры транспорта, `smux` и
`reality-opts` сохраняются. Чекер создаёт собственные уникальные внутренние имена
прокси и сохраняет исходное имя сервера; разрешением DNS занимается Mihomo.
`PROXY_RESOLVE_DOMAINS=true` в этом режиме отклоняется: замена имени на IP может
изменить неявный TLS SNI. Для выбора семейства адресов используйте нативный
параметр `ip-version` в определении прокси.

Проверяются только записи встроенного списка. Удалённые `proxy-providers` не
загружаются; документ, содержащий только провайдеры, отклоняется. Правила
маршрутизации, группы, слушатели, DNS, TUN, контроллер и скрипты верхнего уровня
из подписки не импортируются. Каждый локальный SOCKS-слушатель чекера направляет
запросы прямо в один прокси. Цепочки `dialer-proxy` отклоняются, чтобы проверка
не уходила через группу или другой узел. Итоговый конфиг проверяется выбранным
исполняемым файлом Mihomo.

Ядро работает с правами ОС аккаунта чекера. Нативные параметры прокси могут
ссылаться на локальные файлы, поэтому используйте подписки из доверенных источников.

Методы `ip`, `status` и `download` используют HTTP-запросы через TCP-путь каждого
прокси. При включённом h2mux запросы проходят через него. Отдельные проверки UDP,
ёмкости мультиплексирования и выбранной TLS-группы обмена ключами не выполняются.

#### Нативный запуск на OpenWrt ARM64 или Linux

Для OpenWrt, где `uname -m` возвращает `aarch64`, нужны совместимые исполняемые
файлы чекера и Mihomo для Linux ARM64. Поместите выбранные версии в отдельную
тестовую папку: примеры не заменяют существующий прокси-сервис роутера. API-порт
`2212` и диапазон SOCKS-портов от `12000` позволяют провести проверку рядом с
чекером на стандартных портах, если выбранные порты свободны.

```sh
cd /opt/xray-checker-test
export PROXY_ENGINE=mihomo
export MIHOMO_BINARY=/opt/xray-checker-test/mihomo
export SUBSCRIPTION_URL='https://subscription.example/PRIVATE_TOKEN/mihomo'
export SUBSCRIPTION_JSON_FORMAT=false
export XRAY_START_PORT=12000
export METRICS_HOST=127.0.0.1
export METRICS_PORT=2212
./xray-checker
```

Чекеру нужны право запуска ядра и доступная для записи рабочая папка для
сгенерированного конфига. При запуске как сервиса храните приватный URL подписки
в защищённой конфигурации окружения этого сервиса. Менеджер сервиса должен
штатно завершать чекер, чтобы тот мог остановить дочерний процесс Mihomo.

#### Нативный запуск на Windows

Установите оба подходящих Windows-исполняемых файла в тестовую папку и создайте
в ней подпапку `tmp`. До запуска ограничьте права NTFS на обе папки и их
содержимое аккаунтом чекера, SYSTEM и группой администраторов. Файл
`mihomo_config.yaml` в рабочей папке, а также конфиги проверки и запуска ядра
в `TEMP` содержат учётные данные прокси. Unix-режим файла `0600` в Go не задаёт
права доступа Windows ACL.

В PowerShell укажите эту защищённую временную папку в `TEMP` и `TMP`:

```powershell
Set-Location 'C:\Tools\xray-checker-test'
$env:TEMP = 'C:\Tools\xray-checker-test\tmp'
$env:TMP = $env:TEMP
$env:PROXY_ENGINE = 'mihomo'
$env:MIHOMO_BINARY = 'C:\Tools\xray-checker-test\mihomo.exe'
$env:SUBSCRIPTION_URL = 'https://subscription.example/PRIVATE_TOKEN/mihomo'
$env:SUBSCRIPTION_JSON_FORMAT = 'false'
$env:XRAY_START_PORT = '12000'
$env:METRICS_HOST = '127.0.0.1'
$env:METRICS_PORT = '2212'
.\xray-checker.exe
```

После проверки примените те же переменные в существующем менеджере сервиса
чекера с защищённой рабочей папкой. Направьте stdout и stderr сервиса в папку
с такими же правами: ошибки запуска и получения подписки могут содержать её
приватный URL. Другой системный сервис Xray или Mihomo заменять не требуется.

#### Docker и страницы статуса

Стандартный образ чекера содержит встроенный Xray, но **не содержит Mihomo**.
Для этого режима предоставьте совместимый с архитектурой и средой контейнера
Linux-исполняемый файл, например через bind mount только для чтения:

```yaml
services:
  xray-checker:
    image: kutovoys/xray-checker
    environment:
      PROXY_ENGINE: mihomo
      MIHOMO_BINARY: /opt/mihomo/mihomo
      SUBSCRIPTION_URL: ${SUBSCRIPTION_URL}
      SUBSCRIPTION_JSON_FORMAT: "false"
    volumes:
      - ./mihomo:/opt/mihomo/mihomo:ro
    ports:
      - "127.0.0.1:2212:2112"
```

У пользователя контейнера должно быть право запуска смонтированного файла.
Передавайте `SUBSCRIPTION_URL` через окружение развёртывания.

Структура `/api/v1/public/proxies`, эндпоинтов статуса прокси и метрик Prometheus
сохраняется, поэтому страницы статуса могут получать результаты через прежний
API. Опциональное поле `generatedConfig` для Mihomo-прокси не возвращается:
нативные определения могут содержать учётные данные в произвольных вложенных
параметрах.

При переключении прокси с Xray на Mihomo создаётся другой stable ID и новая
серия мониторинга, даже если адрес не изменился. История страницы статуса
автоматически между этими идентификаторами не переносится.

## Кастомные заголовки запросов

Панели, закрывающие подписку токеном или ожидающие определённого клиента, можно удовлетворить с помощью кастомного `User-Agent` и произвольных заголовков:

```bash
SUBSCRIPTION_USER_AGENT="Happ/1.0"
SUBSCRIPTION_HEADERS="X-Token: abc, X-Region: eu"
```

См. [`SUBSCRIPTION_USER_AGENT`](/ru/configuration/envs#subscription_user_agent) и [`SUBSCRIPTION_HEADERS`](/ru/configuration/envs#subscription_headers).
