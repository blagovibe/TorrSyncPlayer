/**
 * @file networkmanager.cpp
 * @brief Реализация HTTP клиента для связи с Go backend
 *
 * Включает retry logic с экспоненциальным backoff:
 * - Максимум 3 попытки (настраивается)
 * - Базовая задержка 1000мс (настраивается)
 * - Экспоненциальный backoff: delay = baseDelay * 2^attempt
 * - Автоматическое переподключение SSE с exponential backoff
 */

#include "networkmanager.h"
#include "utils.h"

#include <QDebug>
#include <QJsonDocument>
#include <QJsonObject>
#include <QJsonArray>
#include <QRegularExpression>
#include <QMutex>
#include <QMutexLocker>

// ── Константы ──────────────────────────────────────────────────────────

namespace {
    // Максимальное количество попыток переподключения SSE
    const int MaxSSEReconnectAttempts = 5;
    // Базовая задержка для SSE переподключения (мс)
    const int SSEReconnectBaseDelay = 1000;
    // Максимальная задержка для SSE переподключения (мс)
    const int SSEReconnectMaxDelay = 30000;
    // Таймаут передачи данных (мс)
    const int TransferTimeout = 60000;

    QString buildApiPath(const QString &basePath, const QString &id)
    {
        QString sanitized = id;
        sanitized.remove(QRegularExpression("[^a-fA-F0-9]"));
        if (sanitized.isEmpty()) {
            return basePath + "/";
        }
        return basePath + "/" + sanitized;
    }

    QUrl safeUrl(const QUrl &base, const QString &path)
    {
        QUrl resolved(base);
        QString cleanPath = path;
        if (!cleanPath.startsWith('/')) {
            cleanPath.prepend('/');
        }
        resolved.setPath(resolved.path() + cleanPath);
        return resolved;
    }
}

NetworkManager::NetworkManager(QObject *parent)
    : INetworkManager(parent)
    , m_network(new QNetworkAccessManager(this))
    , m_serverUrl(QStringLiteral("https://localhost:8889"))
    , m_sseReply(nullptr)
    , m_retryTimer(new QTimer(this))
    , m_sseReconnectTimer(new QTimer(this))
{
    // SSL mode: default to AllowSelfSigned for development (embedded backend uses self-signed certs)
    // Set SSL_MODE=strict to enforce strict verification
    m_sslMode = SslMode::AllowSelfSigned;
    qDebug() << "NetworkManager: SSL mode set to AllowSelfSigned (development)";
    // Подключаем сигналы менеджера сети
    connect(m_network, &QNetworkAccessManager::finished,
            this, &NetworkManager::onReplyFinished);
    connect(m_network, &QNetworkAccessManager::sslErrors,
            this, &NetworkManager::onSslErrors);

    // Настраиваем таймер retry
    m_retryTimer->setSingleShot(true);
    connect(m_retryTimer, &QTimer::timeout, this, &NetworkManager::retryRequest);

    // Настраиваем таймер SSE переподключения
    m_sseReconnectTimer->setSingleShot(true);
    connect(m_sseReconnectTimer, &QTimer::timeout, this, &NetworkManager::onSSEReconnect);

    // Запрашиваем CSRF-токен при старте (для защиты мутирующих запросов без JWT)
    fetchCsrfToken();

    qDebug() << "NetworkManager: инициализирован с URL" << m_serverUrl.toString();
}

NetworkManager::~NetworkManager()
{
    disconnectSSE();

    // Collect replies under lock
    QList<QNetworkReply*> replies;
    {
        QMutexLocker locker(&m_replyMutex);
        replies = m_replyMap.keys();
        m_replyMap.clear();
    }

    // Abort outside lock
    for (QNetworkReply *reply : replies) {
        if (reply) {
            reply->abort();
            reply->deleteLater();
        }
    }

    m_network->clearAccessCache();
    m_network->clearConnectionCache();
    m_retryTimer->stop();
    m_sseReconnectTimer->stop();
}

// ── Torrent API ───────────────────────────────────────────────────────

void NetworkManager::addTorrent(const QString &magnetUri)
{
    // Валидация magnet URI
    if (magnetUri.isEmpty()) {
        emit error(tr("Magnet-ссылка не может быть пустой"));
        return;
    }

    if (magnetUri.length() > APIConstants::MaxMagnetUriLength) {
        emit error(tr("Magnet-ссылка слишком длинная (максимум %1 символов)").arg(APIConstants::MaxMagnetUriLength));
        return;
    }

    if (!magnetUri.startsWith("magnet:?")) {
        emit error(tr("Некорректная magnet-ссылка. Должна начинаться с 'magnet:?'"));
        return;
    }

    QJsonObject body;
    body["magnetUri"] = magnetUri;
    sendWithRetry("POST", "/api/v1/torrents", RequestType::AddTorrent, body);
}

void NetworkManager::addTorrentFile(const QByteArray &torrentData)
{
    // Валидация размера файла
    if (torrentData.isEmpty()) {
        emit error(tr("Файл .torrent пуст"));
        return;
    }

    const qint64 maxTorrentSize = 1024 * 1024; // 1MB max (matches backend MaxTorrentFileSize)
    if (torrentData.size() > maxTorrentSize) {
        emit error(tr("Файл .torrent слишком большой (максимум %1 МБ)").arg(maxTorrentSize / (1024 * 1024)));
        return;
    }

    // Кодируем в base64
    QByteArray base64Data = torrentData.toBase64();

    QJsonObject body;
    body["torrentFile"] = QString::fromUtf8(base64Data);
    sendWithRetry("POST", "/api/v1/torrents", RequestType::AddTorrent, body);
}

void NetworkManager::removeTorrent(const QString &id)
{
    // Валидация ID торрента
    if (id.isEmpty()) {
        emit error(tr("ID торрента не может быть пустым"));
        return;
    }

    if (id.length() > APIConstants::MaxTorrentIdLength) {
        emit error(tr("ID торрента слишком длинный"));
        return;
    }

    sendWithRetry("DELETE", buildApiPath("/api/v1/torrents", id), RequestType::RemoveTorrent);
}

void NetworkManager::listTorrents()
{
    sendWithRetry("GET", "/api/v1/torrents", RequestType::ListTorrents);
}

void NetworkManager::getFiles(const QString &torrentId)
{
    // Валидация ID торрента
    if (torrentId.isEmpty()) {
        emit error(tr("ID торрента не может быть пустым"));
        return;
    }

    sendWithRetry("GET", buildApiPath("/api/v1/torrents", torrentId) + "/files", RequestType::GetFiles);
}

void NetworkManager::selectFile(const QString &torrentId, int fileIndex)
{
    // Валидация ID торрента
    if (torrentId.isEmpty()) {
        emit error(tr("ID торрента не может быть пустым"));
        return;
    }

    // Валидация индекса файла
    if (fileIndex < 0) {
        emit error(tr("Индекс файла не может быть отрицательным"));
        return;
    }

    QJsonObject body;
    body["fileIndex"] = fileIndex;
    sendWithRetry("POST", buildApiPath("/api/v1/torrents", torrentId) + "/select", RequestType::SelectFile, body);
}

// ── Room API ──────────────────────────────────────────────────────────

void NetworkManager::createRoom(const QString &name, const QString &password)
{
    // Валидация имени комнаты
    if (name.isEmpty()) {
        emit error(tr("Имя комнаты не может быть пустым"));
        return;
    }

    if (name.length() < APIConstants::MinRoomNameLength) {
        emit error(tr("Имя комнаты должно содержать минимум %1 символа").arg(APIConstants::MinRoomNameLength));
        return;
    }

    if (name.length() > APIConstants::MaxRoomNameLength) {
        emit error(tr("Имя комнаты слишком длинное (максимум %1 символов)").arg(APIConstants::MaxRoomNameLength));
        return;
    }

    // Безопасность: блокируем отправку пароля по незащищённому соединению (кроме localhost)
    if (!password.isEmpty() && m_serverUrl.host() != "localhost" && m_serverUrl.host() != "127.0.0.1") {
        if (m_serverUrl.scheme() != "https") {
            emit error(tr("Пароль не может быть отправлен по незащищённому соединению. Используйте HTTPS или localhost."));
            return;
        }
    }

    QJsonObject body;
    body["name"] = name;
    if (!password.isEmpty()) {
        body["password"] = password;
    }
    sendWithRetry("POST", "/api/v1/rooms", RequestType::CreateRoom, body);
}

void NetworkManager::joinRoom(const QString &roomId, const QString &password)
{
    // Валидация ID комнаты
    if (roomId.isEmpty()) {
        emit error(tr("ID комнаты не может быть пустым"));
        return;
    }

    if (roomId.length() > APIConstants::MaxRoomIdLength) {
        emit error(tr("ID комнаты слишком длинный"));
        return;
    }

    QString ssePath;
    {
        QMutexLocker locker(&m_roomIdMutex);
        m_currentRoomId = roomId;

        QString sanitizedRoomId = roomId;
        sanitizedRoomId.remove(QRegularExpression("[^a-fA-F0-9]"));
        ssePath = QString("/api/v1/rooms/%1/events").arg(sanitizedRoomId);
        m_sseReconnectPath = ssePath;
    }

    QJsonObject body;
    body["roomId"] = roomId;
    if (!password.isEmpty()) {
        body["password"] = password;
    }
    sendWithRetry("POST", "/api/v1/rooms/join", RequestType::JoinRoom, body);

    // Подключаемся к SSE потоку комнаты
    if (!ssePath.isEmpty()) {
        connectToSSE(ssePath);
    }
}

void NetworkManager::leaveRoom()
{
    {
        QMutexLocker locker(&m_roomIdMutex);
        if (m_currentRoomId.isEmpty()) {
            emit error(tr("Не в комнате"));
            return;
        }
    }

    sendPost("/api/v1/rooms/leave", QJsonObject(), RequestType::LeaveRoom);

    // Отключаемся от SSE
    disconnectSSE();

    {
        QMutexLocker locker(&m_roomIdMutex);
        m_currentRoomId.clear();
    }
    emit roomLeft();
}

void NetworkManager::sendSignal(const QJsonObject &signal)
{
    {
        QMutexLocker locker(&m_roomIdMutex);
        if (m_currentRoomId.isEmpty()) {
            emit error(tr("Не в комнате"));
            return;
        }
    }
    sendPost("/api/v1/rooms/signal", signal, RequestType::Signal);
}

// ── Sync API ──────────────────────────────────────────────────────────

void NetworkManager::syncPlay()
{
    sendPost("/api/v1/sync/play", QJsonObject(), RequestType::SyncPlay);
}

void NetworkManager::syncPause()
{
    sendPost("/api/v1/sync/pause", QJsonObject(), RequestType::SyncPause);
}

void NetworkManager::syncSeek(double position)
{
    // Валидация позиции
    if (position < 0) {
        emit error(tr("Позиция не может быть отрицательной"));
        return;
    }

    QJsonObject body;
    body["position"] = position;
    sendPost("/api/v1/sync/seek", body, RequestType::SyncSeek);
}

// ── Утилиты ───────────────────────────────────────────────────────────

QString NetworkManager::streamUrl(const QString &torrentId) const
{
    if (torrentId.isEmpty()) {
        qWarning() << "NetworkManager: streamUrl вызван с пустым torrentId";
        return QString();
    }
    QString sanitized = torrentId;
    sanitized.remove(QRegularExpression("[^a-fA-F0-9]"));
    if (sanitized.isEmpty()) {
        qWarning() << "NetworkManager: streamUrl — sanitized ID пуст";
        return QString();
    }
    return QString("%1/api/v1/torrents/%2/stream")
        .arg(m_serverUrl.toString())
        .arg(sanitized);
}

// ── Private slots ─────────────────────────────────────────────────────

void NetworkManager::onReplyFinished(QNetworkReply *reply)
{
    // Проверяем, является ли это SSE ответом (QPointer безопасен от nullptr)
    if (m_sseReply && reply == m_sseReply) {
        return;
    }

    // Получаем тип запроса для идентификации (потокобезопасно)
    RequestType type;
    {
        QMutexLocker locker(&m_replyMutex);
        type = m_replyMap.take(reply);
    }

    QString path = reply->url().path();

    // Проверяем ошибки сети
    if (reply->error() != QNetworkReply::NoError) {
        // Обработка OperationCanceledError - не считаем ошибкой
        if (reply->error() == QNetworkReply::OperationCanceledError) {
            qDebug() << "NetworkManager: запрос отменён" << path;
            return;
        }
        handleApiError(reply);
        return;
    }

    // Сервер доступен - обновляем состояние (потокобезопасно)
    if (!m_serverAvailable.loadRelaxed()) {
        m_serverAvailable.storeRelaxed(1);
        emit serverAvailable();
    }

    // Читаем ответ с ограничением размера (защита от DoS)
    const qint64 maxResponseSize = 10 * 1024 * 1024; // 10 MB
    qint64 contentLength = reply->header(QNetworkRequest::ContentLengthHeader).toLongLong();
    if (contentLength > maxResponseSize) {
        emit error(tr("Ответ сервера слишком большой (максимум %1 байт)").arg(maxResponseSize));
        return;
    }
    QByteArray data = reply->readAll();
    if (data.size() > maxResponseSize) {
        emit error(tr("Ответ сервера слишком большой"));
        return;
    }
    QJsonDocument doc = parseJson(data);

    if (doc.isNull() && !data.isEmpty()) {
        emit error(tr("Ошибка парсинга JSON от %1").arg(path));
        return;
    }

    // Обрабатываем ответ в зависимости от типа запроса
    int statusCode = 0;
    QVariant statusVariant = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute);
    if (statusVariant.isValid()) {
        statusCode = statusVariant.toInt();
    }

    switch (type) {
    case RequestType::ListTorrents:
        if (statusCode == 200 && doc.isArray()) {
            emit torrentListReceived(doc.array());
        }
        break;
    case RequestType::AddTorrent:
        if (statusCode == 201 && doc.isObject()) {
            emit torrentAdded(doc.object());
        } else if (statusCode == 200 && doc.isObject()) {
            emit torrentAdded(doc.object());
        }
        break;
    case RequestType::GetFiles: {
        QRegularExpression re("^/api/v1/torrents/([^/]+)/files$");
        QRegularExpressionMatch match = re.match(path);
        if (match.hasMatch()) {
            QString torrentId = match.captured(1);
            if (doc.isArray()) {
                emit filesReceived(torrentId, doc.array());
            }
        }
        break;
    }
    case RequestType::SelectFile:
        qDebug() << "NetworkManager: file selected successfully";
        break;
    case RequestType::CreateRoom:
        if (doc.isObject()) {
            QJsonObject obj = doc.object();
            QString roomId = obj["id"].toString();
            {
                QMutexLocker locker(&m_roomIdMutex);
                m_currentRoomId = roomId;
            }
            emit roomCreated(roomId);
        }
        break;
    case RequestType::JoinRoom:
        if (doc.isObject()) {
            QJsonObject obj = doc.object();
            QString roomId = obj["id"].toString();
            {
                QMutexLocker locker(&m_roomIdMutex);
                m_currentRoomId = roomId;
            }
            emit roomJoined(roomId);
        }
        break;
    case RequestType::SyncPlay:
    case RequestType::SyncPause:
    case RequestType::SyncSeek:
    case RequestType::SyncStatus:
        if (doc.isObject()) {
            emit syncStatusReceived(doc.object());
        }
        break;
    case RequestType::Signal:
        qDebug() << "NetworkManager: сигнал отправлен";
        break;
    case RequestType::LeaveRoom:
        break;
    default:
        break;
    }

    reply->deleteLater();

    qDebug() << "NetworkManager: ответ от" << path << "статус" << statusCode;
}

void NetworkManager::onSsEReadyRead()
{
    // QPointer автоматически становится nullptr если объект удалён
    QNetworkReply *sseReply = m_sseReply;
    if (!sseReply) return;

    // Максимальный размер буфера SSE (1 MB) и общий лимит данных за соединение (100 MB)
    const int MaxSSEBufferSize = 1024 * 1024;
    const qint64 MaxSSETotalBytes = 100 * 1024 * 1024;
    const int MaxSSEIterations = 10000; // Prevent infinite loops in event loop

    int sseBufferUsage = 0;
    int iterations = 0;

    while (sseReply->canReadLine() && ++iterations <= MaxSSEIterations) {
        QByteArray line = sseReply->readLine();

        sseBufferUsage += line.size();
        m_sseTotalBytesRead += line.size();
        if (sseBufferUsage > MaxSSEBufferSize) {
            qWarning() << "NetworkManager: превышен лимит буфера SSE, разрыв соединения";
            emit error(tr("Превышен лимит буфера SSE"));
            disconnectSSE();
            return;
        }
        if (m_sseTotalBytesRead > MaxSSETotalBytes) {
            qWarning() << "NetworkManager: превышен общий лимит данных SSE, разрыв соединения";
            emit error(tr("Превышен лимит данных SSE"));
            disconnectSSE();
            return;
        }

        if (line.isEmpty() || line.startsWith(':')) {
            continue;
        }

        if (line.startsWith("data: ")) {
            QByteArray jsonData = line.mid(6).trimmed();

            if (jsonData.isEmpty()) {
                continue;
            }

            QJsonDocument doc = QJsonDocument::fromJson(jsonData);
            if (doc.isNull() || !doc.isObject()) {
                qWarning() << "NetworkManager: невалидный JSON в SSE:" << jsonData;
                continue;
            }

            QJsonObject event = doc.object();
            
            // Проверяем, что SSE соединение всё ещё активно перед эмиссией
            if (m_sseReply != sseReply) {
                return; // SSE was disconnected
            }
            
            emit roomEvent(event);

            QString type = event["type"].toString();
            if (type == "signal") {
                emit signalReceived(event["data"].toObject());
            }
        }
    }
    
    // Log if we hit the iteration limit (potential DoS attempt)
    if (iterations >= MaxSSEIterations) {
        qWarning() << "NetworkManager: SSE iteration limit reached, deferring remaining events";
    }
}

void NetworkManager::onNetworkError(QNetworkReply::NetworkError code)
{
    Q_UNUSED(code)
    // Обработка ошибок происходит в onReplyFinished
}

void NetworkManager::onSslErrors(QNetworkReply *reply, const QList<QSslError> &errors)
{
    if (!reply) return;

    if (m_sslMode == SslMode::AllowSelfSigned) {
        // Development mode: only accept self-signed certificates for localhost.
        // HostNameMismatch is NEVER accepted — it indicates a MITM attack.
        bool onlySelfSigned = true;
        for (const QSslError &error : errors) {
            qWarning() << "SSL error:" << error.errorString();
            if (error.error() != QSslError::SelfSignedCertificate &&
                error.error() != QSslError::SelfSignedCertificateInChain) {
                onlySelfSigned = false;
            }
        }

        if (onlySelfSigned && m_serverUrl.scheme() == "https" && m_serverUrl.host() == "localhost") {
            qWarning() << "NetworkManager: accepting self-signed cert for localhost (dev mode)";
            reply->ignoreSslErrors();
            return;
        }
    }

    // Production mode (Strict): reject all SSL errors
    QStringList errorMsgs;
    for (const QSslError &error : errors) {
        errorMsgs << error.errorString();
    }
    qWarning() << "NetworkManager: SSL errors rejected:" << errorMsgs.join(", ");
}

void NetworkManager::retryRequest()
{
    QMutexLocker locker(&m_retryMutex);

    // Проверяем, не устарел ли retry (новый запрос мог прийти)
    if (m_pendingRetrySeq != m_retrySeq) {
        qDebug() << "NetworkManager: retry устарел, отменяем";
        return;
    }

    if (m_pendingRetry.attempt >= m_maxRetries) {
        emit error(tr("Сервер недоступен после %1 попыток").arg(m_maxRetries));

        if (m_serverAvailable.loadRelaxed()) {
            m_serverAvailable.storeRelaxed(0);
            emit serverUnavailable();
        }
        return;
    }

    qDebug() << "NetworkManager: повторная попытка" << (m_pendingRetry.attempt + 1)
             << "для" << m_pendingRetry.path;

    // Выполняем запрос
        if (m_pendingRetry.method == "GET") {
        sendGet(m_pendingRetry.path, m_pendingRetry.type);
    } else if (m_pendingRetry.method == "POST") {
        sendPost(m_pendingRetry.path, m_pendingRetry.body, m_pendingRetry.type);
    } else if (m_pendingRetry.method == "DELETE") {
        sendDelete(m_pendingRetry.path, m_pendingRetry.type);
    }
}

void NetworkManager::onSSEReconnect()
{
    {
        QMutexLocker locker(&m_roomIdMutex);
        if (m_currentRoomId.isEmpty()) {
            return;
        }
    }

    if (m_sseReconnectAttempts.loadRelaxed() >= MaxSSEReconnectAttempts) {
        qWarning() << "NetworkManager: исчерпаны попытки SSE переподключения";
        emit error(tr("Не удалось восстановить SSE соединение после %1 попыток").arg(MaxSSEReconnectAttempts));
        return;
    }

    qDebug() << "NetworkManager: SSE переподключение, попытка" << (m_sseReconnectAttempts.loadRelaxed() + 1);

    // Переподключаемся по сохранённому пути
    if (m_sseReconnectPath.isEmpty()) {
        qWarning() << "NetworkManager: путь SSE переподключения пуст";
        return;
    }
    connectToSSE(m_sseReconnectPath);

    // Увеличиваем счётчик попыток
    m_sseReconnectAttempts.storeRelaxed(m_sseReconnectAttempts.loadRelaxed() + 1);
}

// ── Private methods ───────────────────────────────────────────────────

void NetworkManager::fetchCsrfToken()
{
    QUrl url = safeUrl(m_serverUrl, "/api/v1/csrf-token");
    QNetworkRequest request(url);
    request.setRawHeader("Accept", "application/json");
    request.setTransferTimeout(5000); // 5 second timeout for initial CSRF fetch

    QNetworkReply *reply = m_network->get(request);
    connect(reply, &QNetworkReply::finished, this, [this, reply]() {
        reply->deleteLater();
        if (reply->error() != QNetworkReply::NoError) {
            qDebug() << "NetworkManager: CSRF token fetch failed (non-critical):" << reply->errorString();
            return;
        }
        QByteArray data = reply->readAll();
        QJsonDocument doc = parseJson(data);
        if (doc.isObject()) {
            QString token = doc.object()["csrfToken"].toString();
            if (!token.isEmpty()) {
                {
                    QMutexLocker locker(&m_csrfTokenMutex);
                    m_csrfToken = token;
                }
                m_csrfReady = true;
                flushCsrfQueue();
                qDebug() << "NetworkManager: CSRF token obtained";
            }
        }
    });
}

void NetworkManager::applyCsrfHeader(QNetworkRequest &request)
{
    QString token;
    {
        QMutexLocker locker(&m_csrfTokenMutex);
        token = m_csrfToken;
    }
    if (!token.isEmpty()) {
        request.setRawHeader("X-CSRF-Token", token.toUtf8());
    }
}

void NetworkManager::enqueueOrSend(const QString &method, const QString &path, const QJsonObject &body, RequestType type)
{
    if (!m_csrfReady) {
        m_csrfPendingQueue.append({method, path, body, type});
        return;
    }
    if (method == "GET") {
        sendGet(path, type);
    } else if (method == "POST") {
        sendPost(path, body, type);
    } else if (method == "DELETE") {
        sendDelete(path, type);
    }
}

void NetworkManager::flushCsrfQueue()
{
    QVector<PendingCsrfRequest> pending;
    {
        QMutexLocker locker(&m_csrfTokenMutex);
        pending.swap(m_csrfPendingQueue);
    }
    for (const auto &req : pending) {
        if (req.method == "GET") {
            sendGet(req.path, req.type);
        } else if (req.method == "POST") {
            sendPost(req.path, req.body, req.type);
        } else if (req.method == "DELETE") {
            sendDelete(req.path, req.type);
        }
    }
}

void NetworkManager::applyAuthHeader(QNetworkRequest &request)
{
    QMutexLocker locker(&m_authTokenMutex);
    if (!m_authToken.isEmpty()) {
        request.setRawHeader("Authorization", "Bearer " + m_authToken.toUtf8());
    }
}

void NetworkManager::sendGet(const QString &path, RequestType type)
{
    QUrl url = safeUrl(m_serverUrl, path);
    QNetworkRequest request(url);
    request.setHeader(QNetworkRequest::ContentTypeHeader, "application/json");
    request.setRawHeader("Accept", "application/json");
    applyAuthHeader(request);
    applyCsrfHeader(request);
    request.setTransferTimeout(TransferTimeout);

    QNetworkReply *reply = m_network->get(request);

    {
        QMutexLocker locker(&m_replyMutex);
        m_replyMap[reply] = type;
    }

    qDebug() << "NetworkManager: GET" << path;
}

void NetworkManager::sendPost(const QString &path, const QJsonObject &body, RequestType type)
{
    QUrl url = safeUrl(m_serverUrl, path);
    QNetworkRequest request(url);
    request.setHeader(QNetworkRequest::ContentTypeHeader, "application/json");
    request.setRawHeader("Accept", "application/json");
    applyAuthHeader(request);
    applyCsrfHeader(request);
    request.setTransferTimeout(TransferTimeout);

    QJsonDocument doc(body);
    QByteArray data = doc.toJson(QJsonDocument::Compact);

    QNetworkReply *reply = m_network->post(request, data);

    {
        QMutexLocker locker(&m_replyMutex);
        m_replyMap[reply] = type;
    }

    qDebug() << "NetworkManager: POST" << path;
}

void NetworkManager::sendDelete(const QString &path, RequestType type)
{
    QUrl url = safeUrl(m_serverUrl, path);
    QNetworkRequest request(url);
    request.setHeader(QNetworkRequest::ContentTypeHeader, "application/json");
    request.setRawHeader("Accept", "application/json");
    applyAuthHeader(request);
    applyCsrfHeader(request);
    request.setTransferTimeout(TransferTimeout);

    QNetworkReply *reply = m_network->deleteResource(request);

    {
        QMutexLocker locker(&m_replyMutex);
        m_replyMap[reply] = type;
    }

    qDebug() << "NetworkManager: DELETE" << path;
}

void NetworkManager::sendWithRetry(const QString &method, const QString &path, const QJsonObject &body)
{
    sendWithRetry(method, path, RequestType::Unknown, body);
}

void NetworkManager::sendWithRetry(const QString &method, const QString &path, RequestType type, const QJsonObject &body)
{
    {
        QMutexLocker locker(&m_retryMutex);
        m_retrySeq++;
        m_pendingRetry.method = method;
        m_pendingRetry.path = path;
        m_pendingRetry.body = body;
        m_pendingRetry.type = type;
        m_pendingRetry.attempt = 0;
    }

    enqueueOrSend(method, path, body, type);
}

void NetworkManager::connectToSSE(const QString &path)
{
    // Отключаем предыдущий SSE перед подключением нового (предотвращает утечку)
    disconnectSSE();

    QUrl url = safeUrl(m_serverUrl, path);
    QNetworkRequest request(url);
    request.setRawHeader("Accept", "text/event-stream");
    applyAuthHeader(request);
    request.setAttribute(QNetworkRequest::Http2AllowedAttribute, false);
    request.setTransferTimeout(120000); // 2 minute timeout for SSE (prevents hung connections)

    m_sseReply = m_network->get(request);

    {
        QMutexLocker locker(&m_replyMutex);
        m_replyMap[m_sseReply] = RequestType::RoomEvents;
    }

    connect(m_sseReply, &QNetworkReply::readyRead,
            this, &NetworkManager::onSsEReadyRead);
    connect(m_sseReply, &QNetworkReply::errorOccurred,
            this, [this, self = QPointer<NetworkManager>(this)](QNetworkReply::NetworkError code) {
                Q_UNUSED(code)
                if (!self) return;
                qWarning() << "NetworkManager: SSE ошибка";
                QNetworkReply *reply = self->m_sseReply;
                if (reply) {
                    emit self->error(tr("SSE ошибка: %1").arg(reply->errorString()));
                } else {
                    emit self->error(tr("SSE ошибка: соединение потеряно"));
                }

                // Запускаем переподключение с exponential backoff
                {
                    QMutexLocker locker(&self->m_roomIdMutex);
                    if (!self->m_currentRoomId.isEmpty() && self->m_sseReconnectAttempts.loadRelaxed() < MaxSSEReconnectAttempts) {
                        int delay = qMin(SSEReconnectBaseDelay * (1 << self->m_sseReconnectAttempts.loadRelaxed()), SSEReconnectMaxDelay);
                        qDebug() << "NetworkManager: SSE переподключение через" << delay << "мс";
                        self->m_sseReconnectTimer->start(delay);
                    }
                }
            });

    // Сбрасываем счётчик переподключений при успешном подключении
    m_sseReconnectAttempts.storeRelaxed(0);

    qDebug() << "NetworkManager: SSE подключение к" << path;
}

void NetworkManager::disconnectSSE()
{
    if (m_sseReply) {
        {
            QMutexLocker locker(&m_replyMutex);
            m_replyMap.remove(m_sseReply);
        }
        m_sseReply->abort();
        m_sseReply->deleteLater();
        m_sseReply = nullptr;
        qDebug() << "NetworkManager: SSE отключено";
    }

    // Останавливаем таймер переподключения
    m_sseReconnectTimer->stop();
    m_sseReconnectAttempts.storeRelaxed(0);
    m_sseReconnectPath.clear();
    m_sseTotalBytesRead = 0;
}

QJsonDocument NetworkManager::parseJson(const QByteArray &data)
{
    QJsonParseError error;
    QJsonDocument doc = QJsonDocument::fromJson(data, &error);

    if (error.error != QJsonParseError::NoError) {
        qWarning() << "NetworkManager: ошибка парсинга JSON:"
                   << error.errorString() << "at offset" << error.offset;
        return QJsonDocument();
    }

    return doc;
}

void NetworkManager::handleApiError(QNetworkReply *reply)
{
    QString errorMessage = reply->errorString();
    int statusCode = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();

    // Пытаемся прочитать тело ошибки (с лимитом для защиты от OOM)
    const qint64 maxErrorBodySize = 1024 * 64; // 64 KB
    QByteArray data = reply->read(maxErrorBodySize);
    if (!data.isEmpty()) {
        QJsonDocument doc = parseJson(data);
        if (doc.isObject()) {
            QJsonObject obj = doc.object();
            if (obj.contains("error")) {
                errorMessage = obj["error"].toString();
            }
        }
    }

    if (statusCode == 401) {
        emit error(tr("Authentication required — please log in"));
        return;
    }

    bool shouldRetry = false;
    int attempt = 0;
    {
        QMutexLocker locker(&m_retryMutex);
        shouldRetry = (statusCode >= 500 || statusCode == 0) && m_pendingRetry.attempt < m_maxRetries;
        if (shouldRetry) {
            m_pendingRetry.attempt++;
            m_pendingRetrySeq = m_retrySeq;
        }
        attempt = m_pendingRetry.attempt;
    }

    if (shouldRetry) {
        int delay = calculateRetryDelay(attempt - 1);

        qWarning() << "NetworkManager: ошибка" << statusCode << errorMessage
                   << "- повтор через" << delay << "мс";

        m_retryTimer->start(delay);
    } else {
        emit error(tr("Ошибка %1: %2").arg(statusCode).arg(errorMessage));
        qWarning() << "NetworkManager: ошибка API" << statusCode << errorMessage;
    }
}

int NetworkManager::calculateRetryDelay(int attempt) const
{
    // Экспоненциальный backoff: baseDelay * 2^attempt
    int cappedAttempt = qMin(attempt, 30);
    qint64 delay = static_cast<qint64>(m_retryBaseDelay) * (Q_INT64_C(1) << cappedAttempt);
    return static_cast<int>(qMin(delay, static_cast<qint64>(SSEReconnectMaxDelay)));
}
