// cmd/server/main.go - Nova Kakhovka e-City Backend Server
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	// Внутренние пакеты проекта
	"nova-kakhovka-ecity/internal/config"
	"nova-kakhovka-ecity/internal/database"
	"nova-kakhovka-ecity/internal/handlers"
	"nova-kakhovka-ecity/internal/middleware"
	"nova-kakhovka-ecity/internal/services"
	"nova-kakhovka-ecity/internal/websocket"
	"nova-kakhovka-ecity/pkg/auth"
	"nova-kakhovka-ecity/pkg/validator"

	// Внешние зависимости
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/mongo"
)

var (
	// Глобальная переменная для отслеживания времени запуска сервера
	serverStartTime = time.Now()

	// Версия приложения
	appVersion = "1.0.0"
	buildTime  = "unknown"
	gitCommit  = "unknown"
)

func main() {
	// Загружаем .env файл в режиме разработки
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found, using environment variables")
	}

	// Загружаем конфигурацию
	cfg := config.Load()

	// Настраиваем логирование
	setupLogging(cfg)

	// Выводим информацию о запуске
	printStartupInfo(cfg)

	// Подключаемся к MongoDB
	log.Println("🔌 Connecting to MongoDB...")
	db, err := database.NewMongoDB(cfg)
	if err != nil {
		log.Fatalf("❌ Failed to connect to MongoDB: %v", err)
	}
	defer func() {
		// Graceful отключение от БД при завершении
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := db.Client.Disconnect(ctx); err != nil {
			log.Printf("⚠️  Error disconnecting from MongoDB: %v", err)
		} else {
			log.Println("✅ Disconnected from MongoDB")
		}
	}()

	// Создаем индексы в MongoDB
	if err := createDatabaseIndexes(db); err != nil {
		log.Printf("⚠️  Warning: Failed to create some indexes: %v", err)
	}

	// Инициализируем валидатор
	validator.Init()

	// Инициализируем JWT менеджер
	jwtManager := auth.NewJWTManager(
		cfg.JWTSecret,
		time.Duration(cfg.JWTExpiration)*time.Hour,
		time.Duration(cfg.RefreshTokenExpiration)*time.Hour,
	)

	// Получаем коллекции MongoDB
	collections := getCollections(db.Database)

	// Инициализируем сервисы
	services := initializeServices(cfg, collections, jwtManager)

	// Инициализируем WebSocket Hub для real-time функций
	wsHub := websocket.NewHub()
	go wsHub.Run()
	defer wsHub.Shutdown()

	// Инициализируем хендлеры
	handlers := initializeHandlers(collections, services, jwtManager, wsHub)

	// Создаем и настраиваем роутер
	router := setupRouter(cfg, handlers, jwtManager, wsHub)

	// Создаем HTTP сервер
	srv := &http.Server{
		Addr:           fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Handler:        router,
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   15 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	// Запускаем сервер в горутине
	go func() {
		log.Printf("🚀 Nova Kakhovka e-City Backend Server v%s starting...", appVersion)
		log.Printf("🌐 Server running on http://%s:%s", cfg.Host, cfg.Port)
		log.Printf("📡 WebSocket endpoint: ws://%s:%s/ws", cfg.Host, cfg.Port)

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ Server failed to start: %v", err)
		}
	}()

	// Ожидаем сигнал завершения
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 Shutting down server...")

	// Graceful shutdown с таймаутом
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Отправляем уведомление всем WebSocket клиентам о завершении
	wsHub.Broadcast(websocket.Message{
		Type: "system",
		Data: map[string]interface{}{
			"message": "Server is shutting down",
		},
	})

	// Ждем немного, чтобы сообщения дошли
	time.Sleep(1 * time.Second)

	// Зупиняем HTTP сервер
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("⚠️  Server forced to shutdown: %v", err)
	} else {
		log.Println("✅ Server gracefully stopped")
	}

	log.Println("👋 Nova Kakhovka e-City Backend exited")
}

// setupLogging настраивает логирование в зависимости от окружения
func setupLogging(cfg *config.Config) {
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
		// В production можем настроить вывод в файл
		// logFile, _ := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		// log.SetOutput(logFile)
	} else {
		gin.SetMode(gin.DebugMode)
		// Добавляем время к логам в development
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}
}

// printStartupInfo выводит информацию о запуске сервера
func printStartupInfo(cfg *config.Config) {
	log.Println("================================================================================")
	log.Printf("🏙️  Nova Kakhovka e-City Backend Server")
	log.Printf("📌 Version: %s | Build: %s | Commit: %s", appVersion, buildTime, gitCommit)
	log.Printf("🌍 Environment: %s", cfg.Environment)
	log.Printf("🔧 Configuration:")
	log.Printf("   • Host: %s", cfg.Host)
	log.Printf("   • Port: %s", cfg.Port)
	log.Printf("   • Database: %s", cfg.DatabaseName)
	log.Printf("   • CORS Origins: %v", cfg.AllowedOrigins)
	if cfg.RateLimitEnabled {
		log.Printf("   • Rate Limit: %d requests per %s", cfg.RateLimitRequests, cfg.RateLimitDuration)
	}
	log.Println("================================================================================")
}

// getCollections возвращает все коллекции MongoDB
func getCollections(db *mongo.Database) map[string]*mongo.Collection {
	return map[string]*mongo.Collection{
		"users":             db.Collection("users"),
		"groups":            db.Collection("groups"),
		"messages":          db.Collection("messages"),
		"announcements":     db.Collection("announcements"),
		"events":            db.Collection("events"),
		"petitions":         db.Collection("petitions"),
		"polls":             db.Collection("polls"),
		"city_issues":       db.Collection("city_issues"),
		"notifications":     db.Collection("notifications"),
		"device_tokens":     db.Collection("device_tokens"),
		"transport_routes":  db.Collection("transport_routes"),
		"transport_vehicles": db.Collection("transport_vehicles"),
		"transport_stops":   db.Collection("transport_stops"),
		"audit_logs":        db.Collection("audit_logs"),
	}
}

// initializeServices инициализирует все сервисы
func initializeServices(
	cfg *config.Config,
	collections map[string]*mongo.Collection,
	jwtManager *auth.JWTManager,
) map[string]interface{} {
	// Базовые сервисы
	authService := services.NewAuthService(
		collections["users"],
		jwtManager,
		cfg,
	)

	userService := services.NewUserService(
		collections["users"],
	)

	// Сервис уведомлений (используется другими сервисами)
	notificationService := services.NewNotificationService(
		cfg,
		collections["users"],
		collections["notifications"],
		collections["device_tokens"],
	)

	// Основные сервисы функционала
	groupService := services.NewGroupService(
		collections["groups"],
		collections["messages"],
		collections["users"],
		notificationService,
	)

	announcementService := services.NewAnnouncementService(
		collections["announcements"],
		collections["users"],
		notificationService,
	)

	eventService := services.NewEventService(
		collections["events"],
		collections["users"],
		notificationService,
	)

	petitionService := services.NewPetitionService(
		collections["petitions"],
		collections["users"],
		notificationService,
	)

	pollService := services.NewPollService(
		collections["polls"],
		collections["users"],
		notificationService,
	)

	cityIssueService := services.NewCityIssueService(
		collections["city_issues"],
		collections["users"],
		notificationService,
	)

	transportService := services.NewTransportService(
		collections["transport_routes"],
		collections["transport_vehicles"],
		collections["transport_stops"],
		notificationService,
	)

	// Дополнительные сервисы
	emailService := services.NewEmailService(cfg)
	fileService := services.NewFileService(cfg)
	auditService := services.NewAuditService(collections["audit_logs"])
	statsService := services.NewStatsService(collections)

	return map[string]interface{}{
		"auth":         authService,
		"user":         userService,
		"notification": notificationService,
		"group":        groupService,
		"announcement": announcementService,
		"event":        eventService,
		"petition":     petitionService,
		"poll":         pollService,
		"cityIssue":    cityIssueService,
		"transport":    transportService,
		"email":        emailService,
		"file":         fileService,
		"audit":        auditService,
		"stats":        statsService,
	}
}

// initializeHandlers инициализирует все хендлеры
func initializeHandlers(
	collections map[string]*mongo.Collection,
	services map[string]interface{},
	jwtManager *auth.JWTManager,
	wsHub *websocket.Hub,
) map[string]interface{} {
	return map[string]interface{}{
		"auth": handlers.NewAuthHandler(
			services["auth"].(*services.AuthService),
			services["user"].(*services.UserService),
			services["audit"].(*services.AuditService),
		),
		"user": handlers.NewUserHandler(
			services["user"].(*services.UserService),
			services["file"].(*services.FileService),
		),
		"group": handlers.NewGroupHandler(
			collections["groups"],
			collections["users"],
			collections["messages"],
			wsHub,
		),
		"announcement": handlers.NewAnnouncementHandler(
			collections["announcements"],
			services["notification"].(*services.NotificationService),
		),
		"event": handlers.NewEventHandler(
			collections["events"],
			collections["users"],
			services["notification"].(*services.NotificationService),
		),
		"petition": handlers.NewPetitionHandler(
			collections["petitions"],
			collections["users"],
			services["notification"].(*services.NotificationService),
		),
		"poll": handlers.NewPollHandler(
			collections["polls"],
			collections["users"],
			services["notification"].(*services.NotificationService),
		),
		"cityIssue": handlers.NewCityIssueHandler(
			collections["city_issues"],
			collections["users"],
			services["notification"].(*services.NotificationService),
		),
		"transport": handlers.NewTransportHandler(
			services["transport"].(*services.TransportService),
		),
		"notification": handlers.NewNotificationHandler(
			services["notification"].(*services.NotificationService),
		),
		"websocket": handlers.NewWebSocketHandler(
			jwtManager,
			collections["groups"],
			collections["messages"],
			wsHub,
		),
		"stats": handlers.NewStatsHandler(
			services["stats"].(*services.StatsService),
		),
		"audit": handlers.NewAuditHandler(
			services["audit"].(*services.AuditService),
		),
		"file": handlers.NewFileHandler(
			services["file"].(*services.FileService),
		),
	}
}

// setupRouter настраивает все маршруты
func setupRouter(
	cfg *config.Config,
	handlers map[string]interface{},
	jwtManager *auth.JWTManager,
	wsHub *websocket.Hub,
) *gin.Engine {
	router := gin.New()

	// Глобальные middleware
	router.Use(gin.Recovery())
	router.Use(middleware.RequestID())
	router.Use(middleware.Logger())

	// CORS настройки для поддержки frontend
	corsConfig := cors.Config{
		AllowOrigins:     cfg.AllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Request-ID", "X-Device-ID"},
		ExposeHeaders:    []string{"Content-Length", "X-Total-Count", "X-Request-ID"},
		AllowCredentials: true,
		MaxAge:          12 * time.Hour,
	}
	router.Use(cors.New(corsConfig))

	// Rate limiting (опционально)
	if cfg.RateLimitEnabled {
		router.Use(middleware.RateLimit(cfg.RateLimitRequests, cfg.RateLimitDuration))
	}

	// Security headers
	router.Use(middleware.SecurityHeaders())

	// Request size limit
	router.Use(middleware.RequestSizeLimit(10 << 20)) // 10 MB

	// Статические файлы (для uploaded content)
	router.Static("/uploads", "./uploads")
	router.Static("/public", "./public")

	// WebSocket endpoint - должен быть до других маршрутов
	router.GET("/ws", handlers["websocket"].(*handlers.WebSocketHandler).HandleWebSocket)

	// Health check и метаданные
	setupHealthRoutes(router, wsHub)

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Публичные маршруты (без авторизации)
		setupPublicRoutes(v1, handlers)

		// Защищенные маршруты (требуют JWT)
		setupProtectedRoutes(v1, handlers, jwtManager, cfg.JWTSecret)

		// Админские маршруты (требуют роль moderator/admin)
		setupAdminRoutes(v1, handlers, jwtManager, cfg.JWTSecret)
	}

	// Swagger документация в development режиме
	if cfg.Environment == "development" {
		router.Static("/api/docs", "./docs/swagger")
		router.GET("/api/docs/", func(c *gin.Context) {
			c.File("./docs/swagger/index.html")
		})
	}

	// 404 handler для неизвестных маршрутов
	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Endpoint not found",
			"path":  c.Request.URL.Path,
		})
	})

	// 405 handler для неподдерживаемых методов
	router.NoMethod(func(c *gin.Context) {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"error":  "Method not allowed",
			"method": c.Request.Method,
			"path":   c.Request.URL.Path,
		})
	})

	return router
}

// setupHealthRoutes настраивает маршруты health check и информации о сервере
func setupHealthRoutes(router *gin.Engine, wsHub *websocket.Hub) {
	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"timestamp": time.Now().Format(time.RFC3339),
			"uptime":    time.Since(serverStartTime).String(),
			"version":   appVersion,
			"build": gin.H{
				"time":   buildTime,
				"commit": gitCommit,
			},
			"stats": gin.H{
				"websocket_connections": wsHub.GetConnectionsCount(),
				"active_groups":        wsHub.GetActiveGroupsCount(),
			},
		})
	})

	// Readiness check для Kubernetes
	router.GET("/ready", func(c *gin.Context) {
		// Здесь можно добавить проверки готовности сервисов
		c.JSON(http.StatusOK, gin.H{"ready": true})
	})

	// Liveness check для Kubernetes
	router.GET("/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"alive": true})
	})

	// API информация
	router.GET("/api", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"name":        "Nova Kakhovka e-City API",
			"version":     appVersion,
			"description": "Backend API for Nova Kakhovka e-City platform",
			"documentation": gin.H{
				"swagger": "/api/docs",
				"postman": "https://documenter.getpostman.com/view/...",
			},
			"endpoints": gin.H{
				"websocket":     "/ws",
				"public_api":    "/api/v1/*",
				"protected_api": "/api/v1/* (requires JWT)",
				"admin_api":     "/api/v1/admin/* (requires moderator/admin role)",
				"health":        "/health",
			},
			"features": []string{
				"User Authentication & Authorization (JWT)",
				"Real-time Group Chat (WebSocket)",
				"City Announcements Board",
				"Events Calendar & Registration",
				"Electronic Petitions System",
				"Citizen Polls & Voting",
				"City Issues Reporting & Tracking",
				"Public Transport Live Tracking",
				"Push Notifications (FCM)",
				"File Upload & Storage",
				"Email Notifications",
				"Admin Dashboard API",
				"Analytics & Statistics",
				"Audit Logging",
				"Rate Limiting",
				"CORS Support",
			},
			"contact": gin.H{
				"email":   "dev@nova-kakhovka-ecity.gov.ua",
				"support": "https://support.nova-kakhovka-ecity.gov.ua",
			},
		})
	})
}

// setupPublicRoutes настраивает публичные маршруты
func setupPublicRoutes(v1 *gin.RouterGroup, handlers map[string]interface{}) {
	authHandler := handlers["auth"].(*handlers.AuthHandler)
	announcementHandler := handlers["announcement"].(*handlers.AnnouncementHandler)
	eventHandler := handlers["event"].(*handlers.EventHandler)
	petitionHandler := handlers["petition"].(*handlers.PetitionHandler)
	pollHandler := handlers["poll"].(*handlers.PollHandler)
	transportHandler := handlers["transport"].(*handlers.TransportHandler)

	// Авторизация и регистрация
	v1.POST("/auth/register", authHandler.Register)
	v1.POST("/auth/login", authHandler.Login)
	v1.POST("/auth/refresh", authHandler.RefreshToken)
	v1.POST("/auth/forgot-password", authHandler.ForgotPassword)
	v1.POST("/auth/reset-password", authHandler.ResetPassword)
	v1.POST("/auth/verify-email", authHandler.VerifyEmail)

	// Публичный контент
	v1.GET("/announcements", announcementHandler.GetAnnouncements)
	v1.GET("/announcements/:id", announcementHandler.GetAnnouncement)
	v1.GET("/events", eventHandler.GetEvents)
	v1.GET("/events/:id", eventHandler.GetEvent)
	v1.GET("/events/upcoming", eventHandler.GetUpcomingEvents)
	v1.GET("/petitions", petitionHandler.GetPetitions)
	v1.GET("/petitions/:id", petitionHandler.GetPetition)
	v1.GET("/petitions/popular", petitionHandler.GetPopularPetitions)
	v1.GET("/polls", pollHandler.GetPolls)
	v1.GET("/polls/:id", pollHandler.GetPoll)
	v1.GET("/polls/:id/results", pollHandler.GetPollResults)

	// Транспорт (публичная информация)
	v1.GET("/transport/routes", transportHandler.GetRoutes)
	v1.GET("/transport/routes/:id", transportHandler.GetRoute)
	v1.GET("/transport/stops", transportHandler.GetStops)
	v1.GET("/transport/stops/:id", transportHandler.GetStop)
	v1.GET("/transport/vehicles/live", transportHandler.GetLiveVehicles)
	v1.GET("/transport/schedule/:route_id", transportHandler.GetSchedule)
}

// setupProtectedRoutes настраивает защищенные маршруты
func setupProtectedRoutes(v1 *gin.RouterGroup, handlers map[string]interface{}, jwtManager *auth.JWTManager, jwtSecret string) {
	protected := v1.Group("")
	protected.Use(middleware.AuthMiddleware(jwtSecret))

	userHandler := handlers["user"].(*handlers.UserHandler)
	groupHandler := handlers["group"].(*handlers.GroupHandler)
	announcementHandler := handlers["announcement"].(*handlers.AnnouncementHandler)
	eventHandler := handlers["event"].(*handlers.EventHandler)
	petitionHandler := handlers["petition"].(*handlers.PetitionHandler)
	pollHandler := handlers["poll"].(*handlers.PollHandler)
	cityIssueHandler := handlers["cityIssue"].(*handlers.CityIssueHandler)
	notificationHandler := handlers["notification"].(*handlers.NotificationHandler)
	fileHandler := handlers["file"].(*handlers.FileHandler)

	// Профиль пользователя
	protected.GET("/users/me", userHandler.GetProfile)
	protected.PUT("/users/me", userHandler.UpdateProfile)
	protected.DELETE("/users/me", userHandler.DeleteAccount)
	protected.POST("/users/me/avatar", userHandler.UploadAvatar)
	protected.PUT("/users/me/password", userHandler.ChangePassword)
	protected.GET("/users/me/groups", userHandler.GetMyGroups)
	protected.GET("/users/me/events", userHandler.GetMyEvents)
	protected.GET("/users/me/petitions", userHandler.GetMyPetitions)
	protected.GET("/users/me/issues", userHandler.GetMyIssues)

	// Группы и чаты
	protected.GET("/groups", groupHandler.GetGroups)
	protected.GET("/groups/:id", groupHandler.GetGroup)
	protected.POST("/groups", groupHandler.CreateGroup)
	protected.PUT("/groups/:id", groupHandler.UpdateGroup)
	protected.DELETE("/groups/:id", groupHandler.DeleteGroup)
	protected.POST("/groups/:id/join", groupHandler.JoinGroup)
	protected.POST("/groups/:id/leave", groupHandler.LeaveGroup)
	protected.GET("/groups/:id/members", groupHandler.GetMembers)
	protected.PUT("/groups/:id/members/:user_id/role", groupHandler.UpdateMemberRole)
	protected.DELETE("/groups/:id/members/:user_id", groupHandler.RemoveMember)

	// Сообщения в группах
	protected.GET("/groups/:id/messages", groupHandler.GetMessages)
	protected.POST("/groups/:id/messages", groupHandler.SendMessage)
	protected.PUT("/messages/:id", groupHandler.EditMessage)
	protected.DELETE("/messages/:id", groupHandler.DeleteMessage)
	protected.POST("/messages/:id/reactions", groupHandler.AddReaction)
	protected.DELETE("/messages/:id/reactions/:reaction", groupHandler.RemoveReaction)

	// Объявления
	protected.POST("/announcements", announcementHandler.CreateAnnouncement)
	protected.PUT("/announcements/:id", announcementHandler.UpdateAnnouncement)
	protected.DELETE("/announcements/:id", announcementHandler.DeleteAnnouncement)
	protected.POST("/announcements/:id/report", announcementHandler.ReportAnnouncement)

	// События
	protected.POST("/events", eventHandler.CreateEvent)
	protected.PUT("/events/:id", eventHandler.UpdateEvent)
	protected.DELETE("/events/:id", eventHandler.DeleteEvent)
	protected.POST("/events/:id/register", eventHandler.RegisterForEvent)
	protected.DELETE("/events/:id/register", eventHandler.UnregisterFromEvent)
	protected.GET("/events/:id/participants", eventHandler.GetParticipants)
	protected.POST("/events/:id/comments", eventHandler.AddComment)

	// Петиции
	protected.POST("/petitions", petitionHandler.CreatePetition)
	protected.PUT("/petitions/:id", petitionHandler.UpdatePetition)
	protected.DELETE("/petitions/:id", petitionHandler.DeletePetition)
	protected.POST("/petitions/:id/sign", petitionHandler.SignPetition)
	protected.DELETE("/petitions/:id/sign", petitionHandler.UnsignPetition)
	protected.GET("/petitions/:id/signatures", petitionHandler.GetSignatures)
	protected.POST("/petitions/:id/comments", petitionHandler.AddComment)

	// Опросы
	protected.POST("/polls/:id/vote", pollHandler.VotePoll)
	protected.DELETE("/polls/:id/vote", pollHandler.RemoveVote)
	protected.GET("/polls/:id/my-vote", pollHandler.GetMyVote)

	// Проблемы города
	protected.GET("/issues", cityIssueHandler.GetIssues)
	protected.GET("/issues/:id", cityIssueHandler.GetIssue)
	protected.POST("/issues", cityIssueHandler.CreateIssue)
	protected.PUT("/issues/:id", cityIssueHandler.UpdateIssue)
	protected.DELETE("/issues/:id", cityIssueHandler.DeleteIssue)
	protected.POST("/issues/:id/vote", cityIssueHandler.VoteIssue)
	protected.DELETE("/issues/:id/vote", cityIssueHandler.UnvoteIssue)
	protected.POST("/issues/:id/comments", cityIssueHandler.AddComment)
	protected.GET("/issues/:id/comments", cityIssueHandler.GetComments)
	protected.POST("/issues/:id/photos", cityIssueHandler.UploadPhotos)

	// Уведомления
	protected.GET("/notifications", notificationHandler.GetNotifications)
	protected.GET("/notifications/unread-count", notificationHandler.GetUnreadCount)
	protected.PUT("/notifications/:id/read", notificationHandler.MarkAsRead)
	protected.PUT("/notifications/read-all", notificationHandler.MarkAllAsRead)
	protected.DELETE("/notifications/:id", notificationHandler.DeleteNotification)
	protected.POST("/notifications/register-device", notificationHandler.RegisterDevice)
	protected.DELETE("/notifications/unregister-device", notificationHandler.UnregisterDevice)
	protected.GET("/notifications/settings", notificationHandler.GetSettings)
	protected.PUT("/notifications/settings", notificationHandler.UpdateSettings)

	// Файлы
	protected.POST("/files/upload", fileHandler.UploadFile)
	protected.POST("/files/upload-multiple", fileHandler.UploadMultipleFiles)
	protected.DELETE("/files/:id", fileHandler.DeleteFile)
	protected.GET("/files/:id/info", fileHandler.GetFileInfo)
}

// setupAdminRoutes настраивает админские маршруты
func setupAdminRoutes(v1 *gin.RouterGroup, handlers map[string]interface{}, jwtManager *auth.JWTManager, jwtSecret string) {
	admin := v1.Group("/admin")
	admin.Use(middleware.AuthMiddleware(jwtSecret))
	admin.Use(middleware.RequireRole("moderator", "admin"))

	userHandler := handlers["user"].(*handlers.UserHandler)
	groupHandler := handlers["group"].(*handlers.GroupHandler)
	announcementHandler := handlers["announcement"].(*handlers.AnnouncementHandler)
	eventHandler := handlers["event"].(*handlers.EventHandler)
	petitionHandler := handlers["petition"].(*handlers.PetitionHandler)
	pollHandler := handlers["poll"].(*handlers.PollHandler)
	cityIssueHandler := handlers["cityIssue"].(*handlers.CityIssueHandler)
	transportHandler := handlers["transport"].(*handlers.TransportHandler)
	notificationHandler := handlers["notification"].(*handlers.NotificationHandler)
	statsHandler := handlers["stats"].(*handlers.StatsHandler)
	auditHandler := handlers["audit"].(*handlers.AuditHandler)