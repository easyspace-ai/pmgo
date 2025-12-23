package polymarketbtcupdown

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
)

// 一个用于连通性测试的跨交易所策略：
// - Binance: 订阅 BTCUSDT 15m KLine，判断本根 K 线是上涨还是下跌
// - Polymarket: 对应买入 YES/NO（默认 dry-run，不会真实下单）
//
// 该策略刻意保持 bbgo 的整体风格：通过 CrossSubscribe/CrossRun 注入两个 session。

const ID = "polymarket-btc15m-updown"

var log = logrus.WithField("strategy", ID)

func init() {
	bbgo.RegisterStrategy(ID, &Strategy{})
}

type Strategy struct {
	// BinanceSession 用于行情源（默认 "binance"）
	BinanceSession string `json:"binanceSession" yaml:"binanceSession"`

	// PolymarketSession 用于交易端（默认 "polymarket"）
	PolymarketSession string `json:"polymarketSession" yaml:"polymarketSession"`

	// SourceSymbol 为 Binance 的 symbol（默认 BTCUSDT）
	SourceSymbol string `json:"sourceSymbol" yaml:"sourceSymbol"`

	// Interval 为 KLine 周期（默认 15m）
	Interval types.Interval `json:"interval" yaml:"interval"`

	// YesSymbol / NoSymbol 为 Polymarket 的交易 symbol（需要在 Polymarket market 列表里存在）
	YesSymbol string `json:"yesSymbol" yaml:"yesSymbol"`
	NoSymbol  string `json:"noSymbol" yaml:"noSymbol"`

	// EntryPrice 为下单价格（Polymarket 概率价格通常在 0~1；这里只是示例）
	EntryPrice fixedpoint.Value `json:"entryPrice" yaml:"entryPrice"`

	// QuoteAmount 为每次下注的 USDC 金额（会换算为 quantity = QuoteAmount / EntryPrice）
	QuoteAmount fixedpoint.Value `json:"quoteAmount" yaml:"quoteAmount"`
}

func (s *Strategy) ID() string { return ID }

func (s *Strategy) Defaults() error {
	if s.BinanceSession == "" {
		s.BinanceSession = "binance"
	}
	if s.PolymarketSession == "" {
		s.PolymarketSession = "polymarket"
	}
	if s.SourceSymbol == "" {
		s.SourceSymbol = "BTCUSDT"
	}
	if s.Interval == "" {
		s.Interval = types.Interval15m
	}
	if s.YesSymbol == "" {
		s.YesSymbol = "PM_BTC_15M_UP_YES_USDC"
	}
	if s.NoSymbol == "" {
		s.NoSymbol = "PM_BTC_15M_UP_NO_USDC"
	}
	if s.EntryPrice.IsZero() {
		s.EntryPrice = fixedpoint.NewFromFloat(0.5)
	}
	if s.QuoteAmount.IsZero() {
		s.QuoteAmount = fixedpoint.NewFromFloat(5)
	}
	return nil
}

func (s *Strategy) Validate() error {
	if s.BinanceSession == "" || s.PolymarketSession == "" {
		return fmt.Errorf("binanceSession/polymarketSession is required")
	}
	if s.SourceSymbol == "" {
		return fmt.Errorf("sourceSymbol is required")
	}
	if s.Interval == "" {
		return fmt.Errorf("interval is required")
	}
	if s.YesSymbol == "" || s.NoSymbol == "" {
		return fmt.Errorf("yesSymbol/noSymbol is required")
	}
	if s.EntryPrice.Sign() <= 0 {
		return fmt.Errorf("entryPrice must be positive")
	}
	if s.QuoteAmount.Sign() <= 0 {
		return fmt.Errorf("quoteAmount must be positive")
	}
	return nil
}

func (s *Strategy) CrossSubscribe(sessions map[string]*bbgo.ExchangeSession) {
	binanceSession, ok := sessions[s.BinanceSession]
	if !ok {
		// 这里不 return error（CrossSubscribe 接口不返回），在 CrossRun 里会再做一次校验。
		return
	}

	binanceSession.Subscribe(types.KLineChannel, s.SourceSymbol, types.SubscribeOptions{Interval: s.Interval})
}

func (s *Strategy) CrossRun(ctx context.Context, router bbgo.OrderExecutionRouter, sessions map[string]*bbgo.ExchangeSession) error {
	if err := s.Defaults(); err != nil {
		return err
	}
	if err := s.Validate(); err != nil {
		return err
	}

	binanceSession, ok := sessions[s.BinanceSession]
	if !ok {
		return fmt.Errorf("binance session %q not found", s.BinanceSession)
	}
	_, ok = sessions[s.PolymarketSession]
	if !ok {
		return fmt.Errorf("polymarket session %q not found", s.PolymarketSession)
	}

	binanceSession.MarketDataStream.OnKLineClosed(func(kline types.KLine) {
		if kline.Symbol != s.SourceSymbol || kline.Interval != s.Interval {
			return
		}

		// 极简 up/down 规则：收盘 > 开盘 => up，否则 down
		up := kline.Close.Compare(kline.Open) > 0
		targetSymbol := s.NoSymbol
		if up {
			targetSymbol = s.YesSymbol
		}

		quantity := s.QuoteAmount.Div(s.EntryPrice)

		log.WithFields(logrus.Fields{
			"source":        s.SourceSymbol,
			"interval":      s.Interval,
			"open":          kline.Open.String(),
			"close":         kline.Close.String(),
			"targetSymbol":  targetSymbol,
			"entryPrice":    s.EntryPrice.String(),
			"quoteAmount":   s.QuoteAmount.String(),
			"orderQuantity": quantity.String(),
		}).Info("signal generated, submitting polymarket order")

		_, err := router.SubmitOrdersTo(ctx, s.PolymarketSession, types.SubmitOrder{
			Symbol:      targetSymbol,
			Side:        types.SideTypeBuy,
			Type:        types.OrderTypeLimit,
			Price:       s.EntryPrice,
			Quantity:    quantity,
			TimeInForce: types.TimeInForceGTC,
			Tag:         ID,
		})
		if err != nil {
			log.WithError(err).Error("failed to submit polymarket order")
		}
	})

	return nil
}

