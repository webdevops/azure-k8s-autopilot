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
	"runtime"
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

	log.Infof("starting Azure K8S cluster autopilot v%s (%s; %s; by %v)", gitTag, gitCommit, runtime.Version(), Author)
	startAzureK8sAutorepair()

	log.Infof("starting http server on %s", opts.ServerBind)
	startHttpServer()
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
	}

	// json log format
	if opts.Logger.LogJson {
		log.SetFormatter(&log.JSONFormatter{})
	}
}

// Init and build Azure authorzier
func startAzureK8sAutorepair() {
	autorepair := autopilot.AzureK8sAutopilot{
		Config: opts,
	}
	autorepair.Init()
	autorepair.Run()
}

// start and handle prometheus handler
func startHttpServer() {
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(opts.ServerBind, nil))
}
