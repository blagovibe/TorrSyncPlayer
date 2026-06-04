/**
 * @file networkmanager.cpp
 * @brief Реализация HTTP клиента для связи с Go backend
 * 
 * Включает retry logic с экспоненциальным backoff:
 * - Максимум 3 попытки (настраивается)
 * - Базовая задержка 1000мс (настраивается)
 * - Экспоненциальный backoff: delay = baseDelay * 2^attempt
 */

#include "networkmanager.h"

#include <QDebug>
#include <QJsonDocument>
#include <QJsonObject>
#include <QJsonArray>
#include <QEventLoop>

NetworkManager::NetworkManager(QObject *parent)
    : INetworkManager(parent)
    , m_network(new QNetworkAccessManager(this))
    , m_serverUrl("http://localhost:8889")
    , m_retryTimer(new QTimer(this))
{
    // Подключаем сигналы менеджера сети
    connect(m_network, &QNetworkAccessManager::finished,
            this, &NetworkManager::onReplyFinished);
    connect(m_network, &QNetworkAccessManager::sslErrors,
            this, &NetworkManager::onSslErrors);
    
    // Настраиваем таймер retry
    m_retryTimer->setSingleShot(true);
    connect(m_retryTimer, &QTimer::timeout, this, &NetworkManager::retryRequest);
    
    qDebug() << "NetworkManager: инициализирован с URL" << m_serverUrl.toString();
}

NetworkManager::~NetworkManager()
{
    // Отключаем SSE при уничтожении
    disconnectSSE();
    
    // Отменяем все активные запросы и очищаем карту
    for (auto it = m_replyMap.begin(); it != m_replyMap.end(); ++it) {
        QNetworkReply *reply = it.key();
        if (reply) {
            reply->abort();
            reply->deleteLater();
        }
    }
    m_replyMap.clear();
    
    // Очищаем кэш сети
    m_network->clearAccessCache();
    m_network->clearConnectionCache();
    
    // Останавливаем таймер retry
    m_retryTimer->stop();
}

// ── Torrent API ───────────────────────────────────────────────────────

void NetworkManager::addTorrent(const QString &magnetUri)
{
    QJsonObject body;
    body["magnetUri"] = magnetUri;
    sendWithRetry("POST", "/api/v1/torrents", body);
}

void NetworkManager::removeTorrent(const QString &id)
{
    sendWithRetry("DELETE", QString("/api/v1/torrents/%1").arg(id));
}

void NetworkManager::listTorrents()
{
    sendWithRetry("GET", "/api/v1/torrents");
}

void NetworkManager::getFiles(const QString &torrentId)
{
    sendWithRetry("GET", QString("/api/v1/torrents/%1/files").arg(torrentId));
}

void NetworkManager::selectFile(const QString &torrentId, int fileIndex)
{
    QJsonObject body;
    body["fileIndex"] = fileIndex;
    sendWithRetry("POST", QString("/api/v1/torrents/%1/select").arg(torrentId), body);
}

// ── Room API ──────────────────────────────────────────────────────────

void NetworkManager::createRoom(const QString &name, const QString &password)
{
    QJsonObject body;
    body["name"] = name;
    if (!password.isEmpty()) {
        body["password"] = password;
    }
    sendWithRetry("POST", "/api/v1/rooms", body);
}

void NetworkManager::joinRoom(const QString &roomId, const QString &password)
{
    m_currentRoomId = roomId;
    
    QJsonObject body;
    body["roomId"] = roomId;
    if (!password.isEmpty()) {
        body["password"] = password;
    }
    sendWithRetry("POST", "/api/v1/rooms/join", body);
    
    // Подключаемся к SSE потоку комнаты
    connectToSSE("/api/v1/rooms/events");
}

void NetworkManager::leaveRoom()
{
    if (m_currentRoomId.isEmpty()) {
        emit error(tr("Не в комнате"));
        return;
    }
    
    sendPost("/api/v1/rooms/leave", QJsonObject());
    
    // Отключаемся от SSE
    disconnectSSE();
    
    m_currentRoomId.clear();
    emit roomLeft();
}

void NetworkManager::sendSignal(const QJsonObject &signal)
{
    if (m_currentRoomId.isEmpty()) {
        emit error(tr("Не в комнате"));
        return;
    }
    sendPost("/api/v1/rooms/signal", signal);
}

// ── Sync API ──────────────────────────────────────────────────────────

void NetworkManager::syncPlay()
{
    sendPost("/api/v1/sync/play", QJsonObject());
}

void NetworkManager::syncPause()
{
    sendPost("/api/v1/sync/pause", QJsonObject());
}

void NetworkManager::syncSeek(double position)
{
    QJsonObject body;
    body["position"] = position;
    sendPost("/api/v1/sync/seek", body);
}

// ── Утилиты ───────────────────────────────────────────────────────────

QString NetworkManager::streamUrl(const QString &torrentId) const
{
    return QString("%1/api/v1/torrents/%2/stream")
        .arg(m_serverUrl.toString())
        .arg(torrentId);
}

// ── Private slots ─────────────────────────────────────────────────────

void NetworkManager::onReplyFinished(QNetworkReply *reply)
{
    // Проверяем, является ли это SSE ответом
    if (reply == m_sseReply) {
        return;
    }
    
    reply->deleteLater();
    
    // Получаем путь запроса для идентификации
    QString path = m_replyMap.take(reply);
    if (path.isEmpty()) {
        path = reply->url().path();
    }
    
    // Проверяем ошибки сети
    if (reply->error() != QNetworkReply::NoError) {
        handleApiError(reply);
        return;
    }
    
    // Сервер доступен - обновляем состояние
    if (!m_serverAvailable) {
        m_serverAvailable = true;
        emit serverAvailable();
    }
    
    // Читаем ответ
    QByteArray data = reply->readAll();
    QJsonDocument doc = parseJson(data);
    
    if (doc.isNull()) {
        emit error(tr("Ошибка парсинга JSON от %1").arg(path));
        return;
    }
    
    // Обрабатываем ответ в зависимости от пути
    int statusCode = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
    
    if (path == "/api/v1/torrents") {
        if (statusCode == 200) {
            if (doc.isArray()) {
                emit torrentListReceived(doc.array());
            }
        } else if (statusCode == 201) {
            if (doc.isObject()) {
                emit torrentAdded(doc.object());
            }
        }
    }
    else if (path.startsWith("/api/v1/torrents/") && path.endsWith("/files")) {
        QString torrentId = path.split('/')[4];
        if (doc.isArray()) {
            emit filesReceived(torrentId, doc.array());
        }
    }
    else if (path == "/api/v1/rooms") {
        if (doc.isObject()) {
            QJsonObject obj = doc.object();
            QString roomId = obj["id"].toString();
            m_currentRoomId = roomId;
            emit roomCreated(roomId);
        }
    }
    else if (path == "/api/v1/rooms/join") {
        if (doc.isObject()) {
            QJsonObject obj = doc.object();
            if (obj.contains("id")) {
                m_currentRoomId = obj["id"].toString();
            }
            emit roomJoined(m_currentRoomId);
        }
    }
    else if (path.startsWith("/api/v1/sync")) {
        if (doc.isObject()) {
            emit syncStatusReceived(doc.object());
        }
    }
    else if (path == "/api/v1/rooms/signal") {
        qDebug() << "NetworkManager: сигнал отправлен";
    }
    else if (path == "/api/v1/rooms/leave") {
        // roomLeft() уже вызван в leaveRoom()
    }
    
    qDebug() << "NetworkManager: ответ от" << path << "статус" << statusCode;
}

void NetworkManager::onSsEReadyRead()
{
    if (!m_sseReply) return;
    
    while (m_sseReply->canReadLine()) {
        QByteArray line = m_sseReply->readLine();
        
        if (line.isEmpty() || line.startsWith(':')) {
            continue;
        }
        
        if (line.startsWith("data: ")) {
            QByteArray jsonData = line.mid(6).trimmed();
            
            QJsonDocument doc = QJsonDocument::fromJson(jsonData);
            if (doc.isObject()) {
                QJsonObject event = doc.object();
                emit roomEvent(event);
                
                QString type = event["type"].toString();
                if (type == "signal") {
                    emit signalReceived(event["data"].toObject());
                }
            }
        }
    }
}

void NetworkManager::onNetworkError(QNetworkReply::NetworkError code)
{
    Q_UNUSED(code)
    // Обработка ошибок происходит в onReplyFinished
}

void NetworkManager::onSslErrors(QNetworkReply *reply, const QList<QSslError> &errors)
{
    for (const QSslError &error : errors) {
        qWarning() << "SSL ошибка:" << error.errorString();
    }
    #ifdef QT_NO_DEBUG
    // В production не игнорируем ошибки
    #else
    reply->ignoreSslErrors();
    #endif
}

void NetworkManager::retryRequest()
{
    if (m_pendingRetry.attempt >= m_maxRetries) {
        qWarning() << "NetworkManager: исчерпаны попытки для" << m_pendingRetry.path;
        emit error(tr("Сервер недоступен после %1 попыток").arg(m_maxRetries));
        
        if (m_serverAvailable) {
            m_serverAvailable = false;
            emit serverUnavailable();
        }
        return;
    }
    
    qDebug() << "NetworkManager: повторная попытка" << (m_pendingRetry.attempt + 1) 
             << "для" << m_pendingRetry.path;
    
    // Выполняем запрос
    if (m_pendingRetry.method == "GET") {
        sendGet(m_pendingRetry.path);
    } else if (m_pendingRetry.method == "POST") {
        sendPost(m_pendingRetry.path, m_pendingRetry.body);
    } else if (m_pendingRetry.method == "DELETE") {
        sendDelete(m_pendingRetry.path);
    }
}

// ── Private methods ───────────────────────────────────────────────────

void NetworkManager::sendGet(const QString &path)
{
    QUrl url(m_serverUrl.toString() + path);
    QNetworkRequest request(url);
    request.setHeader(QNetworkRequest::ContentTypeHeader, "application/json");
    request.setRawHeader("Accept", "application/json");
    // Таймаут 60 секунд на передачу данных
    request.setTransferTimeout(60000);
    
    QNetworkReply *reply = m_network->get(request);
    m_replyMap[reply] = path;
    
    connect(reply, &QNetworkReply::errorOccurred,
            this, &NetworkManager::onNetworkError);
    
    qDebug() << "NetworkManager: GET" << path;
}

void NetworkManager::sendPost(const QString &path, const QJsonObject &body)
{
    QUrl url(m_serverUrl.toString() + path);
    QNetworkRequest request(url);
    request.setHeader(QNetworkRequest::ContentTypeHeader, "application/json");
    request.setRawHeader("Accept", "application/json");
    // Таймаут 60 секунд на передачу данных
    request.setTransferTimeout(60000);
    
    QJsonDocument doc(body);
    QByteArray data = doc.toJson(QJsonDocument::Compact);
    
    QNetworkReply *reply = m_network->post(request, data);
    m_replyMap[reply] = path;
    
    connect(reply, &QNetworkReply::errorOccurred,
            this, &NetworkManager::onNetworkError);
    
    qDebug() << "NetworkManager: POST" << path << data;
}

void NetworkManager::sendDelete(const QString &path)
{
    QUrl url(m_serverUrl.toString() + path);
    QNetworkRequest request(url);
    request.setHeader(QNetworkRequest::ContentTypeHeader, "application/json");
    request.setRawHeader("Accept", "application/json");
    // Таймаут 60 секунд на передачу данных
    request.setTransferTimeout(60000);
    
    QNetworkReply *reply = m_network->deleteResource(request);
    m_replyMap[reply] = path;
    
    connect(reply, &QNetworkReply::errorOccurred,
            this, &NetworkManager::onNetworkError);
    
    qDebug() << "NetworkManager: DELETE" << path;
}

void NetworkManager::sendWithRetry(const QString &method, const QString &path, const QJsonObject &body)
{
    // Сохраняем запрос для возможного retry
    m_pendingRetry.method = method;
    m_pendingRetry.path = path;
    m_pendingRetry.body = body;
    m_pendingRetry.attempt = 0;
    
    // Выполняем первую попытку
    if (method == "GET") {
        sendGet(path);
    } else if (method == "POST") {
        sendPost(path, body);
    } else if (method == "DELETE") {
        sendDelete(path);
    }
}

void NetworkManager::connectToSSE(const QString &path)
{
    disconnectSSE();
    
    QUrl url(m_serverUrl.toString() + path);
    QNetworkRequest request(url);
    request.setRawHeader("Accept", "text/event-stream");
    request.setAttribute(QNetworkRequest::Http2AllowedAttribute, false);
    // SSE соединение должно быть долгоживущим, поэтому таймаут отключен
    request.setTransferTimeout(0);
    
    m_sseReply = m_network->get(request);
    
    connect(m_sseReply, &QNetworkReply::readyRead,
            this, &NetworkManager::onSsEReadyRead);
    connect(m_sseReply, &QNetworkReply::errorOccurred,
            this, [this](QNetworkReply::NetworkError code) {
                qWarning() << "NetworkManager: SSE ошибка:" << code;
                if (m_sseReply) {
                    emit error(tr("SSE ошибка: %1").arg(m_sseReply->errorString()));
                }
            });
    
    qDebug() << "NetworkManager: SSE подключение к" << path;
}

void NetworkManager::disconnectSSE()
{
    if (m_sseReply) {
        m_sseReply->abort();
        m_sseReply->deleteLater();
        m_sseReply = nullptr;
        qDebug() << "NetworkManager: SSE отключено";
    }
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
    
    // Пытаемся прочитать тело ошибки
    QByteArray data = reply->readAll();
    if (!data.isEmpty()) {
        QJsonDocument doc = parseJson(data);
        if (doc.isObject()) {
            QJsonObject obj = doc.object();
            if (obj.contains("error")) {
                errorMessage = obj["error"].toString();
            }
        }
    }
    
    // Проверяем необходимость retry (только для серверных ошибок и таймаутов)
    bool shouldRetry = (statusCode >= 500 || statusCode == 0) && m_pendingRetry.attempt < m_maxRetries;
    
    if (shouldRetry) {
        m_pendingRetry.attempt++;
        int delay = calculateRetryDelay(m_pendingRetry.attempt - 1);
        
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
    // С ограничением в 30 секунд
    int delay = m_retryBaseDelay * (1 << attempt);
    return qMin(delay, 30000);
}
