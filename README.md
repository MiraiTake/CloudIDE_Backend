# CloudIDE — Backend

Бэкенд написан на **Go**, запускается на порту `:8888`, использует **MySQL** и управляет Docker-контейнерами как изолированной средой выполнения для каждого проекта.

---

## Стек и зависимости

| Компонент | Роль |
|---|---|
| `gorilla/mux` | HTTP-роутер |
| `golang-jwt/jwt v4` | JWT access-токены (HS256) |
| `bcrypt` | Хэширование паролей |
| `golang.org/x/oauth2` | GitHub OAuth2 |
| `google/uuid` | Генерация UUID |
| `creack/pty` | PTY для WebSocket-терминала |
| `gorilla/websocket` | WebSocket upgrade |
| `go-sql-driver/mysql` | MySQL-драйвер |
| `mailer/resend` | Отправка email через Resend API |

---

## Переменные окружения

```
DB_USER, DB_PASS, DB_HOST, DB_PORT, DB_NAME   — подключение к MySQL
GITHUB_CLIENT_ID, GITHUB_CLIENT_SECRET        — OAuth2 GitHub
GITHUB_OAUTH_REDIRECT_URL                     — redirect URI после OAuth
RESEND_API_KEY                                — ключ Resend для email
```

---

## Структура файлов

```
main.go              — точка входа, регистрация маршрутов, тикер очистки токенов
models.go            — модели: User, Project, RefreshToken, Claims, StatsResponse
db.go                — подключение к MySQL через env-переменные
auth_handlers.go     — регистрация, логин, подтверждение кода, GitHub OAuth, сессии
token_service.go     — генерация/валидация/отзыв access и refresh токенов
project_handlers.go  — CRUD проектов + клонирование с GitHub
file_handlers.go     — файловые операции внутри контейнера
terminal.go          — WebSocket-терминал с PTY
docker_ops.go        — Docker-операции: старт/стоп/авто-стоп, установка языков
changes_handlers.go  — отслеживание изменённых файлов
utils.go             — вспомогательные функции (sanitize путей, getUserID и др.)
crypto_utils.go      — шифрование/дешифрование GitHub-токенов
mailer/resend.go     — отправка кода подтверждения email
```

---

## Маршруты API

### Публичные (без JWT)

| Метод | Путь | Описание |
|---|---|---|
| `POST` | `/register` | Регистрация пользователя |
| `POST` | `/login` | Вход, возвращает `access_token` + `refresh_token` |
| `POST` | `/verify-code` | Подтверждение email кодом |
| `GET` | `/auth/github/login` | Редирект на GitHub OAuth |
| `GET` | `/auth/github/callback` | Обработка callback от GitHub |
| `POST` | `/auth/refresh` | Обновление access-токена по refresh-токену |
| `POST` | `/auth/logout` | Отзыв refresh-токена (выход) |

### Защищённые (требуют `Authorization: Bearer <token>`)

#### Пользователь
| Метод | Путь | Описание |
|---|---|---|
| `GET` | `/users/{id}` | Профиль пользователя (только свой) |
| `GET` | `/auth/sessions` | Список активных сессий |
| `POST` | `/auth/logout-all` | Выход на всех устройствах |

#### Проекты
| Метод | Путь | Описание |
|---|---|---|
| `POST` | `/project` | Создать проект (запускает Docker-контейнер) |
| `DELETE` | `/project/{name}` | Удалить проект и контейнер |
| `GET` | `/projects` | Список проектов пользователя |
| `GET` | `/projects/stats` | Статистика по проектам |
| `POST` | `/project/clone` | Клонировать GitHub-репозиторий |
| `POST` | `/project/start/{name}` | Запустить остановленный контейнер |

#### Файлы и редактор
| Метод | Путь | Описание |
|---|---|---|
| `GET` | `/project/{name}/ws` | WebSocket-терминал (PTY в Docker) |
| `GET` | `/project/{name}/files` | Список файлов и папок |
| `GET` | `/project/{name}/file?filename=...` | Получить содержимое файла |
| `POST` | `/project/{name}/file` | Сохранить содержимое файла |
| `DELETE` | `/project/{name}/file?filename=...` | Удалить файл/папку |
| `POST` | `/project/{name}/file/create` | Создать пустой файл |
| `POST` | `/project/{name}/folder` | Создать папку |
| `POST` | `/project/{name}/move` | Переместить/переименовать файл |
| `POST` | `/project/{name}/run` | Запустить файл в контейнере |
| `GET` | `/project/{name}/changed-files` | Получить список изменённых файлов |
| `POST` | `/project/{name}/changed-files` | Сохранить список изменённых файлов |

---

## Авторизация и токены

Используется схема **access token + refresh token**.

- **Access token** — JWT (HS256), живёт **1 час**. Содержит `user_id` в payload.
- **Refresh token** — случайная base64-строка (32 байта), живёт **30 дней**. Хранится в таблице `refresh_tokens` с привязкой к устройству и IP.
- Каждые **24 часа** горутина очищает истёкшие/отозванные токены из БД.

### Схема регистрации
1. `POST /register` → хэшируется пароль, вставляется запись с `email_verified = FALSE`.
2. Генерируется 6-значный код, отправляется на email через Resend.
3. `POST /verify-code` → код проверяется, помечается как использованный.

### GitHub OAuth
1. Клиент делает `GET /auth/github/login` с JWT в заголовке или query `?token=`.
2. Сервер сохраняет `state → userID` в памяти и делает редирект на GitHub.
3. После подтверждения: GitHub-токен **шифруется** (`crypto_utils.go`) и сохраняется в БД вместе с `github_login`.
4. Клиент редиректится на `androidide://auth/success?token=<jwt>`.

---

## Docker-контейнеры

Каждый проект — отдельный Ubuntu-контейнер.

**Создание проекта (`POST /project`):**
1. `docker run -dit --name container_<uuid> ubuntu:latest bash`
2. Создаются директории `/data/changes` и `/<project_name>` внутри контейнера.
3. `apt update && apt upgrade -y && apt install -y git`
4. Устанавливаются запрошенные языки: `python`, `node`, `dart`, `java`, `go`, `php`.
5. Запись сохраняется в БД. При ошибке — контейнер удаляется откатом.

**Авто-стоп:** при закрытии WebSocket-соединения терминала запускается таймер на **5 минут**, после чего контейнер останавливается (`docker stop`). При новом подключении контейнер перезапускается.

**WebSocket-терминал:**
- Апгрейд соединения до WebSocket.
- `docker exec -it <containerID> bash` запускается через PTY (`creack/pty`).
- Вывод PTY транслируется клиенту; ввод клиента — пишется в PTY.
- При наличии GitHub-токена автоматически создаётся `/root/.netrc` и настраивается `git config`.

**Запуск файла (`POST /project/{name}/run`):** определяет рантайм по расширению (`.py`, `.js`, `.dart`, `.go`, `.php`, `.java`) и выполняет файл внутри контейнера.

---

## Безопасность путей

Все файловые операции прогоняются через `sanitizeContainerPath` (`utils.go`), которая формирует путь вида `/<projectName>/<filename>` и блокирует path traversal (символы `..`).

---

## База данных (MySQL)

Основные таблицы:

| Таблица | Описание |
|---|---|
| `users` | `id`, `username`, `email`, `password` (bcrypt), `github_login`, `github_token` (зашифрован), `email_verified` |
| `projects` | `id`, `user_id`, `name`, `docker_id`, `created_at`, `updated_at` |
| `refresh_tokens` | `id`, `user_id`, `token`, `expires_at`, `last_used_at`, `device_info`, `ip_address`, `revoked` |
| `email_codes` | `user_id`, `email`, `code`, `expires`, `used` |
