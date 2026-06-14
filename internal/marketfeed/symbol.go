package marketfeed

import (
	"fmt"
	"strings"
)

func toBinance(symbol string) string {
	return strings.ToLower(strings.ReplaceAll(symbol, "-", ""))
}

func fromBinance(binance string) (string, error) {
	upper := strings.ToUpper(binance)
	for _, quote := range []string{"USDT", "USDC", "BUSD", "USD", "BTC", "ETH"} {
		if strings.HasSuffix(upper, quote) && len(upper) > len(quote) {
			base := upper[:len(upper)-len(quote)]
			return base + "-" + quote, nil
		}
	}
	return "", fmt.Errorf("fromBinance: cannot split %q", binance)
}

// splitSymbol splits "BTC-USDT" → ("BTC", "USDT"). Returns ("", "") on malformed input.
func splitSymbol(symbol string) (base, quote string) {
	parts := strings.SplitN(symbol, "-", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}
