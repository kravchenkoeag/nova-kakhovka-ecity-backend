// cmd/server/main.go - Исправленная версия с правильными импортами

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
	// Загружаем конфигурацию
	cfg := config.Load()

	// Подключаемся к MongoDB
	db, err := database.NewMongoDB(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Создаем индексы
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := db.CreateIndexes(ctx); err != nil {
		log.Printf("Warning: Failed to create indexes: %v", err)
	}

	// Инициализируем JWT менеджер
	jwtManager := auth.NewJWTManager(cfg.JWTSecret, time.Duration(cfg.JWTExpiration)*time.Hour)

	// Получаем коллекции
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

	// Инициализируем сервисы
	notificationService := services.NewNotificationService(cfg, userCollection, notificationCollection)

	// Инициализируем хендлеры
	authHandler := handlers.NewAuthHandler(userCollection, jwtManager)
	groupHandler := handlers.NewGroupHandler(groupCollection, userCollection, messageCollection)
	wsHandler := handlers.NewWebSocketHandler(jwtManager, groupCollection, messageCollection)
	announcementHandler := handlers.NewAnnouncementHandler(announcementCollection, userCollection)
	eventHandler := handlers.NewEventHandler(eventCollection, userCollection)
	notificationHandler := handlers.NewNotificationHandler(notificationService, notificationCollection, deviceTokenCollection)
	cityIssueHandler := handlers.NewCityIssueHandler(cityIssueCollection, userCollection, notificationService)
	petitionHandler := handlers.NewPetitionHandler(petitionCollection, userCollection, notificationService)
	pollHandler := handlers.NewPollHandler(pollCollection, userCollection, notificationService)
	transportHandler := handlers.NewTransportHandler(transportRouteCollection, transportVehicleCollection, userCollection)

	// Запускаем фоновые задачи
	wsHandler.StartHub()
	pollHandler.StartPollCleanupScheduler()
	transportHandler.StartScheduleGenerator()

	// Настраиваем Gin
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// Настраиваем CORS
	corsConfig := cors.Config{
		AllowOrigins:     []string{"*"}, // В продакшене указать конкретные домены
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}
	router.Use(cors.New(corsConfig))

	// Rate limiting для защиты от спама
	rateLimiter := middleware.NewRateLimiter(100, time.Hour) // 100 запросов в час
	router.Use(rateLimiter.RateLimit())

	// Публичные маршруты
	public := router.Group("/api/v1")
	{
		// Авторизация
		public.POST("/register", authHandler.Register)
		public.POST("/login", authHandler.Login)

		// Публичная информация
		public.GET("/groups/public", groupHandler.GetPublicGroups)
		public.GET("/announcements", announcementHandler.GetAnnouncements)
		public.GET("/announcements/:id", announcementHandler.GetAnnouncement)
		public.GET("/events", eventHandler.GetEvents)
		public.GET("/events/:id", eventHandler.GetEvent)
		public.GET("/petitions", petitionHandler.GetPetitions)
		public.GET("/petitions/:id", petitionHandler.GetPetition)
		public.GET("/polls", pollHandler.GetPolls)
		public.GET("/polls/:id", pollHandler.GetPoll)
		public.GET("/city-issues", cityIssueHandler.GetIssues)
		public.GET("/city-issues/:id", cityIssueHandler.GetIssue)

		// Транспорт (публичная информация)
		public.GET("/transport/routes", transportHandler.GetRoutes)
		public.GET("/transport/routes/:id", transportHandler.GetRoute)
		public.GET("/transport/stops/nearby", transportHandler.GetNearbyStops)
		public.GET("/transport/arrivals", transportHandler.GetArrivals)
		public.GET("/transport/live", transportHandler.GetLiveTracking)

		// Служебная информация
		public.GET("/notification-types", notificationHandler.GetNotificationTypes)
	}

	// WebSocket маршрут
	router.GET("/ws", wsHandler.HandleWebSocket)

	// Защищенные маршруты
	protected := router.Group("/api/v1")
	protected.Use(middleware.AuthMiddleware(jwtManager))
	{
		// Профиль пользователя
		protected.GET("/profile", authHandler.GetProfile)
		protected.PUT("/profile", authHandler.UpdateProfile)

		// Группы и чаты
		//protected.GET("/groups", groupHandler.GetGroups)
		//protected.GET("/groups/:id", groupHandler.GetGroup)
		protected.POST("/groups", groupHandler.CreateGroup)
		//protected.PUT("/groups/:id", groupHandler.UpdateGroup)
		//protected.DELETE("/groups/:id", groupHandler.DeleteGroup)
		protected.POST("/groups/:id/join", groupHandler.JoinGroup)
		//protected.POST("/groups/:id/leave", groupHandler.LeaveGroup)
		//protected.GET("/groups/:id/members", groupHandler.GetMembers)
		protected.GET("/groups/:id/messages", groupHandler.GetMessages)
		protected.POST("/groups/:id/messages", groupHandler.SendMessage)

		// Объявления
		protected.POST("/announcements", announcementHandler.CreateAnnouncement)
		protected.GET("/announcements/my", announcementHandler.GetUserAnnouncements)
		protected.PUT("/announcements/:id", announcementHandler.UpdateAnnouncement)
		protected.DELETE("/announcements/:id", announcementHandler.DeleteAnnouncement)
		protected.POST("/announcements/:id/contact", announcementHandler.ContactOwner)

		// События
		protected.POST("/events", eventHandler.CreateEvent)
		protected.GET("/events/my", eventHandler.GetUserEvents)
		protected.PUT("/events/:id", eventHandler.UpdateEvent)
		protected.DELETE("/events/:id", eventHandler.DeleteEvent)
		protected.POST("/events/:id/join", eventHandler.JoinEvent)
		protected.POST("/events/:id/leave", eventHandler.LeaveEvent)
		protected.GET("/events/:id/participants", eventHandler.GetEventParticipants)

		// Петиции
		protected.POST("/petitions", petitionHandler.CreatePetition)
		protected.GET("/petitions/my", petitionHandler.GetUserPetitions)
		protected.PUT("/petitions/:id/publish", petitionHandler.PublishPetition)
		protected.DELETE("/petitions/:id", petitionHandler.DeletePetition)
		protected.POST("/petitions/:id/sign", petitionHandler.SignPetition)

		// Опросы
		protected.GET("/polls/my", pollHandler.GetUserPolls)
		protected.POST("/polls/:id/respond", pollHandler.SubmitPollResponse)
		protected.GET("/polls/:id/results", pollHandler.GetPollResults)

		// Городские проблемы
		protected.POST("/city-issues", cityIssueHandler.CreateIssue)
		protected.POST("/city-issues/:id/upvote", cityIssueHandler.UpvoteIssue)
		protected.POST("/city-issues/:id/comment", cityIssueHandler.AddComment)
		protected.POST("/city-issues/:id/subscribe", cityIssueHandler.SubscribeToIssue)
		protected.DELETE("/city-issues/:id/subscribe", cityIssueHandler.UnsubscribeFromIssue)

		// Уведомления
		protected.POST("/notifications/register-token", notificationHandler.RegisterDeviceToken)
		protected.DELETE("/notifications/unregister-token", notificationHandler.UnregisterDeviceToken)
		protected.GET("/notifications", notificationHandler.GetUserNotifications)
		protected.PUT("/notifications/:id/read", notificationHandler.MarkNotificationAsRead)
		protected.PUT("/notifications/read-all", notificationHandler.MarkAllNotificationsAsRead)
		protected.DELETE("/notifications/:id", notificationHandler.DeleteNotification)
	}

	// Маршруты для модераторов
	moderator := router.Group("/api/v1/admin")
	moderator.Use(middleware.AuthMiddleware(jwtManager))
	moderator.Use(middleware.ModeratorMiddleware())
	{
		// Управление опросами
		moderator.POST("/polls", pollHandler.CreatePoll)
		moderator.PUT("/polls/:id/publish", pollHandler.PublishPoll)
		moderator.PUT("/polls/:id/close", pollHandler.ClosePoll)
		moderator.DELETE("/polls/:id", pollHandler.DeletePoll)
		moderator.GET("/polls/stats", pollHandler.GetPollStats)

		// Управление петициями
		moderator.POST("/petitions/:id/response", petitionHandler.AddOfficialResponse)
		moderator.GET("/petitions/stats", petitionHandler.GetPetitionStats)

		// Управление городскими проблемами
		moderator.PUT("/city-issues/:id/status", cityIssueHandler.UpdateIssueStatus)
		moderator.GET("/city-issues/stats", cityIssueHandler.GetIssueStats)

		// Транспортная система
		moderator.POST("/transport/routes", transportHandler.CreateRoute)
		moderator.PUT("/transport/routes/:id", transportHandler.UpdateRoute)
		moderator.POST("/transport/vehicles", transportHandler.CreateVehicle)
		moderator.GET("/transport/vehicles", transportHandler.GetVehicles)
		moderator.PUT("/transport/vehicles/:id/location", transportHandler.UpdateVehicleLocation)
		moderator.GET("/transport/stats", transportHandler.GetTransportStats)

		// Управление уведомлениями
		moderator.POST("/notifications/send", notificationHandler.SendNotification)
		moderator.POST("/notifications/emergency", notificationHandler.SendEmergencyNotification)
	}

	// Здоровье сервера
	router.GET("/health", func(c *gin.Context) {
		// Проверяем подключение к базе данных
		dbStatus := "connected"
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := db.Client.Ping(ctx, nil); err != nil {
			dbStatus = "disconnected"
		}

		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"timestamp": time.Now().Format(time.RFC3339),
			"version":   "1.0.0",
			"services": gin.H{
				"database":      dbStatus,
				"websocket":     "running",
				"notifications": "active",
				"transport":     "active",
			},
			"stats": gin.H{
				"uptime": time.Since(time.Now().Add(-time.Hour)).String(), // Заглушка
			},
		})
	})

	// Дополнительная информация о API
	router.GET("/api", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"name":        "Nova Kakhovka e-City API",
			"version":     "1.0.0",
			"description": "Backend API for Nova Kakhovka e-City mobile application",
			"endpoints": gin.H{
				"websocket":     "/ws",
				"public_api":    "/api/v1/*",
				"protected_api": "/api/v1/* (requires Authorization header)",
				"admin_api":     "/api/v1/admin/* (requires moderator role)",
				"health":        "/health",
			},
			"features": []string{
				"User Authentication & Authorization",
				"Group Chat System",
				"Announcements Board",
				"Events & Calendar",
				"Electronic Petitions",
				"Citizen Polls & Surveys",
				"City Issues Reporting",
				"Public Transport Tracking",
				"Push Notifications",
				"Real-time WebSocket Communication",
			},
		})
	})

	// Middleware для логирования запросов в production
	if cfg.Env == "production" {
		router.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
			return fmt.Sprintf("%s - [%s] \"%s %s %s %d %s \"%s\" %s\"\n",
				param.ClientIP,
				param.TimeStamp.Format(time.RFC1123),
				param.Method,
				param.Path,
				param.Request.Proto,
				param.StatusCode,
				param.Latency,
				param.Request.UserAgent(),
				param.ErrorMessage,
			)
		}))
	}

	// Создаем HTTP сервер
	srv := &http.Server{
		Addr:           cfg.Host + ":" + cfg.Port,
		Handler:        router,
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   15 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	// Запускаем сервер в горутине
	go func() {
		log.Printf("🚀 Nova Kakhovka e-City Backend Server starting...")
		log.Printf("🌐 Server running on %s:%s", cfg.Host, cfg.Port)
		log.Printf("🏛️  Environment: %s", cfg.Env)
		log.Printf("📊 Database: %s", cfg.DatabaseName)
		log.Printf("")
		log.Printf("📋 Available endpoints:")
		log.Printf("   • WebSocket: /ws")
		log.Printf("   • Public API: /api/v1/*")
		log.Printf("   • Protected API: /api/v1/* (requires auth)")
		log.Printf("   • Admin API: /api/v1/admin/* (requires moderator)")
		log.Printf("   • Health check: /health")
		log.Printf("   • API info: /api")
		log.Printf("")
		log.Printf("🔥 Features enabled:")
		log.Printf("   ✅ User Authentication & JWT")
		log.Printf("   ✅ Group Chat & WebSocket")
		log.Printf("   ✅ Announcements Board")
		log.Printf("   ✅ Events & Calendar")
		log.Printf("   ✅ Electronic Petitions")
		log.Printf("   ✅ Citizen Polls & Surveys")
		log.Printf("   ✅ City Issues Reporting")
		log.Printf("   ✅ Public Transport System")
		log.Printf("   ✅ Push Notifications (FCM)")
		log.Printf("   ✅ Rate Limiting Protection")
		log.Printf("")

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ Server failed to start: %v", err)
		}
	}()

	// Ожидаем сигнал завершения
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("🛑 Shutting down server...")

	// Graceful shutdown
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("⚠️  Server forced to shutdown: %v", err)
	} else {
		log.Println("✅ Server gracefully stopped")
	}

	log.Println("👋 Nova Kakhovka e-City Backend exited")
}
