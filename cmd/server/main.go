package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"mynance/config"
	"mynance/internal/account"
	"mynance/internal/auth"
	"mynance/internal/engine"
	"mynance/internal/eventbus"
	"mynance/internal/idempotency"
	"mynance/internal/ledger"
	"mynance/internal/marketdata"
	"mynance/internal/order"
	"mynance/internal/outbox"
	"mynance/internal/trade"
	"mynance/internal/user"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}

type engineAdapter struct {
	eng *engine.Engine
}

func (a *engineAdapter) SubmitPlace(orderID, userID, symbol, side string, price, qty float64) error {
	return a.eng.Submit(engine.Command{
		Kind: engine.CommandPlaceOrder,
		Order: &engine.Order{
			ID:        orderID,
			UserID:    userID,
			Symbol:    symbol,
			Side:      engine.Side(side),
			Price:     price,
			Quantity:  qty,
			Remaining: qty,
			Timestamp: time.Now().UTC(),
		},
	})
}

func (a *engineAdapter) SubmitCancel(orderID, symbol string) error {
	return a.eng.Submit(engine.Command{
		Kind:   engine.CommandCancelOrder,
		Cancel: &engine.CancelInfo{OrderID: orderID, Symbol: symbol},
	})
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx := context.Background()
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("parse database url: %w", err)
	}
	if cfg.DBMaxConns > 0 {
		poolConfig.MaxConns = int32(cfg.DBMaxConns)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return fmt.Errorf("create connection pool: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	slog.Info("database connected")

	// Stores
	userStore := user.NewStore(pool)
	accountStore := account.NewStore(pool)
	ledgerStore := ledger.NewStore(pool)
	idempotencyStore := idempotency.NewStore(pool)
	outboxStore := outbox.NewStore(pool)
	orderStore := order.NewStore(pool)
	tradeStore := trade.NewStore(pool)

	// EventBus + Engine
	bus := eventbus.New()
	eng := engine.New(bus, 1024)
	engAdapter := &engineAdapter{eng: eng}
	mdSvc := marketdata.NewService()
	mdSvc.Subscribe(bus)

	// Services
	ledgerSvc := ledger.NewService(ledgerStore)
	userSvc := user.NewService(pool, userStore)
	accountSvc := account.NewService(pool, accountStore, ledgerSvc)
	orderSvc := order.NewServiceWithEngine(pool, orderStore, idempotencyStore, ledgerStore, outboxStore, accountSvc, engAdapter)
	tradeSvc := trade.NewService(pool, tradeStore, idempotencyStore, ledgerStore, orderStore, outboxStore, accountSvc)
	outboxPublisher := outbox.NewPublisher(pool, outboxStore, nil, time.Second)

	// Settlement subscriber
	settlement := engine.NewSettlementSubscriber(tradeSvc)
	bus.Subscribe(eventbus.EventTypeTradeMatched, settlement.OnTradeMatched)

	// Rehydrate engine from open/partial orders
	if err := rehydrateEngine(ctx, pool, eng); err != nil {
		return fmt.Errorf("rehydrate engine: %w", err)
	}

	// Auth
	signer := auth.NewSigner(cfg.JWTSecret)

	// Handlers
	userHandler := user.NewHandler(userSvc, signer)
	accountHandler := account.NewHandler(accountSvc)
	orderHandler := order.NewHandler(orderSvc)
	tradeHandler := trade.NewHandler(tradeSvc)
	mdHandler := marketdata.NewHandler(mdSvc)

	// Router
	r := chi.NewRouter()
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RequestID)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Public routes
	r.Post("/login", userHandler.Login)
	r.Post("/users", userHandler.CreateUser)
	r.Mount("/orderbook", mdHandler.OrderBookRoutes())
	r.Mount("/marketdata/trades", mdHandler.TradesRoutes())

	// Protected routes
	r.Group(func(pr chi.Router) {
		pr.Use(auth.Middleware(signer))
		pr.Get("/me", userHandler.Me)
		pr.Get("/users", userHandler.ListUsers)
		pr.Get("/users/{id}", userHandler.GetUser)
		pr.Delete("/users/{id}", userHandler.DeleteUser)
		pr.Get("/users/{userID}/orders", orderHandler.ListByUser)
		pr.Get("/users/{userID}/trades", tradeHandler.ListByUser)
		pr.Mount("/accounts", accountHandler.Routes())
		pr.Mount("/orders", orderHandler.Routes())
		pr.Mount("/trades", tradeHandler.Routes())
	})

	port := cfg.ServerPort
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	shutdownCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	workerCtx, cancelWorkers := context.WithCancel(context.Background())
	defer cancelWorkers()

	outboxDone := make(chan struct{})
	go func() {
		outboxPublisher.Start(workerCtx)
		close(outboxDone)
	}()

	engineDone := make(chan struct{})
	go func() {
		eng.Start(workerCtx)
		close(engineDone)
	}()

	go func() {
		slog.Info("server starting", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server listen failed", "err", err)
			os.Exit(1)
		}
	}()

	<-shutdownCtx.Done()
	slog.Info("shutting down server")

	drainCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(drainCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	cancelWorkers()
	<-engineDone
	<-outboxDone
	slog.Info("server stopped")
	return nil
}

func rehydrateEngine(ctx context.Context, pool *pgxpool.Pool, eng *engine.Engine) error {
	rows, err := pool.Query(ctx,
		`SELECT id::text, user_id::text, symbol, side, price::text, (quantity - filled_quantity)::text, created_at
		 FROM orders
		 WHERE status IN ('OPEN','PARTIAL')
		 ORDER BY created_at ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id, userID, symbol, side, priceStr, remainStr string
		var createdAt time.Time
		if err := rows.Scan(&id, &userID, &symbol, &side, &priceStr, &remainStr, &createdAt); err != nil {
			return err
		}
		price, err := strconv.ParseFloat(priceStr, 64)
		if err != nil {
			slog.Warn("rehydrate parse price", "id", id, "err", err)
			continue
		}
		remain, err := strconv.ParseFloat(remainStr, 64)
		if err != nil || remain <= 0 {
			continue
		}
		eng.LoadOrder(&engine.Order{
			ID:        id,
			UserID:    userID,
			Symbol:    symbol,
			Side:      engine.Side(side),
			Price:     price,
			Quantity:  remain,
			Remaining: remain,
			Timestamp: createdAt,
		})
		count++
	}
	slog.Info("engine rehydrated", "orders", count)
	return rows.Err()
}
