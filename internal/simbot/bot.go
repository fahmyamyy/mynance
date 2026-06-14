// Package simbot ingests the partner exchange L2 depth snapshot into the
// local matching engine as ownerless ("partner") orders. Partner orders carry
// an empty UserID; they live only in engine memory and are never persisted to
// the orders/trades/ledger tables directly. A trade where one side is a
// partner order is settled by the engine's sandbox processor against the
// MARKET user (user↔partner); a trade where both sides are partner orders is
// dropped (engine.isPartner classifies by empty UserID).
//
// The local book mirrors the partner book: each tick diffs the mirrored
// levels against the latest snapshot, cancelling levels that disappeared or
// drifted and placing new ones.
//
// Tradeoff: a real user order that crosses a partner level is matched and
// removed from the engine book; in sandbox the fill is booked against MARKET.
// This is acceptable for a small-exchange showcase; routing real orders to the
// partner exchange API is out of scope (sandbox only).
package simbot

import (
	"context"
	"log/slog"
	"math"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// EngineSubmitter is the subset of the matching engine the bot drives.
type EngineSubmitter interface {
	SubmitPlace(orderID, userID, symbol, side string, price, qty float64) error
	SubmitCancel(orderID, symbol string) error
}

// Feed exposes the upstream L2 snapshot we mirror.
type Feed interface {
	GetOrderBook(symbol string) (bids [][2]float64, asks [][2]float64, ok bool)
}

// FeedAdapter wraps marketfeed.Client (or any L2 source) into the Feed shape.
type FeedAdapter struct {
	Get func(symbol string) (bids [][2]float64, asks [][2]float64, ok bool)
}

func (a FeedAdapter) GetOrderBook(symbol string) ([][2]float64, [][2]float64, bool) {
	return a.Get(symbol)
}

type Config struct {
	Symbols      []string
	Depth        int           // number of price levels mirrored per side
	TickInterval time.Duration // how often to diff against the upstream snapshot
	QtyJitter    float64       // ± fraction applied to mirrored qty so trades aren't symmetric
}

func DefaultConfig(symbols []string) Config {
	return Config{
		Symbols:      symbols,
		Depth:        5,
		TickInterval: 200 * time.Millisecond,
		QtyJitter:    0.05,
	}
}

type Bot struct {
	cfg    Config
	engine EngineSubmitter
	feed   Feed
	state  *bookState
}

func New(cfg Config, engine EngineSubmitter, feed Feed) (*Bot, error) {
	return &Bot{
		cfg:    cfg,
		engine: engine,
		feed:   feed,
		state:  newBookState(),
	}, nil
}

func (b *Bot) Start(ctx context.Context) {
	slog.Info("partner ingester starting", "symbols", b.cfg.Symbols, "depth", b.cfg.Depth)
	ticker := time.NewTicker(b.cfg.TickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("partner ingester stopped")
			return
		case <-ticker.C:
			for _, sym := range b.cfg.Symbols {
				b.tick(sym)
			}
		}
	}
}

func (b *Bot) tick(symbol string) {
	bids, asks, ok := b.feed.GetOrderBook(symbol)
	if !ok {
		return
	}
	b.sync(symbol, "BUY", topLevels(bids, b.cfg.Depth))
	b.sync(symbol, "SELL", topLevels(asks, b.cfg.Depth))
}

func (b *Bot) sync(symbol, side string, levels [][2]float64) {
	b.state.mu.Lock()
	defer b.state.mu.Unlock()
	cur := b.state.sideMap(symbol, side)

	want := make(map[string]float64, len(levels))
	for _, lvl := range levels {
		price, qty := lvl[0], lvl[1]
		if price <= 0 || qty <= 0 {
			continue
		}
		want[levelKey(price)] = qty * b.jitter()
	}

	// Cancel levels that disappeared or whose qty drifted materially.
	for key, resting := range cur {
		wantQty, keep := want[key]
		if !keep || materialDrift(resting.qty, wantQty) {
			if err := b.engine.SubmitCancel(resting.orderID, symbol); err != nil {
				slog.Warn("simbot cancel", "err", err)
			}
			delete(cur, key)
		}
	}

	// Place new or replaced levels.
	for key, wantQty := range want {
		if _, exists := cur[key]; exists {
			continue
		}
		price := parseLevelKey(key)
		orderID, err := uuid.NewV7()
		if err != nil {
			continue
		}
		// Partner orders are ownerless: empty userID marks them for
		// settlement against MARKET (see engine.isPartner).
		if err := b.engine.SubmitPlace(
			orderID.String(),
			"",
			symbol, side, price, wantQty,
		); err != nil {
			slog.Warn("simbot place", "err", err)
			continue
		}
		cur[key] = restingOrder{orderID: orderID.String(), qty: wantQty}
	}
}

func (b *Bot) jitter() float64 {
	if b.cfg.QtyJitter <= 0 {
		return 1
	}
	// ±QtyJitter fraction
	return 1 + (rand.Float64()*2-1)*b.cfg.QtyJitter
}

// materialDrift returns true when qty change is large enough to justify
// cancel + replace. Avoids churning the engine on tiny ticker wobble.
func materialDrift(prev, next float64) bool {
	if prev == 0 {
		return next != 0
	}
	delta := math.Abs(next-prev) / prev
	return delta > 0.10
}

func topLevels(src [][2]float64, n int) [][2]float64 {
	if len(src) <= n {
		return src
	}
	return src[:n]
}

func parseLevelKey(key string) float64 {
	f, _ := strconv.ParseFloat(key, 64)
	return f
}
