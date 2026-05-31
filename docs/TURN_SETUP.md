# Настройка TURN серверов для TorrSyncPlayer

## Содержание

- [Что такое TURN серверы](#что-такое-turn-серверы)
- [Зачем нужны TURN серверы](#зачем-нужны-turn-серверы)
- [Как работает NAT traversal в WebRTC](#как-работает-nat-traversal-в-webrtc)
- [Варианты TURN серверов](#варианты-turn-серверов)
- [Пошаговая настройка coturn](#пошаговая-настройка-coturn)
- [Интеграция с TorrSyncPlayer](#интеграция-с-torrsyncplayer)
- [Мониторинг и отладка](#мониторинг-и-отладка)
- [Рекомендации по безопасности](#рекомендации-по-безопасности)

---

## Что такое TURN серверы

**TURN** (Traversal Using Relays around NAT) — это протокол, позволяющий устанавливать соединения между пирами через промежуточный сервер (ретранслятор) в случаях, когда прямое P2P соединение невозможно из-за ограничений NAT или файрволов.

В отличие от STUN серверов, которые только помогают определить внешний IP-адрес и порт, TURN серверы непосредственно ретранслируют трафик между пирами.

---

## Зачем нужны TURN серверы

В проекте TorrSyncPlayer TURN серверы необходимы для:

1. **Надежного P2P соединения** — около 10-20% пользователей находятся за симметричными NAT, которые не позволяют установить прямое соединение
2. **Работы через корпоративные файрволы** — многие корпоративные сети блокируют P2P соединения
3. **Гарантированной работы приложения** — без TURN часть пользователей не сможет подключиться к комнатам

### Когда TURN обязателен

- Симметричный NAT у обоих пиров
- Корпоративные сети с ограничениями
- Сети с двойным NAT
- Мобильные сети с ограничениями

---

## Как работает NAT traversal в WebRTC

WebRTC использует следующий алгоритм для установления соединения:

```
┌─────────┐     ┌─────────┐     ┌─────────┐
│  Peer A  │     │  STUN   │     │  Peer B  │
└────┬─────┘     └────┬─────┘     └────┬─────┘
     │                │                │
     │ 1. Запрос внешнего адреса       │
     │───────────────>│                │
     │                │                │
     │ 2. Ответ с внешним адресом      │
     │<───────────────│                │
     │                │                │
     │ 3. Обмен ICE candidates через сигнальный сервер
     │<───────────────────────────────>│
     │                │                │
     │ 4. Попытка прямого соединения   │
     │<───────────────────────────────>│
     │                │                │
     │ 5. Если прямое соединение не удалось — используем TURN
     │<───────────────>│<───────────────>│
```

### Этапы соединения

1. **STUN** — определение внешнего IP и порта
2. **ICE** — сбор всех возможных кандидатов (host, srflx, relay)
3. **Проверка соединения** — попытка соединения по каждому кандидату
4. **TURN** — если все проверки провалились, используется ретранслятор

---

## Варианты TURN серверов

### 1. Собственный сервер (coturn)

**Преимущества:**
- Полный контроль над данными
- Нет ограничений на трафик
- Низкая стоимость при масштабировании

**Недостатки:**
- Требует администрирования
- Нужно следить за доступностью
- Начальные затраты на настройку

**Рекомендуется для:** production-развёртываний с большим количеством пользователей

### 2. Облачные сервисы

#### Twilio Network Traversal Service

```yaml
# Пример конфигурации
urls: "turn:global.turn.twilio.com:3478?transport=udp"
username: "your_twilio_username"
credential: "your_twilio_credential"
```

**Преимущества:**
- Быстрый старт
- Высокая доступность
- Глобальная инфраструктура

**Стоимость:** ~$0.40 за ГБ трафика

#### Xirsys

```yaml
urls: "turn:your-zone.xirsys.com:80?transport=udp"
username: "your_username"
credential: "your_credential"
```

**Преимущества:**
- Простая интеграция
- Гибкие тарифы
- API для управления серверами

#### Metered.ca

```yaml
urls: "turn:your-domain.metered.ca:80"
username: "your_username"
credential: "your_credential"
```

**Преимущества:**
- Бесплатный тариф для разработки
- Простая настройка
- Хорошая документация

---

## Пошаговая настройка coturn

### Установка

#### Ubuntu/Debian

```bash
sudo apt update
sudo apt install coturn
```

#### CentOS/RHEL

```bash
sudo yum install epel-release
sudo yum install coturn
```

#### Docker

```bash
docker run -d \
  --name coturn \
  --network host \
  -v /etc/coturn:/etc/coturn \
  coturn/coturn
```

### Конфигурация

Создайте файл `/etc/turnserver.conf`:

```conf
# Основные настройки
listening-port=3478
tls-listening-port=5349

# Внешний IP сервера (замените на ваш)
external-ip=YOUR_SERVER_IP

# Диапазон портов для relay
min-port=49152
max-port=65535

# Домен и сертификат для TLS
cert=/etc/letsencrypt/live/your-domain.com/fullchain.pem
pkey=/etc/letsencrypt/live/your-domain.com/privkey.pem

# Аутентификация (долгосрочные credentials)
lt-cred-mech
realm=your-domain.com

# Пользователи (логин:пароль)
user=torrsync:your_secure_password

# Логирование
log-file=/var/log/turnserver.log
simple-log

# Ограничения
max-bps=1000000
total-quota=100
user-quota=10

# Безопасность
no-multicast-peers
denied-peer-ip=0.0.0.0-0.255.255.255
denied-peer-ip=10.0.0.0-10.255.255.255
denied-peer-ip=127.0.0.0-127.255.255.255
denied-peer-ip=169.254.0.0-169.254.255.255
denied-peer-ip=172.16.0.0-172.31.255.255
denied-peer-ip=192.168.0.0-192.168.255.255

# Протоколы
no-tcp-relay
```

### Запуск

#### Включение автозапуска

```bash
sudo systemctl enable coturn
sudo systemctl start coturn
```

#### Проверка статуса

```bash
sudo systemctl status coturn
```

#### Проверка работы

```bash
# Проверка порта
sudo netstat -tulpn | grep turnserver

# Тест с помощью turnutils_uclient
turnutils_uclient -u torrsync -w your_secure_password YOUR_SERVER_IP
```

### Настройка файрвола

```bash
# UFW (Ubuntu)
sudo ufw allow 3478/tcp
sudo ufw allow 3478/udp
sudo ufw allow 5349/tcp
sudo ufw allow 5349/udp
sudo ufw allow 49152:65535/udp

# firewalld (CentOS)
sudo firewall-cmd --permanent --add-port=3478/tcp
sudo firewall-cmd --permanent --add-port=3478/udp
sudo firewall-cmd --permanent --add-port=5349/tcp
sudo firewall-cmd --permanent --add-port=5349/udp
sudo firewall-cmd --permanent --add-port=49152-65535/udp
sudo firewall-cmd --reload
```

---

## Интеграция с TorrSyncPlayer

### Конфигурация ICE серверов

Для интеграции TURN серверов необходимо обновить конфигурацию WebRTC в [`p2p_service.go`](../p2p_service.go).

#### Текущая конфигурация (только STUN)

```go
config := webrtc.Configuration{
    ICEServers: []webrtc.ICEServer{
        {
            URLs: []string{"stun:stun.l.google.com:19302"},
        },
        {
            URLs: []string{"stun:stun1.l.google.com:19302"},
        },
    },
}
```

#### Обновлённая конфигурация (с TURN)

```go
config := webrtc.Configuration{
    ICEServers: []webrtc.ICEServer{
        // STUN серверы
        {
            URLs: []string{"stun:stun.l.google.com:19302"},
        },
        {
            URLs: []string{"stun:stun1.l.google.com:19302"},
        },
        // TURN серверы
        {
            URLs:       []string{"turn:your-turn-server.com:3478"},
            Username:   "torrsync",
            Credential: "your_secure_password",
        },
        {
            URLs:       []string{"turns:your-turn-server.com:5349"},
            Username:   "torrsync",
            Credential: "your_secure_password",
        },
    },
}
```

### Конфигурация через переменные окружения

Рекомендуется использовать переменные окружения для настройки TURN серверов:

```bash
# .env файл
TURN_SERVER_URL=turn:your-turn-server.com:3478
TURN_SERVER_URLS=turn:your-turn-server.com:3478,turns:your-turn-server.com:5349
TURN_USERNAME=torrsync
TURN_CREDENTIAL=your_secure_password
```

### Пример кода для загрузки конфигурации

```go
package main

import (
    "os"
    "strings"
    
    "github.com/pion/webrtc/v4"
)

// getICEServers возвращает список ICE серверов из переменных окружения
func getICEServers() []webrtc.ICEServer {
    servers := []webrtc.ICEServer{
        // Всегда добавляем STUN
        {
            URLs: []string{"stun:stun.l.google.com:19302"},
        },
    }
    
    // Добавляем TURN если настроен
    turnURL := os.Getenv("TURN_SERVER_URL")
    turnUsername := os.Getenv("TURN_USERNAME")
    turnCredential := os.Getenv("TURN_CREDENTIAL")
    
    if turnURL != "" && turnUsername != "" && turnCredential != "" {
        servers = append(servers, webrtc.ICEServer{
            URLs:       strings.Split(turnURL, ","),
            Username:   turnUsername,
            Credential: turnCredential,
        })
    }
    
    return servers
}
```

---

## Мониторинг и отладка

### Логирование

Включите подробное логирование в coturn:

```conf
# /etc/turnserver.log
verbose
log-file=/var/log/turnserver.log
```

### Мониторинг метрик

#### Проверка активных сессий

```bash
# Через turnadmin
turnadmin -l -u torrsync -p your_secure_password

# Через лог
tail -f /var/log/turnserver.log | grep "session"
```

#### Мониторинг трафика

```bash
# Использование сети
iftop -i eth0 -f "port 3478 or portrange 49152-65535"

# Статистика по портам
ss -tulpn | grep turnserver
```

### Отладка соединений

#### Включение отладки в WebRTC

```javascript
// В браузере
const pc = new RTCPeerConnection({
    iceServers: [...],
    iceTransportPolicy: 'relay' // Только для отладки TURN
});

// Логирование ICE событий
pc.onicecandidate = (event) => {
    if (event.candidate) {
        console.log('ICE candidate:', event.candidate.candidate);
    }
};

pc.oniceconnectionstatechange = () => {
    console.log('ICE state:', pc.iceConnectionState);
};
```

#### Проверка типа соединения

```javascript
pc.getStats().then(stats => {
    stats.forEach(report => {
        if (report.type === 'candidate-pair' && report.state === 'succeeded') {
            const local = stats.get(report.localCandidateId);
            const remote = stats.get(report.remoteCandidateId);
            console.log('Local type:', local.candidateType);
            console.log('Remote type:', remote.candidateType);
            // relay = используется TURN
            // srflx = используется STUN
            // host = прямое соединение
        }
    });
});
```

### Типичные проблемы и решения

| Проблема | Возможная причина | Решение |
|----------|-------------------|---------|
| `ICE failed` | TURN сервер недоступен | Проверьте доступность сервера и файрвол |
| `Allocation timeout` | Неправильные credentials | Проверьте логин/пароль |
| `Permission denied` | Истекла сессия | Используйте long-term credentials |
| Высокая задержка | Далёкий сервер | Разместите сервер ближе к пользователям |
| `401 Unauthorized` | Неправильный realm | Проверьте настройки realm в coturn |

---

## Рекомендации по безопасности

### 1. Используйте TLS

Всегда используйте `turns://` (TURN over TLS) для шифрования трафика:

```go
{
    URLs:       []string{"turns:your-turn-server.com:5349"},
    Username:   "torrsync",
    Credential: "your_secure_password",
}
```

### 2. Ограничьте доступ

```conf
# Разрешите только определённые IP
allowed-peer-ip=203.0.113.0-203.0.113.255

# Запретите приватные диапазоны
denied-peer-ip=0.0.0.0-0.255.255.255
denied-peer-ip=10.0.0.0-10.255.255.255
denied-peer-ip=127.0.0.0-127.255.255.255
denied-peer-ip=169.254.0.0-169.254.255.255
denied-peer-ip=172.16.0.0-172.31.255.255
denied-peer-ip=192.168.0.0-192.168.255.255
```

### 3. Ограничьте квоты

```conf
# Максимальный трафик на пользователя (в байтах/сек)
max-bps=1000000

# Общая квота на всех пользователей (в мегабайтах)
total-quota=100

# Квота на пользователя (в мегабайтах)
user-quota=10
```

### 4. Используйте временные credentials

Для production рекомендуется использовать временные credentials с ограниченным сроком действия:

```go
// Пример генерации временных credentials
import (
    "crypto/hmac"
    "crypto/sha1"
    "encoding/base64"
    "fmt"
    "time"
)

func generateTURNCredentials(username, secret string) (string, string) {
    // TTL: 24 часа
    ttl := 24 * 60 * 60
    timestamp := time.Now().Add(time.Duration(ttl) * time.Second).Unix()
    
    // Формат: timestamp:username
    fullUsername := fmt.Sprintf("%d:%s", timestamp, username)
    
    // HMAC-SHA1
    mac := hmac.New(sha1.New, []byte(secret))
    mac.Write([]byte(fullUsername))
    credential := base64.StdEncoding.EncodeToString(mac.Sum(nil))
    
    return fullUsername, credential
}
```

### 5. Регулярно обновляйте coturn

```bash
# Ubuntu/Debian
sudo apt update && sudo apt upgrade coturn

# CentOS/RHEL
sudo yum update coturn
```

### 6. Мониторинг аномалий

Настройте алерты на:
- Необычно высокое использование трафика
- Многочисленные неудачные попытки аутентификации
- Аномальное количество сессий с одного IP

---

## Дополнительные ресурсы

- [Документация coturn](https://github.com/coturn/coturn/wiki)
- [WebRTC ICE](https://webrtc.org/getting-started/ice)
- [RFC 8656 - TURN](https://tools.ietf.org/html/rfc8656)
- [Pion WebRTC](https://github.com/pion/webrtc)
