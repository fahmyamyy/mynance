package marketfeed

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSymbolTranslator(t *testing.T) {
	cases := []struct {
		in      string
		binance string
		back    string
	}{
		{"BTC-USDT", "btcusdt", "BTC-USDT"},
		{"ETH-USDT", "ethusdt", "ETH-USDT"},
		{"SOL-BTC", "solbtc", "SOL-BTC"},
	}
	for _, c := range cases {
		require.Equal(t, c.binance, toBinance(c.in))
		got, err := fromBinance(c.binance)
		require.NoError(t, err)
		require.Equal(t, c.back, got)
	}
}

func TestClient_StalenessAndSymbolGate(t *testing.T) {
	c := NewClient([]string{"BTC-USDT"}, true)
	require.True(t, c.HasSymbol("BTC-USDT"))
	require.False(t, c.HasSymbol("ETH-USDT"))

	_, ok := c.GetOrderBook("BTC-USDT")
	require.False(t, ok)

	_, ok = c.GetTicker("BTC-USDT")
	require.False(t, ok)

	require.Len(t, c.Markets(), 0)
}
