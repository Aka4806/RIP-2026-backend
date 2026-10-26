# SQL Index API ЛР4

Go / Gin, PostgreSQL, MinIO, JWT HS256 и Redis blacklist.

Ровно 10 методов API, как в ЛР3. Три метода пользователей: регистрация, аутентификация и деавторизация. Маршрут POST /api/users/authorization выполняет деавторизацию и отзывает токен. Лента без ID и с ID — варианты одного метода.

## Запуск

В PowerShell из этой папки:

```powershell
.\run.ps1
```

Скрипт читает локальный `.env`, запускает Redis и сервер. Для нового компьютера скопируйте `.env.example` в `.env`, заполните параметры PostgreSQL и MinIO и задайте случайный JWT_SECRET длиной минимум 32 байта. PostgreSQL и MinIO должны быть запущены; бакет media должен существовать. Схема новой БД — `database/schema.sql`. Локальная БД ЛР4 — `rip_lab4`, порт приложения — 18084. Данные ЛР3 не изменяются.

Swagger: http://127.0.0.1:18084/swagger/index.html

1. Выполните POST /api/users для регистрации (username и password).
2. Выполните POST /api/users/authentication с теми же данными.
3. Скопируйте access_token, нажмите Authorize и вставьте `Bearer <access_token>`.
4. Выполните POST /api/indexes с name, image, video. creator_id берётся из JWT.
5. GET /api/indexes/draft возвращает свой черновик; PUT /api/indexes/{id}/publication публикует его.
6. POST /api/users/authorization отзывает JWT до истечения его срока действия. Нужен повторный вход.

Список GET /api/indexes и лента GET /api/feed/ или /api/feed/{id} публичны: отсутствующий, неверный, истёкший и отозванный токен не мешают чтению. При действительном токене возвращаются персональные is_owner/is_liked. При отсутствии действительного токена эти поля равны 0. Регистрация и вход также публичны. Остальные методы требуют JWT; без него возвращается 401. Чужая карточка недоступна для изменения (404). Недоступность Redis запрещает защищённые операции (503). Публикация допустима только из draft.

Токен содержит sub (ID), role=creator, iss, aud, iat, nbf, exp и случайный jti. Срок по умолчанию 1 час. Redis хранит ключ lr4:blacklist:<jti>, значение — ID пользователя, TTL — оставшийся срок JWT. Redis включён с AOF и noeviction. Не удаляйте его volume при действующих токенах.

## Postman и Redis

Импортируйте `postman/SQL-Index-LR4.postman_collection.json`. Заполните username/password; вход автоматически сохраняет token. Для гостевого запроса установите No Auth. Для переноса JWT из Swagger задайте переменную token без префикса Bearer.

```powershell
docker exec lr4_redis redis-cli --scan --pattern 'lr4:blacklist:*'
docker exec lr4_redis redis-cli GET 'lr4:blacklist:<jti>'
docker exec lr4_redis redis-cli TTL 'lr4:blacklist:<jti>'
```

## Проверки

```powershell
go test ./...
$env:TEST_DATABASE='1'
$env:PGDATABASE='rip_lab4'
$env:PGPASSWORD='ваш пароль'
go test ./... -count=1
```

Интеграционные тесты работают в транзакциях с откатом, но увеличивают последовательности ID. Используйте только учебную/тестовую БД без параллельной работы. TEST_MINIO=1 дополнительно проверяет реальную загрузку в MinIO с очисткой тестовых объектов.

Swagger генерируется из комментариев:

```powershell
go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g cmd/awesomeProject/main.go --parseInternal
```
