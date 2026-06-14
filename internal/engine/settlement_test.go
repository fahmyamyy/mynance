package engine

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"mynance/internal/eventbus"
	"mynance/internal/trade"
)

type stubTradeService struct {
	calls []trade.ExecuteTradeCommand
}

func (s *stubTradeService) ExecuteTrade(ctx context.Context, cmd trade.ExecuteTradeCommand) (*trade.Trade, error) {
	s.calls = append(s.calls, cmd)
	return &trade.Trade{}, nil
}

func (s *stubTradeService) ListByUser(ctx context.Context, userID uuid.UUID, symbol string, limit, offset int) ([]*trade.UserTrade, error) {
	return nil, nil
}

func TestSettlement_OnTradeMatched_CallsExecuteTrade(t *testing.T) {
	stub := &stubTradeService{}
	sub := NewProcessor(stub)

	buyID := uuid.NewString()
	sellID := uuid.NewString()
	sub.OnTradeMatched(eventbus.TradeMatchedEvent{
		Seq:         1,
		Symbol:      "BTC-USDT",
		BuyOrderID:  buyID,
		SellOrderID: sellID,
		BuyUserID:   uuid.NewString(),
		SellUserID:  uuid.NewString(),
		Price:       30000.0,
		Quantity:    0.5,
		Timestamp:   time.Now(),
	})

	require.Len(t, stub.calls, 1)
	require.Equal(t, "BTC-USDT", stub.calls[0].Symbol)
	require.Contains(t, stub.calls[0].IdempotencyKey, "match-")
	require.Contains(t, stub.calls[0].IdempotencyKey, buyID)
	require.Contains(t, stub.calls[0].IdempotencyKey, sellID)
}

func TestSettlement_IgnoresWrongEventType(t *testing.T) {
	stub := &stubTradeService{}
	sub := NewProcessor(stub)
	sub.OnTradeMatched(eventbus.OrderRestedEvent{})
	require.Len(t, stub.calls, 0)
}

func TestSettlement_DropsPartnerSide(t *testing.T) {
	cases := []struct {
		name              string
		buyUser, sellUser string
	}{
		{"partner buy", "", uuid.NewString()},
		{"partner sell", uuid.NewString(), ""},
		{"both partner", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubTradeService{}
			sub := NewProcessor(stub)
			sub.OnTradeMatched(eventbus.TradeMatchedEvent{
				Seq:         1,
				Symbol:      "BTC-USDT",
				BuyOrderID:  uuid.NewString(),
				SellOrderID: uuid.NewString(),
				BuyUserID:   tc.buyUser,
				SellUserID:  tc.sellUser,
				Price:       30000.0,
				Quantity:    0.5,
				Timestamp:   time.Now(),
			})
			require.Len(t, stub.calls, 0)
		})
	}
}
