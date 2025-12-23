package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_exchangeName(t *testing.T) {
	assert.Equal(t, ExchangeBinance.String(), "binance")
	name, err := ValidExchangeName("binance")
	assert.Equal(t, name, ExchangeName("binance"))
	assert.NoError(t, err)
	name, err = ValidExchangeName("polymarket")
	assert.Equal(t, name, ExchangeName("polymarket"))
	assert.NoError(t, err)
	_, err = ValidExchangeName("dummy")
	assert.Error(t, err)
	assert.True(t, ExchangeBinance.IsValid())
	assert.True(t, ExchangePolymarket.IsValid())
}
