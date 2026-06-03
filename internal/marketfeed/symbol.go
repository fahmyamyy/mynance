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
