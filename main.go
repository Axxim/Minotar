package main

import (
	"fmt"
	"github.com/gorilla/mux"
	"github.com/op/go-logging"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime/debug"
)

const (
	DefaultSize = uint(180)
	MaxSize     = uint(300)
	MinSize     = uint(8)

	SkinCache

	Minutes            uint = 60
	Hours                   = 60 * Minutes
	Days                    = 24 * Hours
	TimeoutActualSkin       = 2 * Days
	TimeoutFailedFetch      = 15 * Minutes

	MinotarVersion = "2.6"
)

var (
	config = &Configuration{}
	cache  Cache
	stats  *StatusCollector
)

var log = logging.MustGetLogger("imgd")
var format = "[%{time:15:04:05.000000}] %{level:.4s} %{message}"

func setupConfig() {
	err := config.load()
	if err != nil {
		fmt.Printf("Error loading config: %s\n", err)
		return
	}
}

func setupCache() {
	cache = MakeCache(config.Server.Cache)
	cache.setup()
}

func setupLog(logBackend *logging.LogBackend) {
	logging.SetBackend(logBackend)
	logging.SetFormatter(logging.MustStringFormatter(format))
}

func startServer() {
	r := Router{Mux: mux.NewRouter()}
	r.Bind()
	http.Handle("/", r.Mux)
	err := http.ListenAndServe(config.Server.Address, nil)
	log.Critical(err.Error())
}

func main() {
	logBackend := logging.NewLogBackend(os.Stdout, "", 0)
	debug.SetGCPercent(10)
	stats = MakeStatsCollector()
	setupLog(logBackend)
	setupConfig()
	setupCache()
	startServer()
}
