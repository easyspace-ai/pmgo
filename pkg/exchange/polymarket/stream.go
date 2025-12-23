package polymarket

import (
	"context"

	"github.com/c9s/bbgo/pkg/types"
)

// Stream 是一个“最小可用”的 stream：
// - 满足 bbgo 的 Stream 接口要求
// - Connect 不会真正建立 websocket（避免因为 Polymarket websocket 细节未知而导致启动失败）
//
// 这对“用 Binance 做行情源、用 Polymarket 做交易端”的跨交易所策略足够用。
// 如果你希望从 Polymarket 拉盘口/成交/价格，可以再在这里接入真实 websocket 并派发事件。
type Stream struct {
	types.StandardStream
}

func NewStream() *Stream {
	ss := types.NewStandardStream()
	return &Stream{StandardStream: ss}
}

func (s *Stream) Connect(ctx context.Context) error {
	// 不进行真实连接，但要让框架认为“已连接”，避免 connectivity 一直处于 disconnected。
	s.EmitConnect()
	s.EmitStart()
	return nil
}

func (s *Stream) Close() error {
	s.EmitDisconnect()
	return nil
}

