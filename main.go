package main

import (
	"fmt"
	"github.com/jessevdk/go-flags"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	Author  = "webdevops.io"
)

var (
	argparser *flags.Parser
	Verbose   bool
	Logger    *DaemonLogger

	// Git version information
	gitCommit = "<unknown>"
	gitTag = "<unknown>"
)

var opts struct {
	// general settings
	Verbose []bool `long:"verbose" short:"v" env:"VERBOSE"   description:"Verbose mode"`
	DryRun  bool   `long:"dry-run"           env:"DRY_RUN"   description:"Dry run (no redeploy triggered)"`

	// check settings
	RepairInterval     time.Duration `long:"repair.interval"      env:"REPAIR_INTERVAL"       description:"Duration of check run"                                   default:"30s"`
	RepairWaitDuration time.Duration `long:"repair.waitduration"  env:"REPAIR_WAIT_DURATION"  description:"Duration to wait when redeploy will be triggered"        default:"10m"`
	RepairLimit        int64         `long:"repair.concurrency"   env:"REPAIR_CONCURRENCY"    description:"How many VMs should be redeployed concurrently"          default:"1"`
	RepairLockDuration time.Duration `long:"repair.lockduration"  env:"REPAIR_LOCK_DURATION"  description:"Duration how long should be waited for another redeploy" default:"15m"`

	// server settings
	ServerBind string `long:"bind" env:"SERVER_BIND"  description:"Server address"  default:":8080"`
}

func main() {
	initArgparser()

	// set verbosity
	Verbose = len(opts.Verbose) >= 1

	Logger = NewLogger(log.Lshortfile, Verbose)
	defer Logger.Close()

	// set verbosity
	Verbose = len(opts.Verbose) >= 1

	Logger.Infof("Init Azure K8S cluster AutoRepair v%s (%s; by %v)", gitTag, gitCommit, Author)
	startAzureK8sAutorepair()

	Logger.Infof("Starting http server on %s", opts.ServerBind)
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
}

// Init and build Azure authorzier
func startAzureK8sAutorepair() {
	autorepair := K8sAutoRepair{}
	autorepair.Interval = &opts.RepairInterval
	autorepair.WaitDuration = &opts.RepairWaitDuration
	autorepair.LockDuration = &opts.RepairLockDuration
	autorepair.Limit = opts.RepairLimit
	autorepair.DryRun = opts.DryRun
	autorepair.Init()
	autorepair.Run()
}

// start and handle prometheus handler
func startHttpServer() {
	http.Handle("/metrics", promhttp.Handler())
	Logger.Fatal(http.ListenAndServe(opts.ServerBind, nil))
}
