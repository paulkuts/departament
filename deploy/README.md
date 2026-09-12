## Развёртывание (12.09.2026)

Продовой инстанс: `rf-vps`, домен `https://mitm.kutspv.ru` (Caddy на rf-vps → 127.0.0.1:8085 → контейнер `departament-mitm`).

- Рецепт развёртывания — каталог `deploy/` (Dockerfile, docker-compose.yml, шаблон `configs/default.yaml`). Бинарник статический: `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w"`.
- Конфиг и секрет JWT не в образе: `configs/` монтируется в контейнер только для чтения, `jwt_secret` подставляется на сервере при установке.
- Секрет и `data/` живут только на сервере; в репозиторий не попадают.
- Изменение против архива передачи: `internal/handler/auth.go` — refresh-cookie переведён на `Secure: true` (закрыто замечание из раздела «Перед production» HANDOFF.md). Приложение доступно исключительно через HTTPS-reverse-proxy, поэтому флаг безопасен.
- `.gitignore` исходного проекта игнорировал `pkg/`, `go.mod`, `go.sum`, из-за чего снимок нельзя было собрать из репозитория. Список приведён в рабочий вид; демонстрационные и реальные данные, `.env`, `configs/local.yaml` и `configs/production.yaml` по-прежнему игнорируются.
- База развёрнута пустой: переноса данных старого инстанса нет (реальная база в передаче не участвовала). Старый инстанс и его данные сохранены в архивах на rf-vps в `/root/backups/`.
- First admin создаётся вручную: зарегистрировать аккаунт, затем `UPDATE users SET role='admin' WHERE id=?`.
