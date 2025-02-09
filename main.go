package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/jessevdk/go-flags"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/webdevops/go-common/azuresdk/prometheus/tracing"

	"github.com/webdevopos/azure-k8s-autopilot/autopilot"
	"github.com/webdevopos/azure-k8s-autopilot/config"
)

const (
	Author = "webdevops.io"

	UserAgent = "azure-k8s-autopilot/"
)

var (
	argparser *flags.Parser
	Opts      config.Opts

	// Git version information
	gitCommit = "<unknown>"
	gitTag    = "<unknown>"
)

func main() {
	initArgparser()
	initLogger()

	logger.Infof("starting azure-k8s-autopilot v%s (%s; %s; by %v)", gitTag, gitCommit, runtime.Version(), Author)
	logger.Info(string(Opts.GetJson()))
	initSystem()

	pilot := autopilot.AzureK8sAutopilot{
		Config:    Opts,
		UserAgent: UserAgent + gitTag,
		Logger:    logger,
	}
	pilot.Init()
	pilot.Start()

	logger.Infof("starting http server on %s", Opts.Server.Bind)
	startHttpServer()

	termChan := make(chan os.Signal, 1)
	signal.Notify(termChan, syscall.SIGINT, syscall.SIGTERM) //nolint:staticcheck
	<-termChan
	logger.Info("shutdown signal received, trying to stop")
	pilot.Stop()
	logger.Info("finished, terminating now")
}

// init argparser and parse/validate arguments
func initArgparser() {
	argparser = flags.NewParser(&Opts, flags.Default)
	_, err := argparser.Parse()

	// check if there is an parse error
	if err != nil {
		var flagsErr *flags.Error
		if ok := errors.As(err, &flagsErr); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		} else {
			fmt.Println(err)
			fmt.Println()
			argparser.WriteHelp(os.Stdout)
			os.Exit(1)
		}
	}

	if val := os.Getenv("DRAIN_DELETE_LOCAL_DATA"); val != "" {
		panic("env var DRAIN_DELETE_LOCAL_DATA is deprecated, please use DRAIN_DELETE_EMPTYDIR_DATA")
	}
}

// start and handle prometheus handler
func startHttpServer() {
	mux := http.NewServeMux()

	// healthz
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, "Ok"); err != nil {
			logger.Error(err)
		}
	})

	// readyz
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, "Ok"); err != nil {
			logger.Error(err)
		}
	})

	mux.Handle("/metrics", tracing.RegisterAzureMetricAutoClean(promhttp.Handler()))

	go func() {
		srv := &http.Server{
			Addr:         Opts.Server.Bind,
			Handler:      mux,
			ReadTimeout:  Opts.Server.ReadTimeout,
			WriteTimeout: Opts.Server.WriteTimeout,
		}
		logger.Fatal(srv.ListenAndServe())
	}()
}
