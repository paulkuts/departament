FROM alpine:3.21

# Пакеты не устанавливаются: бинарник статический (CGO_ENABLED=0, чистый Go
# включая modernc SQLite), внешних TLS-запросов у приложения нет.
# Работает под встроенным пользователем nobody (65534) — apk/adduser не нужны,
# а сам apk в контейнере на rf-vps отвечает по 60 секунд (IPv6-фолбэк).
WORKDIR /app

COPY departament /app/departament
COPY internal/db/migration /app/internal/db/migration
COPY configs /app/configs

RUN mkdir -p /app/data/photos /app/data/avatars /app/export && \
    chown -R 65534:65534 /app

USER 65534:65534

EXPOSE 8080

CMD ["./departament"]
