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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"mynance/config"
	"mynance/internal/account"
	"mynance/internal/asset"
	"mynance/internal/auth"
	"mynance/internal/deposit"
	"mynance/internal/engine"
	"mynance/internal/eventbus"
	"mynance/internal/idempotency"
	"mynance/internal/klines"
	"mynance/internal/ledger"
	"mynance/internal/marketdata"
	"mynance/internal/marketfeed"
	"mynance/internal/order"
	"mynance/internal/outbox"
	"mynance/internal/partnerfeed"
	"mynance/internal/trade"
	"mynance/internal/user"
	"mynance/internal/wallet"
	"mynance/internal/withdrawal"
	"mynance/internal/wsfeed"
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
	assetStore := asset.NewStore(pool)
	ledgerStore := ledger.NewStore(pool)
	idempotencyStore := idempotency.NewStore(pool)
	outboxStore := outbox.NewStore(pool)
	orderStore := order.NewStore(pool)
	tradeStore := trade.NewStore(pool)

	// EventBus + Engine
	bus := eventbus.New()
	eng := engine.New(bus, cfg.BinanceSymbols, 1024)
	engAdapter := &engineAdapter{eng: eng}
	mdSvc := marketdata.NewService()
	mdSvc.Subscribe(bus)

	// Services
	ledgerSvc := ledger.NewService(ledgerStore)
	assetSvc := asset.NewService(assetStore)
	provisioner := account.NewProvisioner(accountStore, assetSvc)
	userSvc := user.NewServiceWithProvisioner(pool, userStore, provisioner)
	accountSvc := account.NewService(pool, accountStore, ledgerSvc)
	orderSvc := order.NewServiceWithEngine(pool, orderStore, idempotencyStore, ledgerStore, outboxStore, accountSvc, engAdapter, cfg.BinanceSymbols)
	tradeSvc := trade.NewService(pool, tradeStore, idempotencyStore, ledgerStore, orderStore, outboxStore, accountSvc)
	outboxPublisher := outbox.NewPublisher(pool, outboxStore, nil, time.Second)

	// Settlement processor is environment-aware. Sandbox books user↔partner
	// trades against the MARKET user so they land in DB through the normal
	// trade path; production drops any partner-side trade (and the partner
	// ingester is not started anyway). Cutover requires only the env flag.
	settlement := newSettlementProcessor(cfg, pool, tradeSvc, orderStore)
	bus.Subscribe(eventbus.EventTypeTradeMatched, settlement.OnTradeMatched)

	// WS hub + publisher wiring. Engine-side bridge runs immediately;
	// marketfeed-side bridge attaches after mfClient is constructed.
	wsHub := wsfeed.NewHub()
	wsfeed.NewEngineBridge(wsHub).Subscribe(bus)

	// Rehydrate engine from open/partial orders
	if err := rehydrateEngine(ctx, pool, eng, cfg.BinanceSymbols); err != nil {
		return fmt.Errorf("rehydrate engine: %w", err)
	}

	// Auth
	signer := auth.NewSigner(cfg.JWTSecret)

	// Binance marketfeed
	mfClient := marketfeed.NewClient(cfg.BinanceSymbols, cfg.MarketfeedEnabled)
	mfHandler := marketfeed.NewHandler(mfClient)

	// Marketfeed → WS bridge (kline + ticker passthrough).
	mfClient.SubscribeKline(func(k marketfeed.KlineSnapshot) {
		wsHub.Publish("kline."+k.Symbol+"."+k.Interval, k)
	})
	mfClient.SubscribeTicker(func(t marketfeed.TickerSnapshot) {
		wsHub.Publish("ticker."+t.Symbol, t)
	})

	// Klines REST proxy
	klinesHandler := klines.NewHandler(klines.NewService())

	// WS handler
	wsHandler := wsfeed.NewHandler(wsHub)

	// Wallet + deposit + withdrawal
	walletStore := wallet.NewStore(pool)
	depositStore := deposit.NewStore(pool)
	withdrawalStore := withdrawal.NewStore(pool)
	catalogAdapter := &assetCatalogAdapter{svc: assetSvc}
	walletSvc := wallet.NewService(pool, walletStore, accountSvc, catalogAdapter)
	depositSvc := deposit.NewService(pool, depositStore, walletSvc, ledgerStore, outboxStore, accountSvc)
	withdrawalSvc := withdrawal.NewService(pool, withdrawalStore, idempotencyStore, ledgerStore, outboxStore, accountSvc, catalogAdapter)
	walletHandler := wallet.NewHandler(walletSvc)
	depositHandler := deposit.NewHandler(depositSvc, newDepositProcessor(cfg, depositSvc))
	withdrawalHandler := withdrawal.NewHandler(withdrawalSvc, newWithdrawalProcessor(cfg, withdrawalSvc))

	// Handlers
	userHandler := user.NewHandler(userSvc, signer)
	accountHandler := account.NewHandler(accountSvc)
	assetHandler := asset.NewHandler(assetSvc)
	orderHandler := order.NewHandler(orderSvc)
	tradeHandler := trade.NewHandler(tradeSvc)
	mdHandler := marketdata.NewHandler(mdSvc)
	ledgerHandler := ledger.NewHandler(ledgerSvc)

	// Router
	r := chi.NewRouter()
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RequestID)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Requested-With", "Idempotency-Key"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
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
	r.Mount("/markets", mfHandler.Routes())
	r.Mount("/klines", klinesHandler.Routes())
	r.Mount("/assets", assetHandler.Routes())
	r.Handle("/ws", wsHandler)

	// Protected routes
	r.Group(func(pr chi.Router) {
		pr.Use(auth.Middleware(signer))
		pr.Get("/me", userHandler.Me)
		pr.Get("/users", userHandler.ListUsers)
		pr.Get("/users/{id}", userHandler.GetUser)
		pr.Delete("/users/{id}", userHandler.DeleteUser)
		pr.Get("/users/{userID}/orders", orderHandler.ListByUser)
		pr.Get("/users/{userID}/trades", tradeHandler.ListByUser)
		pr.Mount("/me/assets", accountHandler.Routes())
		pr.Mount("/me/ledger", ledgerHandler.Routes())
		pr.Mount("/admin/accounts", accountHandler.AdminRoutes())
		pr.Mount("/admin/ledger", ledgerHandler.AdminRoutes())
		pr.Mount("/withdrawals", withdrawalHandler.Routes())
		pr.Mount("/orders", orderHandler.Routes())
		pr.Mount("/trades", tradeHandler.Routes())
		pr.Mount("/wallets", walletHandler.Routes())
		pr.Mount("/deposits", depositHandler.UserRoutes())

		pr.Group(func(ar chi.Router) {
			ar.Use(auth.RequireAdmin)
			ar.Mount("/admin/deposits", depositHandler.AdminRoutes())
		})
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

	marketfeedDone := make(chan struct{})
	go func() {
		mfClient.Start(workerCtx)
		close(marketfeedDone)
	}()

	// Partner ingester only runs in sandbox: in production it would inject
	// partner liquidity into the real engine, which is exactly what production
	// must not allow. The env guard makes the cutover config-only.
	partnerfeedDone := make(chan struct{})
	if cfg.IsSandbox() && cfg.SimbotEnabled {
		feed := partnerfeed.FeedAdapter{Get: mfClient.RawSnapshot}
		bot, err := partnerfeed.New(partnerfeed.DefaultConfig(cfg.BinanceSymbols), engAdapter, feed)
		if err != nil {
			return fmt.Errorf("partner ingester init: %w", err)
		}
		go func() {
			bot.Start(workerCtx)
			close(partnerfeedDone)
		}()
	} else {
		close(partnerfeedDone)
	}

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
	<-marketfeedDone
	<-partnerfeedDone
	slog.Info("server stopped")
	return nil
}

func rehydrateEngine(ctx context.Context, pool *pgxpool.Pool, eng *engine.Engine, symbols []string) error {
	rows, err := pool.Query(ctx,
		`SELECT id::text, user_id::text, symbol, side, price::text, (quantity - filled_quantity)::text, created_at
		 FROM orders
		 WHERE status IN ('OPEN','PARTIAL') AND symbol = ANY($1)
		 ORDER BY created_at ASC`, symbols)
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

// assetCatalogAdapter bridges internal/asset.Service to the minimal
// consumer-side interfaces declared by wallet and withdrawal. Each package
// asks for just the methods it needs (network lookup, address validation),
// so this single adapter satisfies both without leaking asset types.
type assetCatalogAdapter struct {
	svc asset.Service
}

func (a *assetCatalogAdapter) NetworkID(ctx context.Context, assetSymbol, networkName string) (uuid.UUID, error) {
	n, err := a.svc.GetNetwork(ctx, assetSymbol, networkName)
	if err != nil {
		return uuid.Nil, err
	}
	return n.ID, nil
}

func (a *assetCatalogAdapter) ValidateAddress(ctx context.Context, networkID uuid.UUID, address string) error {
	return a.svc.ValidateAddress(ctx, networkID, address)
}

// newDepositProcessor picks the deposit strategy for the current environment.
// Sandbox runs the simulator; everything else returns 501 — user-initiated
// deposits do not exist in production because real funds arrive via the
// on-chain listener and admin intake path.
func newDepositProcessor(cfg *config.Config, svc deposit.Service) deposit.Processor {
	if cfg.IsSandbox() {
		return deposit.NewSandboxProcessor(svc)
	}
	return deposit.NewProcessor()
}

// newWithdrawalProcessor picks the withdrawal strategy for the current
// environment. Sandbox auto-fills the idempotency key and applies the
// simulator amount cap; production requires an explicit key from the caller.
func newWithdrawalProcessor(cfg *config.Config, svc withdrawal.Service) withdrawal.Processor {
	if cfg.IsSandbox() {
		return withdrawal.NewSandboxProcessor(svc)
	}
	return withdrawal.NewProcessor(svc)
}

// newSettlementProcessor picks the engine settlement strategy for the
// current environment. Sandbox books user↔partner crossings against the MARKET
// user (user.MarketUserID); production drops any trade that touches a partner
// (ownerless) side (and in production the partner ingester is not started, so
// this is the identity case).
func newSettlementProcessor(
	cfg *config.Config,
	pool *pgxpool.Pool,
	tradeSvc trade.Service,
	orderStore order.Store,
) engine.SettlementProcessor {
	if cfg.IsSandbox() {
		return engine.NewSandboxProcessor(pool, tradeSvc, orderStore, uuid.MustParse(user.MarketUserID))
	}
	return engine.NewProcessor(tradeSvc)
}
