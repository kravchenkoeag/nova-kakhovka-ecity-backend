# Dockerfile
FROM golang:1.21-alpine AS builder

# Устанавливаем необходимые пакеты
RUN apk add --no-cache git ca-certificates tzdata

# Создаем директорию приложения
WORKDIR /app

# Копируем go mod файлы
COPY go.mod go.sum ./

# Скачиваем зависимости
RUN go mod download

# Копируем исходный код
COPY . .

# Собираем приложение
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main cmd/server/main.go

# Финальный образ
FROM alpine:latest

# Устанавливаем необходимые пакеты
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

# Копируем бинарный файл
COPY --from=builder /app/main .

# Открываем порт
EXPOSE 8080

# Запускаем приложение
CMD ["./main"]

---

# docker-compose.yml
version: '3.8'

services:
  app:
    build: .
    ports:
      - "8080:8080"
    environment:
      - MONGO_URI=mongodb://mongo:27017
      - DATABASE_NAME=nova_kakhovka_ecity
      - JWT_SECRET=your-super-secret-jwt-key
      - ENV=production
    depends_on:
      - mongo
    restart: unless-stopped

  mongo:
    image: mongo:6.0
    ports:
      - "27017:27017"
    volumes:
      - mongodb_data:/data/db
      - ./init-mongo.js:/docker-entrypoint-initdb.d/init-mongo.js:ro
    environment:
      - MONGO_INITDB_ROOT_USERNAME=admin
      - MONGO_INITDB_ROOT_PASSWORD=password
      - MONGO_INITDB_DATABASE=nova_kakhovka_ecity
    restart: unless-stopped

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    restart: unless-stopped

volumes:
  mongodb_data:
  redis_data:

---

# init-mongo.js
db = db.getSiblingDB('nova_kakhovka_ecity');

// Создание пользователя приложения
db.createUser({
  user: 'app_user',
  pwd: 'app_password',
  roles: [
    {
      role: 'readWrite',
      db: 'nova_kakhovka_ecity'
    }
  ]
});

// Создание коллекций с базовой валидацией
db.createCollection('users', {
  validator: {
    $jsonSchema: {
      bsonType: 'object',
      required: ['email', 'password_hash', 'first_name', 'last_name'],
      properties: {
        email: {
          bsonType: 'string',
          description: 'must be a string and is required'
        },
        password_hash: {
          bsonType: 'string',
          description: 'must be a string and is required'
        },
        first_name: {
          bsonType: 'string',
          description: 'must be a string and is required'
        },
        last_name: {
          bsonType: 'string',
          description: 'must be a string and is required'
        }
      }
    }
  }
});

db.createCollection('groups');
db.createCollection('messages');
db.createCollection('announcements');
db.createCollection('events');