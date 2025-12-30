// cmd/server/main.go
// Nova Kakhovka e-City Platform - Main Server Entry Point
//
// Цей файл ініціалізує і запускає головний HTTP сервер з усіма залежностями:
// - MongoDB підключення та індекси
// - JWT авторизація
// - Всі handlers (auth, groups, events, announcements, users, polls, тощо)
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

	// ✅ Poll handler - опитування (ВИПРАВЛЕНО)
	pollHandler := handlers.NewPollHandler(
		db.Database, // Передаємо весь database для доступу до колекції
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
	go wsHandler.StartHub()

	// ✅ Cleanup старих опитувань (90+ днів)
	go handlers.StartPollCleanupTask(pollCollection)
	log.Println("✅ Poll cleanup task started")

	// Генерація розкладу транспорту (якщо є відповідний метод)
	// go transportHandler.StartScheduleGenerator()

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

	// ========================================
	// 9. MIDDLEWARE
	// ========================================
	// Базові middleware
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// ========================================
	// 10. CORS CONFIGURATION
	// ========================================
	log.Println("🌐 Configuring CORS...")
	corsConfig := cors.Config{
		AllowOrigins: []string{
			"http://localhost:3000",      // Next.js web app
			"http://localhost:3001",      // Next.js admin app
			"https://ecity.gov.ua",       // Production web
			"https://admin.ecity.gov.ua", // Production admin
		},
		AllowMethods: []string{
			"GET",
			"POST",
			"PUT",
			"PATCH",
			"DELETE",
			"OPTIONS",
		},
		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Accept",
			"Authorization",
			"X-Requested-With",
		},
		ExposeHeaders: []string{
			"Content-Length",
			"Content-Type",
		},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}
	router.Use(cors.New(corsConfig))
	log.Println("✅ CORS configured")

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

		// Опитування (публічні)
		api.GET("/polls", pollHandler.GetAllPolls)
		api.GET("/polls/:id", pollHandler.GetPoll)
		api.GET("/polls/:id/results", pollHandler.GetPollResults)

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
		protected.PUT("/auth/password", authHandler.ChangePassword)

		// ===== ГРУПИ ТА ЧАТИ =====
		protected.POST("/groups", groupHandler.CreateGroup)
		protected.GET("/groups", groupHandler.GetUserGroups)
		protected.GET("/groups/:id", groupHandler.GetGroup)
		protected.PUT("/groups/:id", groupHandler.UpdateGroup)
		protected.DELETE("/groups/:id", groupHandler.DeleteGroup)
		protected.POST("/groups/:id/join", groupHandler.JoinGroup)
		protected.POST("/groups/:id/leave", groupHandler.LeaveGroup)

		// Повідомлення в групах
		protected.POST("/groups/:id/messages", groupHandler.SendMessage)
		protected.GET("/groups/:id/messages", groupHandler.GetMessages)

		// ===== ОГОЛОШЕННЯ =====
		protected.POST("/announcements", announcementHandler.CreateAnnouncement)
		protected.PUT("/announcements/:id", announcementHandler.UpdateAnnouncement)
		protected.DELETE("/announcements/:id", announcementHandler.DeleteAnnouncement)

		// ===== ПОДІЇ =====
		protected.POST("/events", eventHandler.CreateEvent)
		protected.PUT("/events/:id", eventHandler.UpdateEvent)
		protected.DELETE("/events/:id", eventHandler.DeleteEvent)
		protected.POST("/events/:id/attend", eventHandler.AttendEvent)

		// ===== ПЕТИЦІЇ =====
		protected.POST("/petitions", petitionHandler.CreatePetition)
		protected.POST("/petitions/:id/sign", petitionHandler.SignPetition)
		protected.PUT("/petitions/:id", petitionHandler.UpdatePetition)

		// ===== ОПИТУВАННЯ =====
		// ✅ Створення опитування з rate limiting (5 хвилин між створенням)
		protected.POST("/polls", middleware.RateLimitMiddleware(), pollHandler.CreatePoll)

		// Голосування в опитуваннях
		protected.POST("/polls/:id/respond", pollHandler.VotePoll)

		// Редагування/видалення (тільки автор або модератор)
		protected.PUT("/polls/:id", pollHandler.UpdatePoll)
		protected.DELETE("/polls/:id", pollHandler.DeletePoll)

		// ===== ПРОБЛЕМИ МІСТА =====
		protected.POST("/city-issues", cityIssueHandler.CreateIssue)
		protected.PUT("/city-issues/:id", cityIssueHandler.UpdateIssue)
		protected.POST("/city-issues/:id/upvote", cityIssueHandler.UpvoteIssue)

		// ===== СПОВІЩЕННЯ =====
		protected.GET("/notifications", notificationHandler.GetNotifications)
		protected.PUT("/notifications/:id/read", notificationHandler.MarkAsRead)
		protected.PUT("/notifications/read-all", notificationHandler.MarkAllAsRead)
		protected.DELETE("/notifications/:id", notificationHandler.DeleteNotification)

		// Реєстрація device token для push-сповіщень
		protected.POST("/device-tokens", notificationHandler.RegisterDeviceToken)
		protected.DELETE("/device-tokens/:token", notificationHandler.UnregisterDeviceToken)

		// Налаштування сповіщень
		protected.GET("/notification-preferences", notificationHandler.GetPreferences)
		protected.PUT("/notification-preferences", notificationHandler.UpdatePreferences)
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
		moderator.PUT("/events/:id/moderate", eventHandler.ModerateEvent)

		// Управління проблемами міста
		moderator.PUT("/city-issues/:id/status", cityIssueHandler.UpdateIssueStatus)
		moderator.PUT("/city-issues/:id/assign", cityIssueHandler.AssignIssue)

		// Модерація опитувань
		moderator.PUT("/polls/:id/status", pollHandler.UpdatePoll)
		moderator.DELETE("/polls/:id/force", pollHandler.DeletePoll)

		// Модерація петицій
		moderator.PUT("/petitions/:id/status", petitionHandler.UpdatePetition)
	}

	// ========================================
	// 🔒 АДМІНІСТРАТОРСЬКІ МАРШРУТИ
	// ========================================
	admin := api.Group("")
	admin.Use(middleware.AuthMiddleware(jwtManager))
	admin.Use(middleware.RequireRole("ADMIN"))
	{
		// ===== УПРАВЛІННЯ КОРИСТУВАЧАМИ =====
		admin.GET("/users", usersHandler.GetAllUsers)
		admin.GET("/users/:id", usersHandler.GetUser)
		admin.PUT("/users/:id", usersHandler.UpdateUser)
		admin.DELETE("/users/:id", usersHandler.DeleteUser)
		admin.PUT("/users/:id/block", usersHandler.BlockUser)
		admin.PUT("/users/:id/unblock", usersHandler.UnblockUser)
		admin.PUT("/users/:id/verify", usersHandler.VerifyUser)
		admin.PUT("/users/:id/role", usersHandler.UpdateUserRole)

		// ===== СПОВІЩЕННЯ =====
		// Відправка сповіщень користувачам
		admin.POST("/notifications/send", notificationHandler.SendNotification)

		// Екстрені сповіщення (всім користувачам)
		admin.POST("/notifications/emergency", notificationHandler.SendEmergencyNotification)

		// ===== УПРАВЛІННЯ ТРАНСПОРТОМ =====
		admin.POST("/transport/routes", transportHandler.CreateRoute)
		admin.PUT("/transport/routes/:id", transportHandler.UpdateRoute)
		admin.DELETE("/transport/routes/:id", transportHandler.DeleteRoute)

		admin.POST("/transport/vehicles", transportHandler.CreateVehicle)
		admin.PUT("/transport/vehicles/:id", transportHandler.UpdateVehicle)
		admin.DELETE("/transport/vehicles/:id", transportHandler.DeleteVehicle)

		// ===== АНАЛІТИКА =====
		// Статистика використання платформи
		admin.GET("/analytics/users", usersHandler.GetUserStats)
		admin.GET("/analytics/content", eventHandler.GetContentStats)
		admin.GET("/analytics/polls", pollHandler.GetPollStats)
	}

	// ========================================
	// 🔌 WEBSOCKET МАРШРУТ
	// ========================================
	// WebSocket endpoint для real-time чату
	// ws://localhost:8080/ws
	router.GET("/ws", wsHandler.HandleWebSocket)

	// ========================================
	// 🏥 HEALTH CHECK
	// ========================================
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "nova-kakhovka-ecity",
			"version": "1.0.0",
			"time":    time.Now().Format(time.RFC3339),
		})
	})

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
		log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ Failed to start server: %v", err)
		}
	}()

	// ========================================
	// 13. GRACEFUL SHUTDOWN
	// ========================================
	// Очікуємо сигнал для graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 Shutting down server...")

	// Таймаут для завершення поточних запитів
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Закриваємо WebSocket з'єднання
	log.Println("📡 Closing WebSocket connections...")
	// wsHandler.Shutdown() // Якщо є такий метод

	// Зупиняємо HTTP сервер
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("⚠️  Server forced to shutdown: %v", err)
	}

	log.Println("✅ Server exited gracefully")
}
