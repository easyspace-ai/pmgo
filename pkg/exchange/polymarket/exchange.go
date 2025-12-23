package polymarket

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
)

// 说明：
// 1) 这里先提供“最小可用”的 Polymarket Exchange 适配层，让 bbgo 的整体框架能初始化与下单。
// 2) 真实的 Polymarket CLOB 鉴权/下单签名（EIP-712 等）在不同账号体系下差异较大，
//    后续你给我具体的 Polymarket API Key/签名方式后，可以在 SubmitOrder 中替换为真实请求。
//
// 当前实现支持：
// - 通过 POLYMARKET_MARKETS_FILE 或 POLYMARKET_MARKETS_JSON 注入 market 列表
// - Dry-run 下单（默认开启）与内存中的 open orders/取消
//
// 这样可以先把策略和框架跑通，再逐步把 Polymarket 真实交易能力补齐。

const (
	envMarketsFile = "POLYMARKET_MARKETS_FILE"
	envMarketsJSON = "POLYMARKET_MARKETS_JSON"
	envDryRun      = "POLYMARKET_DRY_RUN"
	envBalanceUSDC = "POLYMARKET_BALANCE_USDC"
)

type Exchange struct {
	key        string
	secret     string
	passphrase string

	mu      sync.Mutex
	markets types.MarketMap

	nextOrderID uint64
	orders      map[uint64]*types.Order
}

func New(key, secret, passphrase string) *Exchange {
	return &Exchange{
		key:        key,
		secret:     secret,
		passphrase: passphrase,
		markets:    nil,
		orders:     make(map[uint64]*types.Order),
		// order id 从 1 开始，方便调试
		nextOrderID: 1,
	}
}

func (e *Exchange) Name() types.ExchangeName { return types.ExchangePolymarket }

// Polymarket 以 USDC 为主要结算资产（目前按常见实现设定）。
func (e *Exchange) PlatformFeeCurrency() string { return "USDC" }

func (e *Exchange) NewStream() types.Stream { return NewStream() }

func (e *Exchange) DefaultFeeRates() types.ExchangeFee {
	// Polymarket 的费率取决于具体 API/市场；这里先给一个 0 的默认值，避免框架强制从 Account 取费率。
	return types.ExchangeFee{
		MakerFeeRate: fixedpoint.Zero,
		TakerFeeRate: fixedpoint.Zero,
	}
}

func (e *Exchange) QueryMarkets(ctx context.Context) (types.MarketMap, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.markets != nil && len(e.markets) > 0 {
		return e.markets, nil
	}

	markets, err := loadMarketsFromEnv()
	if err != nil {
		return nil, err
	}

	// 兜底：如果用户没有配置 market，给一个可运行的默认 market 列表（用于示例策略）。
	if len(markets) == 0 {
		markets = defaultExampleMarkets()
	}

	// 填充 Exchange 字段
	for symbol, m := range markets {
		m.Exchange = types.ExchangePolymarket
		if m.Symbol == "" {
			m.Symbol = symbol
		}
		markets[symbol] = m
	}

	e.markets = markets
	return e.markets, nil
}

func (e *Exchange) QueryTicker(ctx context.Context, symbol string) (*types.Ticker, error) {
	// 最小实现：不调用真实接口；返回一个可用但可能为 0 的 ticker。
	// 如果你在 Polymarket session 里只持有 USDC，这里通常不会影响 bbgo 的初始化流程。
	t := &types.Ticker{
		Time: time.Now(),
	}
	return t, nil
}

func (e *Exchange) QueryTickers(ctx context.Context, symbol ...string) (map[string]types.Ticker, error) {
	out := make(map[string]types.Ticker, len(symbol))
	for _, s := range symbol {
		t, err := e.QueryTicker(ctx, s)
		if err != nil {
			return nil, err
		}
		out[s] = *t
	}
	return out, nil
}

func (e *Exchange) QueryKLines(ctx context.Context, symbol string, interval types.Interval, options types.KLineQueryOptions) ([]types.KLine, error) {
	return nil, fmt.Errorf("polymarket: QueryKLines is not implemented (use Binance session for kline source)")
}

func (e *Exchange) QueryAccount(ctx context.Context) (*types.Account, error) {
	acct := types.NewAccount()

	// 用 env 注入一个可用余额，便于 dry-run/测试策略时展示账户估值等信息
	if v := strings.TrimSpace(os.Getenv(envBalanceUSDC)); v != "" {
		if fp, err := fixedpoint.NewFromString(v); err == nil {
			acct.UpdateBalances(types.BalanceMap{
				"USDC": types.Balance{Currency: "USDC", Available: fp},
			})
		}
	}

	acct.HasFeeRate = true
	acct.MakerFeeRate = fixedpoint.Zero
	acct.TakerFeeRate = fixedpoint.Zero
	return acct, nil
}

func (e *Exchange) QueryAccountBalances(ctx context.Context) (types.BalanceMap, error) {
	acct, err := e.QueryAccount(ctx)
	if err != nil {
		return nil, err
	}
	return acct.Balances(), nil
}

func (e *Exchange) SubmitOrder(ctx context.Context, order types.SubmitOrder) (createdOrder *types.Order, err error) {
	// 默认 dry-run：只在内存里创建订单，便于先把策略跑通。
	dryRun := true
	if v := strings.TrimSpace(os.Getenv(envDryRun)); v != "" {
		// 支持 0/1, true/false
		if b, err2 := strconv.ParseBool(v); err2 == nil {
			dryRun = b
		}
	}

	if !dryRun {
		// TODO: 在这里实现真实的 Polymarket 下单。
		// 需要明确：CLOB endpoint、鉴权方式（API key/签名）、market token id 的映射（LocalSymbol）等。
		return nil, fmt.Errorf("polymarket: real trading is not implemented yet; set %s=true to use dry-run", envDryRun)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	now := types.Time(time.Now())
	oid := e.nextOrderID
	e.nextOrderID++

	created := &types.Order{
		SubmitOrder:       order,
		Exchange:          types.ExchangePolymarket,
		OrderID:           oid,
		Status:            types.OrderStatusNew,
		ExecutedQuantity:  fixedpoint.Zero,
		IsWorking:         true,
		CreationTime:      now,
		UpdateTime:        now,
		OriginalStatus:    "NEW",
		IsFutures:         false,
		IsMargin:          false,
		IsIsolated:        false,
	}

	e.orders[oid] = created

	logrus.WithFields(created.LogFields()).Infof("polymarket(dry-run) order created: %s", created.String())
	return created, nil
}

func (e *Exchange) QueryOpenOrders(ctx context.Context, symbol string) (orders []types.Order, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, o := range e.orders {
		if !o.IsWorking {
			continue
		}
		if symbol != "" && o.Symbol != symbol {
			continue
		}
		orders = append(orders, *o)
	}
	return orders, nil
}

func (e *Exchange) CancelOrders(ctx context.Context, orders ...types.Order) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := types.Time(time.Now())
	for _, o := range orders {
		if existing, ok := e.orders[o.OrderID]; ok {
			existing.IsWorking = false
			existing.Status = types.OrderStatusCanceled
			existing.OriginalStatus = "CANCELED"
			existing.UpdateTime = now
		}
	}
	return nil
}

func loadMarketsFromEnv() (types.MarketMap, error) {
	if path := strings.TrimSpace(os.Getenv(envMarketsFile)); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("polymarket: read %s failed: %w", envMarketsFile, err)
		}
		return decodeMarketsJSON(b)
	}

	if raw := strings.TrimSpace(os.Getenv(envMarketsJSON)); raw != "" {
		return decodeMarketsJSON([]byte(raw))
	}

	return nil, nil
}

func decodeMarketsJSON(b []byte) (types.MarketMap, error) {
	// 支持两种格式：
	// 1) MarketMap: {"SYMBOL": {...}, ...}
	// 2) []Market: [{...}, {...}]（会用 Market.Symbol 做 key）
	var mm types.MarketMap
	if err := json.Unmarshal(b, &mm); err == nil && len(mm) > 0 {
		return mm, nil
	}

	var arr []types.Market
	if err := json.Unmarshal(b, &arr); err != nil {
		return nil, fmt.Errorf("polymarket: decode markets json failed: %w", err)
	}

	out := make(types.MarketMap, len(arr))
	for _, m := range arr {
		if m.Symbol == "" {
			return nil, fmt.Errorf("polymarket: market symbol is empty in json")
		}
		out[m.Symbol] = m
	}
	return out, nil
}

func defaultExampleMarkets() types.MarketMap {
	// 这是用于示例策略（BTC 15m up/down）跑通框架的默认 market。
	// LocalSymbol 目前预留给“Polymarket tokenId/marketId”等内部映射。
	return types.MarketMap{
		"PM_BTC_15M_UP_YES_USDC": {
			Symbol:          "PM_BTC_15M_UP_YES_USDC",
			LocalSymbol:     "PM_BTC_15M_UP_YES_USDC",
			BaseCurrency:    "PM_BTC_15M_UP_YES",
			QuoteCurrency:   "USDC",
			PricePrecision:  4,
			VolumePrecision: 2,
			QuotePrecision:  2,
			// 概率价格（0~1）常用 0.0001 tick；这里只是示例
			TickSize:   fixedpoint.NewFromFloat(0.0001),
			StepSize:   fixedpoint.NewFromFloat(0.01),
			MinNotional: fixedpoint.NewFromFloat(1),
			MinQuantity: fixedpoint.NewFromFloat(1),
		},
		"PM_BTC_15M_UP_NO_USDC": {
			Symbol:          "PM_BTC_15M_UP_NO_USDC",
			LocalSymbol:     "PM_BTC_15M_UP_NO_USDC",
			BaseCurrency:    "PM_BTC_15M_UP_NO",
			QuoteCurrency:   "USDC",
			PricePrecision:  4,
			VolumePrecision: 2,
			QuotePrecision:  2,
			TickSize:        fixedpoint.NewFromFloat(0.0001),
			StepSize:        fixedpoint.NewFromFloat(0.01),
			MinNotional:     fixedpoint.NewFromFloat(1),
			MinQuantity:     fixedpoint.NewFromFloat(1),
		},
	}
}

