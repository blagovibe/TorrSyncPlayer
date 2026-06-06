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

#include <QDebug>
#include <QJsonDocument>
#include <QJsonObject>
#include <QJsonArray>
#include <QEventLoop>
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
    // Максимальная длина magnet URI
    const int MaxMagnetUriLength = 8192;
    // Максимальная длина ID торрента
    const int MaxTorrentIdLength = 40;
    // Максимальная длина ID комнаты
    const int MaxRoomIdLength = 32;

    QString buildApiPath(const QString &basePath, const QString &id)
    {
        QString sanitized = id;
        sanitized.remove(QRegularExpression("[^a-fA-F0-9]"));
        return basePath + "/" + sanitized;
    }
}

NetworkManager::NetworkManager(QObject *parent)
    : INetworkManager(parent)
    , m_network(new QNetworkAccessManager(this))
    , m_serverUrl(QStringLiteral("http://localhost:8889"))
    , m_sseReply(nullptr)
    , m_retryTimer(new QTimer(this))
    , m_sseReconnectTimer(new QTimer(this))
{
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

    qDebug() << "NetworkManager: инициализирован с URL" << m_serverUrl.toString();
}

NetworkManager::~NetworkManager()
{
    // Отключаем SSE при уничтожении
    disconnectSSE();

    // Отменяем все активные запросы и очищаем карту
    {
        QMutexLocker locker(&m_replyMutex);
        for (auto it = m_replyMap.begin(); it != m_replyMap.end(); ++it) {
            QNetworkReply *reply = it.key();
            if (reply) {
                reply->abort();
                reply->deleteLater();
            }
        }
        m_replyMap.clear();
    }

    // Очищаем кэш сети
    m_network->clearAccessCache();
    m_network->clearConnectionCache();

    // Останавливаем таймеры
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

    if (magnetUri.length() > MaxMagnetUriLength) {
        emit error(tr("Magnet-ссылка слишком длинная (максимум %1 символов)").arg(MaxMagnetUriLength));
        return;
    }

    if (!magnetUri.startsWith("magnet:?")) {
        emit error(tr("Некорректная magnet-ссылка. Должна начинаться с 'magnet:?'"));
        return;
    }

    QJsonObject body;
    body["magnetUri"] = magnetUri;
    sendWithRetry("POST", "/api/v1/torrents", body);
}

void NetworkManager::removeTorrent(const QString &id)
{
    // Валидация ID торрента
    if (id.isEmpty()) {
        emit error(tr("ID торрента не может быть пустым"));
        return;
    }

    if (id.length() > MaxTorrentIdLength) {
        emit error(tr("ID торрента слишком длинный"));
        return;
    }

    sendWithRetry("DELETE", buildApiPath("/api/v1/torrents", id));
}

void NetworkManager::listTorrents()
{
    sendWithRetry("GET", "/api/v1/torrents");
}

void NetworkManager::getFiles(const QString &torrentId)
{
    // Валидация ID торрента
    if (torrentId.isEmpty()) {
        emit error(tr("ID торрента не может быть пустым"));
        return;
    }

    sendWithRetry("GET", buildApiPath("/api/v1/torrents", torrentId) + "/files");
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
    sendWithRetry("POST", buildApiPath("/api/v1/torrents", torrentId) + "/select", body);
}

// ── Room API ──────────────────────────────────────────────────────────

void NetworkManager::createRoom(const QString &name, const QString &password)
{
    // Валидация имени комнаты
    if (name.isEmpty()) {
        emit error(tr("Имя комнаты не может быть пустым"));
        return;
    }

    if (name.length() < 2) {
        emit error(tr("Имя комнаты должно содержать минимум 2 символа"));
        return;
    }

    if (name.length() > 50) {
        emit error(tr("Имя комнаты слишком длинное (максимум 50 символов)"));
        return;
    }

    QJsonObject body;
    body["name"] = name;
    if (!password.isEmpty()) {
        body["password"] = password;
    }
    sendWithRetry("POST", "/api/v1/rooms", body);
}

void NetworkManager::joinRoom(const QString &roomId, const QString &password)
{
    // Валидация ID комнаты
    if (roomId.isEmpty()) {
        emit error(tr("ID комнаты не может быть пустым"));
        return;
    }

    if (roomId.length() > MaxRoomIdLength) {
        emit error(tr("ID комнаты слишком длинный"));
        return;
    }

    {
        QMutexLocker locker(&m_roomIdMutex);
        m_currentRoomId = roomId;
    }

    QJsonObject body;
    body["roomId"] = roomId;
    if (!password.isEmpty()) {
        body["password"] = password;
    }
    sendWithRetry("POST", "/api/v1/rooms/join", body);

    // Подключаемся к SSE потоку комнаты
    // connectToSSE уже отключает предыдущий SSE
    connectToSSE("/api/v1/rooms-events");
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

    sendPost("/api/v1/rooms/leave", QJsonObject());

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
    // Валидация позиции
    if (position < 0) {
        emit error(tr("Позиция не может быть отрицательной"));
        return;
    }

    QJsonObject body;
    body["position"] = position;
    sendPost("/api/v1/sync/seek", body);
}

// ── Утилиты ───────────────────────────────────────────────────────────

QString NetworkManager::streamUrl(const QString &torrentId) const
{
    QString sanitized = torrentId;
    sanitized.remove(QRegularExpression("[^a-fA-F0-9]"));
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

    reply->deleteLater();

    // Получаем путь запроса для идентификации (потокобезопасно)
    QString path;
    {
        QMutexLocker locker(&m_replyMutex);
        path = m_replyMap.take(reply);
    }
    if (path.isEmpty()) {
        path = reply->url().path();
    }

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

    // Читаем ответ
    QByteArray data = reply->readAll();
    QJsonDocument doc = parseJson(data);

    if (path != "/api/v1/torrents" && path != "/api/v1/rooms/join" && !path.startsWith("/api/v1/torrents/")) {
        if (doc.isNull() && !data.isEmpty()) {
            emit error(tr("Ошибка парсинга JSON от %1").arg(path));
            return;
        }
    }

    if (doc.isNull()) {
        emit error(tr("Ошибка парсинга JSON от %1").arg(path));
        return;
    }

    // Обрабатываем ответ в зависимости от пути
    int statusCode = 0;
    QVariant statusVariant = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute);
    if (statusVariant.isValid()) {
        statusCode = statusVariant.toInt();
    }

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
        // Используем QRegularExpression для корректного извлечения torrentId
        QRegularExpression re("^/api/v1/torrents/([^/]+)/files$");
        QRegularExpressionMatch match = re.match(path);
        if (match.hasMatch()) {
            QString torrentId = match.captured(1);
            if (doc.isArray()) {
                emit filesReceived(torrentId, doc.array());
            }
        }
    }
    else if (path.startsWith("/api/v1/torrents/") && path.endsWith("/select")) {
        qDebug() << "NetworkManager: file selected successfully";
    }
    else if (path == "/api/v1/rooms") {
        if (doc.isObject()) {
            QJsonObject obj = doc.object();
            QString roomId = obj["id"].toString();
            {
                QMutexLocker locker(&m_roomIdMutex);
                m_currentRoomId = roomId;
            }
            emit roomCreated(roomId);
        }
    }
    else if (path == "/api/v1/rooms/join") {
        if (doc.isObject()) {
            QJsonObject obj = doc.object();
            if (obj.contains("id")) {
                QMutexLocker locker(&m_roomIdMutex);
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
    // QPointer автоматически становится nullptr если объект удалён
    QNetworkReply *sseReply = m_sseReply;
    if (!sseReply) return;

    // Максимальный размер буфера SSE (1 MB)
    const int MaxSSEBufferSize = 1024 * 1024;
    int sseBufferUsage = 0;

    while (sseReply->canReadLine()) {
        QByteArray line = sseReply->readLine();

        sseBufferUsage += line.size();
        if (sseBufferUsage > MaxSSEBufferSize) {
            qWarning() << "NetworkManager: превышен лимит буфера SSE";
            emit error(tr("Превышен лимит буфера SSE"));
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
            emit roomEvent(event);

            QString type = event["type"].toString();
            if (type == "signal") {
                emit signalReceived(event["data"].toObject());
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
        qWarning() << "SSL error:" << error.errorString();
    }
    qWarning() << "NetworkManager: SSL errors rejected";
}

void NetworkManager::retryRequest()
{
    if (m_pendingRetry.attempt >= m_maxRetries) {
        qWarning() << "NetworkManager: исчерпаны попытки для" << m_pendingRetry.path;
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
        sendGet(m_pendingRetry.path);
    } else if (m_pendingRetry.method == "POST") {
        sendPost(m_pendingRetry.path, m_pendingRetry.body);
    } else if (m_pendingRetry.method == "DELETE") {
        sendDelete(m_pendingRetry.path);
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

    // Переподключаемся
    connectToSSE("/api/v1/rooms-events");

    // Увеличиваем счётчик попыток
    m_sseReconnectAttempts.storeRelaxed(m_sseReconnectAttempts.loadRelaxed() + 1);
}

// ── Private methods ───────────────────────────────────────────────────

void NetworkManager::sendGet(const QString &path)
{
    QUrl url(m_serverUrl.toString() + path);
    QNetworkRequest request(url);
    request.setHeader(QNetworkRequest::ContentTypeHeader, "application/json");
    request.setRawHeader("Accept", "application/json");
    // Таймаут 60 секунд на передачу данных
    request.setTransferTimeout(TransferTimeout);

    QNetworkReply *reply = m_network->get(request);

    {
        QMutexLocker locker(&m_replyMutex);
        m_replyMap[reply] = path;
    }

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
    request.setTransferTimeout(TransferTimeout);

    QJsonDocument doc(body);
    QByteArray data = doc.toJson(QJsonDocument::Compact);

    QNetworkReply *reply = m_network->post(request, data);

    {
        QMutexLocker locker(&m_replyMutex);
        m_replyMap[reply] = path;
    }

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
    request.setTransferTimeout(TransferTimeout);

    QNetworkReply *reply = m_network->deleteResource(request);

    {
        QMutexLocker locker(&m_replyMutex);
        m_replyMap[reply] = path;
    }

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
    // Отключаем предыдущий SSE перед подключением нового (предотвращает утечку)
    disconnectSSE();

    QUrl url(m_serverUrl.toString() + path);
    QNetworkRequest request(url);
    request.setRawHeader("Accept", "text/event-stream");
    request.setAttribute(QNetworkRequest::Http2AllowedAttribute, false);
    // SSE соединение должно быть долгоживущим, поэтому таймаут отключен
    request.setTransferTimeout(0);

    m_sseReply = m_network->get(request);

    {
        QMutexLocker locker(&m_replyMutex);
        m_replyMap[m_sseReply] = path;
    }

    connect(m_sseReply, &QNetworkReply::readyRead,
            this, &NetworkManager::onSsEReadyRead);
    connect(m_sseReply, &QNetworkReply::errorOccurred,
            this, [this](QNetworkReply::NetworkError code) {
                Q_UNUSED(code)
                qWarning() << "NetworkManager: SSE ошибка";
                QNetworkReply *reply = m_sseReply;
                if (reply) {
                    emit error(tr("SSE ошибка: %1").arg(reply->errorString()));
                } else {
                    emit error(tr("SSE ошибка: соединение потеряно"));
                }

                // Запускаем переподключение с exponential backoff
                {
                    QMutexLocker locker(&m_roomIdMutex);
                    if (!m_currentRoomId.isEmpty() && m_sseReconnectAttempts.loadRelaxed() < MaxSSEReconnectAttempts) {
                        int delay = qMin(SSEReconnectBaseDelay * (1 << m_sseReconnectAttempts.loadRelaxed()), SSEReconnectMaxDelay);
                        qDebug() << "NetworkManager: SSE переподключение через" << delay << "мс";
                        m_sseReconnectTimer->start(delay);
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
    return qMin(delay, SSEReconnectMaxDelay);
}
