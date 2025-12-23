package types

func ExchangeFooterIcon(exName ExchangeName) string {
	footerIcon := ""

	switch exName {
	case ExchangeBinance:
		footerIcon = "https://bin.bnbstatic.com/static/images/common/favicon.ico"
	case ExchangePolymarket:
		// 目前先用官网 favicon；后续可换成更稳定的静态资源
		footerIcon = "https://polymarket.com/favicon.ico"
	}

	return footerIcon
}
