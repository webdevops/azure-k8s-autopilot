package main

import (
	"fmt"
	"github.com/jessevdk/go-flags"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/webdevopos/azure-k8s-autopilot/autopilot"
	"github.com/webdevopos/azure-k8s-autopilot/config"
	"net/http"
	"os"
	"os/signal"
	"path"
	"runtime"
	"strings"
	"syscall"
)

const (
	Author = "webdevops.io"
)

var (
	argparser *flags.Parser

	// Git version information
	gitCommit = "<unknown>"
	gitTag    = "<unknown>"
)

var opts = config.Opts{}

func main() {
	initArgparser()

	log.Infof("starting azure-k8s-autopilot v%s (%s; %s; by %v)", gitTag, gitCommit, runtime.Version(), Author)
	log.Info(string(opts.GetJson()))

	pilot := autopilot.AzureK8sAutopilot{
		Config: opts,
	}
	pilot.Init()
	pilot.Start()

	log.Infof("starting http server on %s", opts.ServerBind)
	startHttpServer()

	termChan := make(chan os.Signal)
	signal.Notify(termChan, syscall.SIGINT, syscall.SIGTERM)
	<-termChan
	log.Info("shutdown signal received, trying to stop")
	pilot.Stop()
	log.Info("finished, terminating now")
}

// init argparser and parse/validate arguments
func initArgparser() {
	argparser = flags.NewParser(&opts, flags.Default)
	_, err := argparser.Parse()

	// check if there is an parse error
	if err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		} else {
			fmt.Println(err)
			fmt.Println()
			argparser.WriteHelp(os.Stdout)
			os.Exit(1)
		}
	}

	// verbose level
	if opts.Logger.Verbose {
		log.SetLevel(log.DebugLevel)
	}

	// debug level
	if opts.Logger.Debug {
		log.SetReportCaller(true)
		log.SetLevel(log.TraceLevel)
		log.SetFormatter(&log.TextFormatter{
			CallerPrettyfier: func(f *runtime.Frame) (string, string) {
				s := strings.Split(f.Function, ".")
				funcName := s[len(s)-1]
				return funcName, fmt.Sprintf("%s:%d", path.Base(f.File), f.Line)
			},
		})
	}

	// json log format
	if opts.Logger.LogJson {
		log.SetReportCaller(true)
		log.SetFormatter(&log.JSONFormatter{
			DisableTimestamp: true,
			CallerPrettyfier: func(f *runtime.Frame) (string, string) {
				s := strings.Split(f.Function, ".")
				funcName := s[len(s)-1]
				return funcName, fmt.Sprintf("%s:%d", path.Base(f.File), f.Line)
			},
		})
	}
}

// start and handle prometheus handler
func startHttpServer() {
	// healthz
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, "Ok"); err != nil {
			log.Error(err)
		}
	})

	http.Handle("/metrics", promhttp.Handler())
	go func() {
		log.Fatal(http.ListenAndServe(opts.ServerBind, nil))
	}()
}
