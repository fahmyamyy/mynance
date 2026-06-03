package engine

import "time"

type Side string

const (
	SideBuy  Side = "BUY"
	SideSell Side = "SELL"
)

type Order struct {
	ID        string
	UserID    string
	Symbol    string
	Side      Side
	Price     float64
	Quantity  float64
	Remaining float64
	Timestamp time.Time
}

type Trade struct {
	BuyOrderID  string
	SellOrderID string
	BuyUserID   string
	SellUserID  string
	Symbol      string
	Price       float64
	Quantity    float64
	Timestamp   time.Time
}

type PriceLevel struct {
	Price  float64
	Orders []*Order
}

func (pl *PriceLevel) Add(order *Order) {
	pl.Orders = append(pl.Orders, order)
}

func (pl *PriceLevel) Peek() *Order {
	if len(pl.Orders) == 0 {
		return nil
	}
	return pl.Orders[0]
}

func (pl *PriceLevel) Pop() {
	if len(pl.Orders) > 0 {
		pl.Orders = pl.Orders[1:]
	}
}

type OrderBook struct {
	Symbol string
	Bids   []*PriceLevel // DESC by price
	Asks   []*PriceLevel // ASC by price
}

func NewOrderBook(symbol string) *OrderBook {
	return &OrderBook{Symbol: symbol}
}

func (ob *OrderBook) addBid(order *Order) {
	for i, level := range ob.Bids {
		if level.Price == order.Price {
			level.Add(order)
			return
		}
		if order.Price > level.Price {
			newLevel := &PriceLevel{Price: order.Price, Orders: []*Order{order}}
			ob.Bids = append(ob.Bids[:i], append([]*PriceLevel{newLevel}, ob.Bids[i:]...)...)
			return
		}
	}
	ob.Bids = append(ob.Bids, &PriceLevel{Price: order.Price, Orders: []*Order{order}})
}

func (ob *OrderBook) addAsk(order *Order) {
	for i, level := range ob.Asks {
		if level.Price == order.Price {
			level.Add(order)
			return
		}
		if order.Price < level.Price {
			newLevel := &PriceLevel{Price: order.Price, Orders: []*Order{order}}
			ob.Asks = append(ob.Asks[:i], append([]*PriceLevel{newLevel}, ob.Asks[i:]...)...)
			return
		}
	}
	ob.Asks = append(ob.Asks, &PriceLevel{Price: order.Price, Orders: []*Order{order}})
}

func (ob *OrderBook) Match(incoming *Order) []Trade {
	if incoming.Side == SideBuy {
		return ob.matchBuy(incoming)
	}
	return ob.matchSell(incoming)
}

func (ob *OrderBook) matchBuy(buy *Order) []Trade {
	var trades []Trade
	for buy.Remaining > 0 && len(ob.Asks) > 0 {
		bestAsk := ob.Asks[0]
		if buy.Price < bestAsk.Price {
			break
		}
		for buy.Remaining > 0 && len(bestAsk.Orders) > 0 {
			sell := bestAsk.Peek()
			tradeQty := min(buy.Remaining, sell.Remaining)
			trades = append(trades, Trade{
				BuyOrderID:  buy.ID,
				SellOrderID: sell.ID,
				BuyUserID:   buy.UserID,
				SellUserID:  sell.UserID,
				Symbol:      ob.Symbol,
				Price:       bestAsk.Price,
				Quantity:    tradeQty,
				Timestamp:   time.Now().UTC(),
			})
			buy.Remaining -= tradeQty
			sell.Remaining -= tradeQty
			if sell.Remaining == 0 {
				bestAsk.Pop()
			}
		}
		if len(bestAsk.Orders) == 0 {
			ob.Asks = ob.Asks[1:]
		}
	}
	if buy.Remaining > 0 {
		ob.addBid(buy)
	}
	return trades
}

func (ob *OrderBook) matchSell(sell *Order) []Trade {
	var trades []Trade
	for sell.Remaining > 0 && len(ob.Bids) > 0 {
		bestBid := ob.Bids[0]
		if sell.Price > bestBid.Price {
			break
		}
		for sell.Remaining > 0 && len(bestBid.Orders) > 0 {
			buy := bestBid.Peek()
			tradeQty := min(sell.Remaining, buy.Remaining)
			trades = append(trades, Trade{
				BuyOrderID:  buy.ID,
				SellOrderID: sell.ID,
				BuyUserID:   buy.UserID,
				SellUserID:  sell.UserID,
				Symbol:      ob.Symbol,
				Price:       bestBid.Price,
				Quantity:    tradeQty,
				Timestamp:   time.Now().UTC(),
			})
			sell.Remaining -= tradeQty
			buy.Remaining -= tradeQty
			if buy.Remaining == 0 {
				bestBid.Pop()
			}
		}
		if len(bestBid.Orders) == 0 {
			ob.Bids = ob.Bids[1:]
		}
	}
	if sell.Remaining > 0 {
		ob.addAsk(sell)
	}
	return trades
}

func (ob *OrderBook) Cancel(orderID string) bool {
	for i, level := range ob.Bids {
		if removed := removeFromLevel(level, orderID); removed {
			if len(level.Orders) == 0 {
				ob.Bids = append(ob.Bids[:i], ob.Bids[i+1:]...)
			}
			return true
		}
	}
	for i, level := range ob.Asks {
		if removed := removeFromLevel(level, orderID); removed {
			if len(level.Orders) == 0 {
				ob.Asks = append(ob.Asks[:i], ob.Asks[i+1:]...)
			}
			return true
		}
	}
	return false
}

func removeFromLevel(level *PriceLevel, orderID string) bool {
	for i, o := range level.Orders {
		if o.ID == orderID {
			level.Orders = append(level.Orders[:i], level.Orders[i+1:]...)
			return true
		}
	}
	return false
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
