package handler

import (
	"net/http"
	"strconv"
	"time"

	json "binance-proxy/internal/tool"

	log "github.com/sirupsen/logrus"

	"binance-proxy/internal/service"
)

func (s *Handler) klines(w http.ResponseWriter, r *http.Request) {
	symbol := r.URL.Query().Get("symbol")
	interval := r.URL.Query().Get("interval")
	limitInt, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limitInt == 0 {
		limitInt = 500
	}

	startTimeUnix, _ := strconv.Atoi(r.URL.Query().Get("startTime"))
	startTime := time.Unix(int64(startTimeUnix/1000), 0)

	switch {
	case limitInt <= 0, limitInt > 1000,
		startTime.Unix() > 0 && startTime.Before(time.Now().Add(service.INTERVAL_2_DURATION[interval]*999*-1)),
		r.URL.Query().Get("endTime") != "",
		r.URL.Query().Get("startTime") == "0",
		symbol == "", interval == "", interval == "1d":
		log.Tracef("%s %s@%s kline proxying via REST", s.class, symbol, interval)
		s.reverseProxy(w, r)
		return
	}

	data := s.srv.Klines(symbol, interval)
	if data == nil {
		log.Tracef("%s %s@%s kline proxying via REST", s.class, symbol, interval)
		s.reverseProxy(w, r)
		return
	}
	klines := make([]interface{}, 0)
	startTimeUnixMs := startTime.Unix() * 1000
	if startTimeUnixMs == 0 && limitInt < len(data) {
		data = data[len(data)-limitInt:]
	}
	for _, v := range data {
		if len(klines) >= limitInt {
			break
		}

		if startTimeUnixMs > 0 && startTimeUnixMs > v.OpenTime {
			continue
		}

		klines = append(klines, []interface{}{
			v.OpenTime,
			v.Open,
			v.High,
			v.Low,
			v.Close,
			v.Volume,
			v.CloseTime,
			v.QuoteAssetVolume,
			v.TradeNum,
			v.TakerBuyBaseAssetVolume,
			v.TakerBuyQuoteAssetVolume,
			"0",
		})
	}

	var fakeKlineTimestampOpen int64 = 0
	if len(data) > 0 && time.Now().UnixNano()/1e6 > data[len(data)-1].CloseTime {
		fakeKlineTimestampOpen = data[len(data)-1].CloseTime + 1
		log.Tracef("%s %s@%s kline requested for %s but not yet received", s.class, symbol, interval, strconv.FormatInt(fakeKlineTimestampOpen, 10))
		if s.enableFakeKline {
			log.Tracef("%s %s@%s kline faking candle for timestamp %s", s.class, symbol, interval, strconv.FormatInt(fakeKlineTimestampOpen, 10))
			klines = append(klines, []interface{}{
				data[len(data)-1].CloseTime + 1,
				data[len(data)-1].Close,
				data[len(data)-1].Close,
				data[len(data)-1].Close,
				data[len(data)-1].Close,
				"0.0",
				data[len(data)-1].CloseTime + 1 + (data[len(data)-1].CloseTime - data[len(data)-1].OpenTime),
				"0.0",
				0,
				"0.0",
				"0.0",
				"0",
			})
			if len(klines) > limitInt {
				klines = klines[len(klines)-limitInt:]
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Data-Source", "websocket")
	j, _ := json.Marshal(klines)
	w.Write(j)
}
