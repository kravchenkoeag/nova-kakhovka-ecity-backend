# Nova Kakhovka e-City Backend - Makefile

# Змінні
APP_NAME := nova-kakhovka-ecity-backend
BINARY_NAME := server
MAIN_PATH := cmd/server/main.go
BUILD_DIR := build
DOCKER_IMAGE := nova-kakhovka-ecity-backend
DOCKER_TAG := latest

# Go змінні
GO := go
GOFMT := gofmt
GOVET := go vet
GOTEST := go test
GOGET := go get
GOMOD := go mod
GOBUILD := go build
GOCLEAN := go clean

# Флаги збірки
BUILD_FLAGS := -v
LDFLAGS := -ldflags "-w -s"

# Кольори для виводу
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[0;33m
BLUE := \033[0;34m
NC := \033[0m # No Color

.PHONY: all build run clean test coverage lint fmt vet deps docker help

# За замовчуванням показуємо help
all: help

## help: Показати цю довідку
help:
@echo "$(BLUE)Nova Kakhovka e-City Backend - Makefile Commands$(NC)"
@echo ""
@echo "$(GREEN)Available commands:$(NC)"
@awk '/^##/ { \
getline nextLine; \
if (match(nextLine, /^[a-zA-Z_-]+:/)) { \
sub(/^## /, "", $$0); \
sub(/:.*/, "", nextLine); \
printf "  $(YELLOW)%-20s$(NC) %s\n", nextLine, $$0 \
} \
}' $(MAKEFILE_LIST)
@echo ""

## run: Запустити сервер в режимі розробки
run:
@echo "$(BLUE)🚀 Starting development server...$(NC)"
$(GO) run $(MAIN_PATH)

## run-watch: Запустити сервер з hot-reload (потрібен air)
run-watch:
@echo "$(BLUE)🔄 Starting server with hot-reload...$(NC)"
@which air > /dev/null || (echo "$(RED)❌ air not installed. Run: go install github.com/cosmtrek/air@latest$(NC)" && exit 1)
air

## build: Зібрати бінарний файл
build:
@echo "$(BLUE)🔨 Building $(APP_NAME)...$(NC)"
@mkdir -p $(BUILD_DIR)
$(GOBUILD) $(BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
@echo "$(GREEN)✅ Build completed: $(BUILD_DIR)/$(BINARY_NAME)$(NC)"

## build-linux: Зібрати для Linux
build-linux:
@echo "$(BLUE)🐧 Building for Linux...$(NC)"
@mkdir -p $(BUILD_DIR)
GOOS=linux GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux $(MAIN_PATH)
@echo "$(GREEN)✅ Linux build completed$(NC)"

## build-windows: Зібрати для Windows
build-windows:
@echo "$(BLUE)🪟 Building for Windows...$(NC)"
@mkdir -p $(BUILD_DIR)
GOOS=windows GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME).exe $(MAIN_PATH)
@echo "$(GREEN)✅ Windows build completed$(NC)"

## build-mac: Зібрати для macOS
build-mac:
@echo "$(BLUE)🍎 Building for macOS...$(NC)"
@mkdir -p $(BUILD_DIR)
GOOS=darwin GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin $(MAIN_PATH)
@echo "$(GREEN)✅ macOS build completed$(NC)"

## build-all: Зібрати для всіх платформ
build-all: build-linux build-windows build-mac
@echo "$(GREEN)✅ All builds completed$(NC)"

## clean: Очистити артефакти збірки
clean:
@echo "$(YELLOW)🧹 Cleaning...$(NC)"
$(GOCLEAN)
rm -rf $(BUILD_DIR)
rm -f coverage.out coverage.html
@echo "$(GREEN)✅ Cleaned$(NC)"

## test: Запустити тести
test:
@echo "$(BLUE)🧪 Running tests...$(NC)"
$(GOTEST) -v ./...

## test-short: Запустити короткі тести
test-short:
@echo "$(BLUE)🧪 Running short tests...$(NC)"
$(GOTEST) -short ./...

## coverage: Згенерувати звіт покриття тестами
coverage:
@echo "$(BLUE)📊 Generating coverage report...$(NC)"
$(GOTEST) -coverprofile=coverage.out ./...
$(GO) tool cover -html=coverage.out -o coverage.html
@echo "$(GREEN)✅ Coverage report: coverage.html$(NC)"
@echo "$(YELLOW)📈 Coverage summary:$(NC)"
$(GO) tool cover -func=coverage.out

## benchmark: Запустити бенчмарки
benchmark:
@echo "$(BLUE)⚡ Running benchmarks...$(NC)"
$(GOTEST) -bench=. -benchmem ./...

## lint: Запустити лінтер (потрібен golangci-lint)
lint:
@echo "$(BLUE)🔍 Running linter...$(NC)"
@which golangci-lint > /dev/null || (echo "$(RED)❌ golangci-lint not installed. Visit: https://golangci-lint.run/usage/install/$(NC)" && exit 1)
golangci-lint run

## fmt: Форматувати код
fmt:
@echo "$(BLUE)💅 Formatting code...$(NC)"
$(GOFMT) -w .
@echo "$(GREEN)✅ Code formatted$(NC)"

## vet: Запустити go vet
vet:
@echo "$(BLUE)🔍 Running go vet...$(NC)"
$(GOVET) ./...
@echo "$(GREEN)✅ No issues found$(NC)"

## deps: Завантажити залежності
deps:
@echo "$(BLUE)📦 Downloading dependencies...$(NC)"
$(GOMOD) download
$(GOMOD) tidy
@echo "$(GREEN)✅ Dependencies updated$(NC)"

## deps-update: Оновити залежності до останніх версій
deps-update:
@echo "$(BLUE)⬆️ Updating dependencies...$(NC)"
$(GOGET) -u ./...
$(GOMOD) tidy
@echo "$(GREEN)✅ Dependencies updated$(NC)"

## security: Перевірка безпеки (потрібен gosec)
security:
@echo "$(BLUE)🔐 Running security scan...$(NC)"
@which gosec > /dev/null || (echo "$(RED)❌ gosec not installed. Run: go install github.com/securego/gosec/v2/cmd/gosec@latest$(NC)" && exit 1)
gosec ./...

## docker-build: Зібрати Docker образ
docker-build:
@echo "$(BLUE)🐳 Building Docker image...$(NC)"
docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .
@echo "$(GREEN)✅ Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)$(NC)"

## docker-run: Запустити Docker контейнер
docker-run:
@echo "$(BLUE)🐳 Running Docker container...$(NC)"
docker run -d \
--name $(APP_NAME) \
-p 8080:8080 \
--env-file .env \
$(DOCKER_IMAGE):$(DOCKER_TAG)
@echo "$(GREEN)✅ Container started$(NC)"

## docker-stop: Зупинити Docker контейнер
docker-stop:
@echo "$(YELLOW)⏹️ Stopping Docker container...$(NC)"
docker stop $(APP_NAME)
docker rm $(APP_NAME)
@echo "$(GREEN)✅ Container stopped$(NC)"

## docker-logs: Показати логи Docker контейнера
docker-logs:
docker logs -f $(APP_NAME)

## docker-compose-up: Запустити через docker-compose
docker-compose-up:
@echo "$(BLUE)🐳 Starting with docker-compose...$(NC)"
docker-compose up -d
@echo "$(GREEN)✅ Services started$(NC)"

## docker-compose-down: Зупинити docker-compose
docker-compose-down:
@echo "$(YELLOW)⏹️ Stopping docker-compose...$(NC)"
docker-compose down
@echo "$(GREEN)✅ Services stopped$(NC)"

## migrate-up: Запустити міграції БД
migrate-up:
@echo "$(BLUE)📤 Running database migrations...$(NC)"
migrate -path ./migrations -database "$(DATABASE_URL)" up

## migrate-down: Відкотити міграції БД
migrate-down:
@echo "$(YELLOW)📥 Rolling back database migrations...$(NC)"
migrate -path ./migrations -database "$(DATABASE_URL)" down

## seed: Заповнити БД тестовими даними
seed:
@echo "$(BLUE)🌱 Seeding database...$(NC)"
$(GO) run scripts/seed.go

## swagger: Згенерувати Swagger документацію
swagger:
@echo "$(BLUE)📚 Generating Swagger documentation...$(NC)"
@which swag > /dev/null || (echo "$(RED)❌ swag not installed. Run: go install github.com/swaggo/swag/cmd/swag@latest$(NC)" && exit 1)
swag init -g $(MAIN_PATH) -o ./docs/swagger
@echo "$(GREEN)✅ Swagger docs generated$(NC)"

## proto: Згенерувати protobuf файли
proto:
@echo "$(BLUE)🔧 Generating protobuf files...$(NC)"
protoc --go_out=. --go-grpc_out=. proto/*.proto
@echo "$(GREEN)✅ Protobuf files generated$(NC)"

## install-tools: Встановити необхідні інструменти розробки
install-tools:
@echo "$(BLUE)🛠️ Installing development tools...$(NC)"
go install github.com/cosmtrek/air@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install github.com/securego/gosec/v2/cmd/gosec@latest
go install github.com/swaggo/swag/cmd/swag@latest
go install -tags 'mongodb' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
@echo "$(GREEN)✅ Tools installed$(NC)"

## check: Запустити всі перевірки (fmt, vet, lint, test)
check: fmt vet lint test
@echo "$(GREEN)✅ All checks passed$(NC)"

## ci: Команди для CI/CD
ci: deps check build
@echo "$(GREEN)✅ CI pipeline completed$(NC)"

## dev: Налаштування для розробки
dev: deps install-tools
@echo "$(GREEN)✅ Development environment ready$(NC)"
@echo "$(YELLOW)💡 Run 'make run-watch' to start development server with hot-reload$(NC)"

## prod: Збірка для production
prod: clean deps test build-linux
@echo "$(GREEN)✅ Production build ready$(NC)"
@echo "$(YELLOW)📦 Binary location: $(BUILD_DIR)/$(BINARY_NAME)-linux$(NC)"

## version: Показати версію Go
version:
@$(GO) version

## env-example: Створити приклад .env файлу
env-example:
@echo "$(BLUE)📝 Creating .env.example...$(NC)"
@cat > .env.example << EOF
# Server Configuration
PORT=8080
HOST=0.0.0.0
ENV=development

# MongoDB Configuration
MONGODB_URI=mongodb://localhost:27017/nova_kakhovka_ecity
DATABASE_NAME=nova_kakhovka_ecity

# JWT Configuration
JWT_SECRET=your-super-secret-jwt-key-minimum-32-characters
JWT_EXPIRY=24h
REFRESH_TOKEN_EXPIRY=168h

# CORS Configuration
ALLOWED_ORIGINS=http://localhost:3000,http://localhost:3001

# Rate Limiting
RATE_LIMIT_ENABLED=true
RATE_LIMIT_REQUESTS=100
RATE_LIMIT_DURATION=1m

# Firebase (optional)
FIREBASE_CREDENTIALS_PATH=

# SMTP (optional)
SMTP_HOST=
SMTP_PORT=
SMTP_USER=
SMTP_PASSWORD=

# AWS S3 (optional)
AWS_REGION=
AWS_ACCESS_KEY_ID=
AWS_SECRET_ACCESS_KEY=
S3_BUCKET_NAME=
EOF
@echo "$(GREEN)✅ .env.example created$(NC)"

.DEFAULT_GOAL := help