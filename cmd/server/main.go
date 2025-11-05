// cmd/server/main.go
// Nova Kakhovka e-City Platform - Main Server Entry Point
//
// Цей файл ініціалізує і запускає головний HTTP сервер з усіма залежностями:
// - MongoDB підключення та індекси
// - JWT авторизація
// - Всі handlers (auth, groups, events, announcements, users, тощо)
// - WebSocket для real-time чату
// - Background tasks (cleanup, scheduler)
// - CORS та Rate Limiting
// - Graceful shutdown

package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nova-kakhovka-ecity/internal/config"
	"nova-kakhovka-ecity/internal/database"
	"nova-kakhovka-ecity/internal/handlers"
	"nova-kakhovka-ecity/internal/middleware"
	"nova-kakhovka-ecity/internal/services"
	"nova-kakhovka-ecity/pkg/auth"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	log.Println("🚀 Starting Nova Kakhovka e-City Platform...")

	// ========================================
	// 1. КОНФІГУРАЦІЯ
	// ========================================
	cfg := config.Load()
	log.Printf("📋 Configuration loaded (Environment: %s)", cfg.Env)

	// ========================================
	// 2. ПІДКЛЮЧЕННЯ ДО MONGODB
	// ========================================
	log.Println("🔌 Connecting to MongoDB...")
	db, err := database.NewMongoDB(cfg)
	if err != nil {
		log.Fatalf("❌ Failed to connect to database: %v", err)
	}
	defer db.Close()
	log.Println("✅ MongoDB connected successfully")

	// Створення індексів для оптимізації запитів
	log.Println("📊 Creating database indexes...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := db.CreateIndexes(ctx); err != nil {
		log.Printf("⚠️  Warning: Failed to create indexes: %v", err)
	} else {
		log.Println("✅ Database indexes created")
	}

	// ========================================
	// 3. ІНІЦІАЛІЗАЦІЯ JWT МЕНЕДЖЕРА
	// ========================================
	log.Println("🔐 Initializing JWT manager...")
	jwtManager := auth.NewJWTManager(
		cfg.JWTSecret,
		time.Duration(cfg.JWTExpiration)*time.Hour,
	)
	log.Println("✅ JWT manager initialized")

	// ========================================
	// 4. ОТРИМАННЯ КОЛЕКЦІЙ MONGODB
	// ========================================
	userCollection := db.Database.Collection("users")
	groupCollection := db.Database.Collection("groups")
	messageCollection := db.Database.Collection("messages")
	announcementCollection := db.Database.Collection("announcements")
	eventCollection := db.Database.Collection("events")
	notificationCollection := db.Database.Collection("notifications")
	deviceTokenCollection := db.Database.Collection("device_tokens")
	cityIssueCollection := db.Database.Collection("city_issues")
	petitionCollection := db.Database.Collection("petitions")
	pollCollection := db.Database.Collection("polls")
	transportRouteCollection := db.Database.Collection("transport_routes")
	transportVehicleCollection := db.Database.Collection("transport_vehicles")

	// ========================================
	// 5. ІНІЦІАЛІЗАЦІЯ СЕРВІСІВ
	// ========================================
	log.Println("⚙️  Initializing services...")
	notificationService := services.NewNotificationService(
		cfg,
		userCollection,
		notificationCollection,
	)
	log.Println("✅ Services initialized")

	// ========================================
	// 6. ІНІЦІАЛІЗАЦІЯ HANDLERS
	// ========================================
	log.Println("🎯 Initializing handlers...")

	// Auth handler - авторизація та реєстрація
	authHandler := handlers.NewAuthHandler(userCollection, jwtManager)

	// Users handler - управління користувачами (ADMIN)
	usersHandler := handlers.NewUsersHandler(userCollection)

	// Group handler - групи та чати
	groupHandler := handlers.NewGroupHandler(
		groupCollection,
		userCollection,
		messageCollection,
	)

	// WebSocket handler - real-time чат
	wsHandler := handlers.NewWebSocketHandler(
		jwtManager,
		groupCollection,
		messageCollection,
	)

	// Announcement handler - оголошення
	announcementHandler := handlers.NewAnnouncementHandler(
		announcementCollection,
		userCollection,
	)

	// Event handler - події міста
	eventHandler := handlers.NewEventHandler(
		eventCollection,
		userCollection,
	)

	// Notification handler - сповіщення
	notificationHandler := handlers.NewNotificationHandler(
		notificationService,
		notificationCollection,
		deviceTokenCollection,
	)

	// City Issue handler - проблеми міста
	cityIssueHandler := handlers.NewCityIssueHandler(
		cityIssueCollection,
		userCollection,
		notificationService,
	)

	// Petition handler - петиції
	petitionHandler := handlers.NewPetitionHandler(
		petitionCollection,
		userCollection,
		notificationService,
	)

	// Poll handler - опитування
	pollHandler := handlers.NewPollHandler(
		pollCollection,
		userCollection,
		notificationService,
	)

	// Transport handler - громадський транспорт
	transportHandler := handlers.NewTransportHandler(
		transportRouteCollection,
		transportVehicleCollection,
		userCollection,
	)

	log.Println("✅ All handlers initialized")

	// ========================================
	// 7. ЗАПУСК ФОНОВИХ ЗАДАЧ
	// ========================================
	log.Println("🔄 Starting background tasks...")

	// WebSocket hub для управління з'єднаннями
	wsHandler.StartHub()

	// Cleanup старих опитувань
	pollHandler.StartPollCleanupScheduler()

	// Генерація розкладу транспорту
	transportHandler.StartScheduleGenerator()

	log.Println("✅ Background tasks started")

	// ========================================
	// 8. НАЛАШТУВАННЯ GIN ROUTER
	// ========================================
	// Встановлюємо режим роботи Gin
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
		log.Println("🏭 Running in PRODUCTION mode")
	} else {
		gin.SetMode(gin.DebugMode)
		log.Println("🔧 Running in DEVELOPMENT mode")
	}

	router := gin.New()

	// Middleware
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// ========================================
	// 9. НАЛАШТУВАННЯ CORS
	// ========================================
	log.Println("🌐 Configuring CORS...")
	corsConfig := cors.Config{
		AllowOrigins: []string{
			"http://localhost:3000",           // Web app (development)
			"http://localhost:3001",           // Admin app (development)
			"https://nova-kakhovka.com",       // Production web
			"https://admin.nova-kakhovka.com", // Production admin
		},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}

	// У development режимі дозволяємо всі origins
	if cfg.Env == "development" {
		corsConfig.AllowOrigins = []string{"*"}
		corsConfig.AllowOriginFunc = func(origin string) bool {
			return true
		}
	}

	router.Use(cors.New(corsConfig))
	log.Println("✅ CORS configured")

	// ========================================
	// 10. RATE LIMITING
	// ========================================
	log.Println("🛡️  Configuring rate limiting...")
	rateLimiter := middleware.NewRateLimiter(100, time.Hour) // 100 запитів на годину
	router.Use(rateLimiter.RateLimit())
	log.Println("✅ Rate limiting enabled")

	// ========================================
	// 11. API ROUTES
	// ========================================
	log.Println("🛣️  Setting up API routes...")

	// API v1 base group
	api := router.Group("/api/v1")

	// ========================================
	// 🔓 ПУБЛІЧНІ МАРШРУТИ (без автентифікації)
	// ========================================
	{
		// ===== АВТОРИЗАЦІЯ =====
		api.POST("/auth/register", authHandler.Register)
		api.POST("/auth/login", authHandler.Login)

		// ===== ПУБЛІЧНА ІНФОРМАЦІЯ =====
		// Групи
		api.GET("/groups/public", groupHandler.GetPublicGroups)

		// Оголошення
		api.GET("/announcements", announcementHandler.GetAnnouncements)
		api.GET("/announcements/:id", announcementHandler.GetAnnouncement)

		// Події
		api.GET("/events", eventHandler.GetEvents)
		api.GET("/events/:id", eventHandler.GetEvent)

		// Петиції
		api.GET("/petitions", petitionHandler.GetPetitions)
		api.GET("/petitions/:id", petitionHandler.GetPetition)

		// Опитування
		api.GET("/polls", pollHandler.GetPolls)
		api.GET("/polls/:id", pollHandler.GetPoll)

		// Проблеми міста
		api.GET("/city-issues", cityIssueHandler.GetIssues)
		api.GET("/city-issues/:id", cityIssueHandler.GetIssue)

		// Транспорт (публічна інформація)
		api.GET("/transport/routes", transportHandler.GetRoutes)
		api.GET("/transport/routes/:id", transportHandler.GetRoute)
		api.GET("/transport/stops/nearby", transportHandler.GetNearbyStops)
		api.GET("/transport/arrivals", transportHandler.GetArrivals)
		api.GET("/transport/live", transportHandler.GetLiveTracking)

		// Типи сповіщень
		api.GET("/notification-types", notificationHandler.GetNotificationTypes)
	}

	// ========================================
	// 🔒 ЗАХИЩЕНІ МАРШРУТИ (потрібна автентифікація)
	// ========================================
	protected := api.Group("")
	protected.Use(middleware.AuthMiddleware(jwtManager))
	{
		// ===== ПРОФІЛЬ КОРИСТУВАЧА =====
		protected.GET("/auth/profile", authHandler.GetProfile)
		protected.PUT("/auth/profile", authHandler.UpdateProfile)

		// ===== ГРУПИ ТА ЧАТИ =====
		protected.POST("/groups", groupHandler.CreateGroup)
		protected.GET("/groups", groupHandler.GetUserGroups) // ✅ ВИПРАВЛЕНО: використовуємо GetUserGroups
		protected.POST("/groups/:id/join", groupHandler.JoinGroup)
		// protected.POST("/groups/:id/leave", groupHandler.LeaveGroup) // TODO: Реалізувати метод

		// Повідомлення
		protected.GET("/groups/:id/messages", groupHandler.GetMessages) // ✅ ВИПРАВЛЕНО: використовуємо :id замість :group_id
		protected.POST("/groups/:id/messages", groupHandler.SendMessage)

		// ===== ОГОЛОШЕННЯ =====
		protected.POST("/announcements", announcementHandler.CreateAnnouncement)
		protected.GET("/announcements/my", announcementHandler.GetUserAnnouncements)
		protected.PUT("/announcements/:id", announcementHandler.UpdateAnnouncement)
		protected.DELETE("/announcements/:id", announcementHandler.DeleteAnnouncement)
		protected.POST("/announcements/:id/contact", announcementHandler.ContactOwner)

		// ===== ПОДІЇ =====
		protected.POST("/events", eventHandler.CreateEvent)
		protected.GET("/events/my", eventHandler.GetUserEvents)
		protected.PUT("/events/:id", eventHandler.UpdateEvent)
		protected.DELETE("/events/:id", eventHandler.DeleteEvent)
		protected.POST("/events/:id/join", eventHandler.JoinEvent)
		protected.POST("/events/:id/leave", eventHandler.LeaveEvent)
		protected.GET("/events/:id/participants", eventHandler.GetEventParticipants)

		// ===== ПЕТИЦІЇ =====
		protected.POST("/petitions", petitionHandler.CreatePetition)
		protected.GET("/petitions/my", petitionHandler.GetUserPetitions)
		protected.POST("/petitions/:id/sign", petitionHandler.SignPetition)

		// ===== ОПИТУВАННЯ =====
		protected.POST("/polls", pollHandler.CreatePoll)
		protected.GET("/polls/my", pollHandler.GetUserPolls)
		protected.POST("/polls/:id/vote", pollHandler.VotePoll)
		protected.GET("/polls/:id/results", pollHandler.GetPollResults)

		// ===== ПРОБЛЕМИ МІСТА =====
		protected.POST("/city-issues", cityIssueHandler.CreateIssue)
		protected.GET("/city-issues/my", cityIssueHandler.GetUserIssues)
		protected.POST("/city-issues/:id/support", cityIssueHandler.SupportIssue)
		protected.POST("/city-issues/:id/comment", cityIssueHandler.AddComment)

		// ===== СПОВІЩЕННЯ =====
		protected.POST("/notifications/device-token", notificationHandler.RegisterDeviceToken)
		protected.GET("/notifications", notificationHandler.GetUserNotifications)
		protected.PUT("/notifications/:id/read", notificationHandler.MarkAsRead)
		protected.PUT("/notifications/read-all", notificationHandler.MarkAllAsRead)
	}

	// ========================================
	// 🔒 USERS MANAGEMENT API (ADMIN/MODERATOR)
	// ========================================
	// 🎯 Відповідає Frontend: apps/admin/app/(dashboard)/users/*
	usersGroup := api.Group("/users")
	usersGroup.Use(middleware.AuthMiddleware(jwtManager))
	{
		// GET /api/v1/users - Отримати список користувачів з фільтрами
		// 🔒 Права: Permission.USERS_MANAGE або Permission.MANAGE_USERS
		// 📊 Frontend: UsersManagementClient.tsx -> fetchUsers()
		usersGroup.GET("",
			middleware.RequirePermission("users:manage"),
			usersHandler.GetAllUsers,
		)

		// GET /api/v1/users/stats - Отримати статистику користувачів
		// 🔒 Права: Permission.VIEW_ANALYTICS або Permission.USERS_MANAGE
		// 📊 Frontend: UsersManagementClient.tsx -> fetchStats()
		usersGroup.GET("/stats",
			middleware.RequirePermission("users:manage"),
			usersHandler.GetUserStats,
		)

		// GET /api/v1/users/:id - Отримати користувача за ID
		// 🔒 Права: Permission.USERS_MANAGE або Permission.MANAGE_USERS
		// 📊 Frontend: UsersApi.getById()
		usersGroup.GET("/:id",
			middleware.RequirePermission("users:manage"),
			usersHandler.GetUserByID,
		)

		// PUT /api/v1/users/:id/password - Змінити пароль користувача
		// 🔒 Права: Permission.MANAGE_USERS (тільки ADMIN+)
		// 📊 Frontend: UsersManagementClient.tsx -> handleChangePassword()
		usersGroup.PUT("/:id/password",
			middleware.RequireRole("ADMIN"),
			usersHandler.UpdateUserPassword,
		)

		// PUT /api/v1/users/:id/block - Заблокувати/розблокувати користувача
		// 🔒 Права: Permission.BLOCK_USER (ADMIN+)
		// 📊 Frontend: UsersManagementClient.tsx -> handleToggleBlock()
		usersGroup.PUT("/:id/block",
			middleware.RequireRole("ADMIN"),
			usersHandler.BlockUser,
		)
	}

	// ========================================
	// 🔒 МОДЕРАТОРСЬКІ МАРШРУТИ
	// ========================================
	moderator := api.Group("")
	moderator.Use(middleware.AuthMiddleware(jwtManager))
	moderator.Use(middleware.RequireRole("MODERATOR"))
	{
		// Модерація оголошень
		moderator.PUT("/announcements/:id/approve", announcementHandler.ApproveAnnouncement)
		moderator.PUT("/announcements/:id/reject", announcementHandler.RejectAnnouncement)

		// Управління подіями
		moderator.PUT("/events/:id", eventHandler.UpdateEvent)
		moderator.DELETE("/events/:id", eventHandler.DeleteEvent)

		// Управління проблемами міста
		moderator.PUT("/city-issues/:id/status", cityIssueHandler.UpdateIssueStatus)
	}

	// ========================================
	// 🔒 АДМІНІСТРАТОРСЬКІ МАРШРУТИ
	// ========================================
	admin := api.Group("")
	admin.Use(middleware.AuthMiddleware(jwtManager))
	admin.Use(middleware.RequireRole("ADMIN"))
	{
		// Відправка сповіщень користувачам
		admin.POST("/notifications/send", notificationHandler.SendNotification)

		// Екстрені сповіщення (всім користувачам)
		admin.POST("/notifications/emergency", notificationHandler.SendEmergencyNotification)

		// Управління транспортом
		admin.POST("/transport/routes", transportHandler.CreateRoute)
		admin.PUT("/transport/routes/:id", transportHandler.UpdateRoute)
		admin.DELETE("/transport/routes/:id", transportHandler.DeleteRoute)

		admin.POST("/transport/vehicles", transportHandler.CreateVehicle)
		admin.PUT("/transport/vehicles/:id", transportHandler.UpdateVehicle)
		admin.DELETE("/transport/vehicles/:id", transportHandler.DeleteVehicle)
	}

	// ========================================
	// 🔌 WEBSOCKET МАРШРУТ
	// ========================================
	// WebSocket endpoint для real-time чату
	// ws://localhost:8080/ws
	router.GET("/ws", wsHandler.HandleWebSocket)

	log.Println("✅ All routes configured")

	// ========================================
	// 12. ЗАПУСК HTTP СЕРВЕРА
	// ========================================
	port := cfg.Port
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:           ":" + port,
		Handler:        router,
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   15 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	// Запускаємо сервер в окремій горутині
	go func() {
		log.Printf("🌍 Server starting on http://localhost:%s", port)
		log.Printf("📡 WebSocket available on ws://localhost:%s/ws", port)
		log.Println("✨ Nova Kakhovka e-City Platform is ready!")
		log.Println("")
		log.Println("Available endpoints:")
		log.Println("  🔓 Public:    http://localhost:" + port + "/api/v1")
		log.Println("  🔒 Protected: http://localhost:" + port + "/api/v1 (requires JWT)")
		log.Println("  👥 Users:     http://localhost:" + port + "/api/v1/users (ADMIN)")
		log.Println("  🔌 WebSocket: ws://localhost:" + port + "/ws")
		log.Println("")
		log.Println("Press Ctrl+C to stop the server")
		log.Println("─────────────────────────────────────────────────────")

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ Server failed to start: %v", err)
		}
	}()

	// ========================================
	// 13. GRACEFUL SHUTDOWN
	// ========================================
	// Чекаємо на сигнал завершення (Ctrl+C або kill)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("")
	log.Println("🛑 Shutting down server...")

	// Даємо 5 секунд на graceful shutdown
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("⚠️  Server forced to shutdown: %v", err)
	}

	log.Println("✅ Server stopped gracefully")
	log.Println("👋 Goodbye!")
}
