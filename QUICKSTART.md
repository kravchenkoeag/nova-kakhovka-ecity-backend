# 🚀 QUICKSTART - Nova Kakhovka e-City Backend

## Швидкий старт за 5 хвилин

### 1️⃣ Клонування репозиторію
```bash
git clone https://github.com/kravchenkoeag/nova-kakhovka-ecity-backend.git
cd nova-kakhovka-ecity-backend
```

### 2️⃣ Встановлення залежностей Go
```bash
# Переконайтесь, що у вас Go >= 1.21
go version

# Завантажте всі залежності
go mod download
```

### 3️⃣ Налаштування бази даних MongoDB

#### Варіант A: Docker (рекомендовано)
```bash
# Запустіть MongoDB в Docker
docker run -d \
  --name nova-kakhovka-mongodb \
  -p 27017:27017 \
  -e MONGO_INITDB_ROOT_USERNAME=admin \
  -e MONGO_INITDB_ROOT_PASSWORD=secretpassword \
  -e MONGO_INITDB_DATABASE=nova_kakhovka_ecity \
  -v mongodb_data:/data/db \
  mongo:7.0
```

#### Варіант B: MongoDB Atlas (хмара)
1. Створіть безкоштовний кластер на [MongoDB Atlas](https://www.mongodb.com/atlas)
2. Отримайте connection string
3. Додайте ваш IP до whitelist

### 4️⃣ Конфігурація змінних оточення
```bash
# Скопіюйте приклад конфігурації
cp .env.example .env

# Відредагуйте .env файл
nano .env
```

**Мінімальна конфігурація (.env):**
```env
# Server Configuration
PORT=8080
HOST=0.0.0.0
ENV=development

# MongoDB Configuration
MONGODB_URI=mongodb://admin:secretpassword@localhost:27017/nova_kakhovka_ecity?authSource=admin
DATABASE_NAME=nova_kakhovka_ecity

# JWT Secret (згенеруйте свій!)
JWT_SECRET=your-super-secret-jwt-key-minimum-32-characters-long

# CORS Origins (для frontend)
ALLOWED_ORIGINS=http://localhost:3000,http://localhost:3001

# Optional: Firebase для push-сповіщень
# FIREBASE_CREDENTIALS_PATH=./firebase-credentials.json
```

### 5️⃣ Запуск сервера
```bash
# Варіант 1: Прямий запуск
go run cmd/server/main.go

# Варіант 2: З hot-reload (для розробки)
# Встановіть air якщо ще не маєте
go install github.com/cosmtrek/air@latest
# Запустіть з hot-reload
air

# Варіант 3: Збірка та запуск
go build -o server cmd/server/main.go
./server
```

### ✅ Перевірка роботи

**Backend повинен запуститися на `http://localhost:8080`**

```bash
# Перевірте health endpoint
curl http://localhost:8080/health

# Отримаєте відповідь:
# {
#   "status": "ok",
#   "timestamp": "2024-12-27T12:00:00Z",
#   "version": "1.0.0",
#   "services": {
#     "database": "connected",
#     "websocket": "running"
#   }
# }
```

### 🔧 Швидке тестування API

#### Реєстрація користувача
```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "username": "testuser",
    "email": "test@example.com",
    "password": "SecurePass123!",
    "first_name": "Test",
    "last_name": "User"
  }'
```

#### Вхід в систему
```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "SecurePass123!"
  }'
```

### 📝 Корисні команди

```bash
# Запуск тестів
go test ./...

# Запуск з певним .env файлом
go run cmd/server/main.go --env=.env.local

# Збірка для production
CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o server cmd/server/main.go

# Перевірка покриття тестами
go test -cover ./...

# Генерація документації API
swag init -g cmd/server/main.go
```

### 🐳 Docker запуск (опціонально)

```bash
# Збірка Docker образу
docker build -t nova-kakhovka-backend .

# Запуск контейнера
docker run -d \
  --name nova-kakhovka-backend \
  -p 8080:8080 \
  --env-file .env \
  --link nova-kakhovka-mongodb:mongodb \
  nova-kakhovka-backend
```

### 🚨 Вирішення проблем

**MongoDB не підключається:**
- Перевірте чи запущений MongoDB: `docker ps`
- Перевірте правильність MONGODB_URI в .env
- Перевірте firewall/порти

**JWT помилки:**
- Згенеруйте новий JWT_SECRET: `openssl rand -base64 32`
- Переконайтесь, що секрет однаковий для backend та frontend

**CORS помилки:**
- Додайте URL вашого frontend до ALLOWED_ORIGINS в .env
- Перезапустіть сервер після змін

### 📚 Наступні кроки

1. **Налаштуйте Frontend:**
   ```bash
   cd ../nova-kakhovka-ecity-frontend
   # Дивіться QUICKSTART.md для frontend
   ```

2. **Активуйте додаткові сервіси:**
    - Firebase для push-сповіщень
    - Redis для кешування
    - Elasticsearch для пошуку

3. **Ознайомтесь з документацією:**
    - [API Documentation](./docs/API.md)
    - [Architecture Overview](./docs/ARCHITECTURE.md)
    - [Development Guide](./DEVELOPMENT_CHECKLIST.md)

---

💡 **Підказка:** Використовуйте `make` команди для швидкого управління:
```bash
make run      # Запустити сервер
make test     # Запустити тести
make build    # Зібрати бінарний файл
make docker   # Зібрати Docker образ
```

📧 **Потрібна допомога?** Створіть issue на GitHub або зв'яжіться з командою розробки.