package server

import (
	"github.com/anunay/wallet-service/config"
	"github.com/anunay/wallet-service/internal/handler"
	"github.com/anunay/wallet-service/internal/middleware"
	"github.com/anunay/wallet-service/internal/repository"
	"github.com/anunay/wallet-service/internal/service"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Server struct {
	App *fiber.App
	Cfg *config.Config
	Log *zap.Logger
}

func InitializeServer(gormDB *gorm.DB, cfg *config.Config, log *zap.Logger) *Server {
	// 1. Instantiate Repositories (using GORM)
	userRepo := repository.NewUserRepository(gormDB)
	walletRepo := repository.NewWalletRepository(gormDB)
	idempotencyRepo := repository.NewIdempotencyRepository(gormDB)
	transferRepo := repository.NewTransferRepository(gormDB)

	// 2. Instantiate Services
	authSvc := service.NewAuthService(userRepo, cfg.Auth)
	walletSvc := service.NewWalletService(walletRepo, userRepo)
	transferSvc := service.NewTransferService(transferRepo, userRepo, walletRepo, idempotencyRepo, service.WithLogger(log))

	// 3. Instantiate Handlers
	healthHdlr := handler.NewHealthHandler(gormDB)
	metricsHdlr := handler.NewMetricsHandler()
	authHdlr := handler.NewAuthHandler(authSvc, handler.WithAuthHandlerLogger(log))
	walletHdlr := handler.NewWalletHandler(walletSvc, handler.WithWalletHandlerLogger(log))
	transferHdlr := handler.NewTransferHandler(transferSvc, handler.WithTransferHandlerLogger(log))

	// 4. Create Fiber App
	app := fiber.New(fiber.Config{
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		ErrorHandler: createCustomErrorHandler(log),
	})

	// Middlewares
	app.Use(middleware.NewRecoveryMiddleware(log))
	app.Use(cors.New())
	app.Use(middleware.NewLoggerMiddleware(log))

	// Static web UI
	app.Static("/", "./ui")

	// Public routes
	app.Get("/health", healthHdlr.HealthCheck)
	app.Get("/metrics", metricsHdlr.GetMetrics)
	app.Post("/auth/register", authHdlr.Register)
	app.Post("/auth/login", authHdlr.Login)

	// Protected routes
	authMiddleware := middleware.NewAuthMiddleware(cfg.Auth.JWTSecret)
	api := app.Group("", authMiddleware)

	api.Post("/wallets", walletHdlr.GetOrCreateWallet)
	api.Get("/wallets/:id", walletHdlr.GetWalletByID)
	api.Post("/transfers", transferHdlr.InitiateTransfer)
	api.Get("/transfers/:id", transferHdlr.GetTransferByID)

	return &Server{
		App: app,
		Cfg: cfg,
		Log: log,
	}
}

func createCustomErrorHandler(log *zap.Logger) fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		code := fiber.StatusInternalServerError
		if e, ok := err.(*fiber.Error); ok {
			code = e.Code
		}
		log.Error("Unhandled HTTP Error", zap.Int("code", code), zap.Error(err))
		return c.Status(code).JSON(fiber.Map{"error": err.Error()})
	}
}
