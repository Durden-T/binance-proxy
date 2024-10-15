package main

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adshao/go-binance/v2/futures"

	"binance-proxy/internal/handler"
	"binance-proxy/internal/service"

	"github.com/jessevdk/go-flags"
	log "github.com/sirupsen/logrus"
)

func startProxy(ctx context.Context, port int, class service.Class, alwaysshowforwards bool) {
	mux := http.NewServeMux()
	address := fmt.Sprintf(":%d", port)
	mux.HandleFunc("/", handler.NewHandler(ctx, class, alwaysshowforwards))

	log.Infof("%s websocket proxy starting on port %d.", class, port)
	if err := http.ListenAndServe(address, mux); err != nil {
		log.Fatalf("%s websocket proxy start failed (error: %s).", class, err)
	}
}

func handleSignal() {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	for s := range signalChan {
		switch s {
		case syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
			cancel()
		}
	}
}

type Config struct {
	Verbose            []bool `short:"v" long:"verbose" env:"BPX_VERBOSE" description:"Verbose output (increase with -vv)"`
	SpotAddress        int    `short:"p" long:"port-spot" env:"BPX_PORT_SPOT" description:"Port to which to bind for SPOT markets" default:"8090"`
	FuturesAddress     int    `short:"t" long:"port-futures" env:"BPX_PORT_FUTURES" description:"Port to which to bind for FUTURES markets" default:"8091"`
	DisableSpot        bool   `short:"s" long:"disable-spot" env:"BPX_DISABLE_SPOT" description:"Disable proxying spot markets"`
	DisableFutures     bool   `short:"f" long:"disable-futures" env:"BPX_DISABLE_FUTURES" description:"Disable proxying futures markets"`
	AlwaysShowForwards bool   `short:"a" long:"always-show-forwards" env:"BPX_ALWAYS_SHOW_FORWARDS" description:"Always show requests forwarded via REST even if verbose is disabled"`
}

var (
	config      Config
	parser             = flags.NewParser(&config, flags.Default)
	Version     string = "develop"
	Buildtime   string = "undefined"
	ctx, cancel        = context.WithCancel(context.Background())
)

func init() {
	t := http.DefaultTransport.(*http.Transport)
	t.MaxIdleConnsPerHost = 200
	t.MaxIdleConns = 200
	futures.WebsocketKeepalive = true
	go func() {
		tk := time.NewTicker(time.Minute)
		defer tk.Stop()

		ctx := context.Background()
		for range tk.C {
			_, _ = futures.NewClient("", "").NewSetServerTimeService().Do(ctx)
		}
	}()
}

func main() {

	log.SetFormatter(&log.TextFormatter{
		DisableColors: true,
		FullTimestamp: true,
	})

	log.Infof("Binance proxy version %s, build time %s", Version, Buildtime)

	if _, err := parser.Parse(); err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		} else {
			log.Fatalf("%s - %s", err, flagsErr.Type)
		}
	}

	if len(config.Verbose) >= 2 {
		log.SetLevel(log.TraceLevel)
	} else if len(config.Verbose) == 1 {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	if log.GetLevel() > log.InfoLevel {
		log.Infof("Set level to %s", log.GetLevel())
	}

	if config.DisableSpot && config.DisableFutures {
		log.Fatal("can't start if both SPOT and FUTURES are disabled!")
	}

	if config.AlwaysShowForwards {
		log.Infof("Always show forwards is enabled, all API requests, that can't be served from websockets cached will be logged.")
	}

	go handleSignal()

	if !config.DisableSpot {
		go startProxy(ctx, config.SpotAddress, service.SPOT, config.AlwaysShowForwards)
	}
	if !config.DisableFutures {
		go startProxy(ctx, config.FuturesAddress, service.FUTURES, config.AlwaysShowForwards)
	}
	<-ctx.Done()

	log.Info("SIGINT received, aborting ...")
}
