package autopilot

import (
	"encoding/json"
	"fmt"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/containrrr/shoutrrr"
	"github.com/patrickmn/go-cache"
	"github.com/prometheus/client_golang/prometheus"
	cron "github.com/robfig/cron/v3"
	log "github.com/sirupsen/logrus"
	"github.com/webdevopos/azure-k8s-autopilot/config"
	"github.com/webdevopos/azure-k8s-autopilot/k8s"
	"golang.org/x/net/context"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"strings"
	"time"
)

type (
	AzureK8sAutopilot struct {
		DryRun bool

		ctx    context.Context
		Config config.Opts

		prometheus struct {
			general struct {
				errors *prometheus.CounterVec
			}

			repair struct {
				count      *prometheus.CounterVec
				nodeStatus *prometheus.GaugeVec
				duration   *prometheus.GaugeVec
			}

			update struct {
				count    *prometheus.CounterVec
				duration *prometheus.GaugeVec
			}
		}

		azureAuthorizer autorest.Authorizer
		k8sClient       *kubernetes.Clientset

		cache          *cache.Cache
		nodeRepairLock *cache.Cache
		nodeUpdateLock *cache.Cache
	}
)

func (r *AzureK8sAutopilot) Init() {
	r.initAzure()
	r.initK8s()
	r.initMetricsRepair()
	r.initMetricsUpdate()
	r.cache = cache.New(1*time.Minute, 1*time.Minute)
	r.nodeRepairLock = cache.New(15*time.Minute, 1*time.Minute)
	r.nodeUpdateLock = cache.New(15*time.Minute, 1*time.Minute)
	r.ctx = context.Background()

	r.Config.Repair.ProvisioningStateAll = false
	for key, val := range r.Config.Repair.ProvisioningState {
		val = strings.ToLower(val)
		r.Config.Repair.ProvisioningState[key] = val

		if val == "*" {
			r.Config.Repair.ProvisioningStateAll = true
		}
	}
}

func (r *AzureK8sAutopilot) initAzure() {
	var err error

	// setup azure authorizer
	r.azureAuthorizer, err = auth.NewAuthorizerFromEnvironment()
	if err != nil {
		panic(err)
	}
}

func (r *AzureK8sAutopilot) initK8s() {
	var err error
	var config *rest.Config

	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		// KUBECONFIG
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			panic(err.Error())
		}
	} else {
		// K8S in cluster
		config, err = rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
	}

	r.k8sClient, err = kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
}

func (r *AzureK8sAutopilot) initMetricsGeneral() {
	r.prometheus.general.errors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "autopilot_errors",
			Help: "azure_k8s_autopilot error counter",
		},
		[]string{"scope"},
	)
	prometheus.MustRegister(r.prometheus.general.errors)
}

func (r *AzureK8sAutopilot) initMetricsRepair() {
	r.prometheus.repair.nodeStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "autopilot_repair_node_status",
			Help: "azure_k8s_autopilot repair node status",
		},
		[]string{"nodeName"},
	)
	prometheus.MustRegister(r.prometheus.repair.nodeStatus)

	r.prometheus.repair.count = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "autopilot_repair_count",
			Help: "azure_k8s_autopilot repair counter",
		},
		[]string{},
	)
	prometheus.MustRegister(r.prometheus.repair.count)

	r.prometheus.repair.duration = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "autopilot_repair_duration",
			Help: "azure_k8s_autopilot repair duration",
		},
		[]string{},
	)
	prometheus.MustRegister(r.prometheus.repair.duration)
}

func (r *AzureK8sAutopilot) initMetricsUpdate() {
	r.prometheus.update.count = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "autopilot_update_count",
			Help: "azure_k8s_autopilot update counter",
		},
		[]string{},
	)
	prometheus.MustRegister(r.prometheus.update.count)

	r.prometheus.update.duration = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "autopilot_update_duration",
			Help: "azure_k8s_autopilot update duration",
		},
		[]string{},
	)
	prometheus.MustRegister(r.prometheus.update.duration)
}

func (r *AzureK8sAutopilot) Run() {
	log.Infof("starting cluster check loop")

	config, _ := json.Marshal(r.Config)
	log.Info(string(config))

	cron := cron.New(
		cron.WithChain(
			cron.SkipIfStillRunning(
				cron.PrintfLogger(
					log.StandardLogger(),
				),
			),
		),
	)

	// repair job
	if r.Config.Repair.Crontab != "" {
		_, err := cron.AddFunc(r.Config.Repair.Crontab, func() {
			contextLogger := log.WithField("job", "repair")

			// concurrency repair limit
			if r.Config.Repair.Limit > 0 && r.nodeRepairLock.ItemCount() >= r.Config.Repair.Limit {
				contextLogger.Infof("concurrent repair limit reached, skipping run")
			} else {
				start := time.Now()
				contextLogger.Infoln("starting repair check")
				r.repairRun(contextLogger)
				runtime := time.Now().Sub(start)
				r.prometheus.repair.duration.WithLabelValues().Set(runtime.Seconds())
				contextLogger.WithField("duration", runtime.String()).Infof("finished after %s", runtime.String())
			}
		})
		if err != nil {
			log.Panic(err)
		}
	}

	// upgrade job
	if r.Config.Update.Crontab != "" {
		_, err := cron.AddFunc(r.Config.Update.Crontab, func() {
			contextLogger := log.WithField("job", "update")

			// concurrency repair limit
			if r.Config.Update.Limit > 0 && r.nodeUpdateLock.ItemCount() >= r.Config.Update.Limit {
				contextLogger.Infof("concurrent update limit reached, skipping run")
			} else {
				contextLogger.Infoln("starting update check")
				start := time.Now()
				r.updateRun(contextLogger)
				runtime := time.Now().Sub(start)
				r.prometheus.update.duration.WithLabelValues().Set(runtime.Seconds())
				contextLogger.WithField("duration", runtime.String()).Infof("finished after %s", runtime.String())
			}
		})
		if err != nil {
			log.Panic(err)
		}
	}

	cron.Start()
}

func (r *AzureK8sAutopilot) sendNotificationf(message string, args ...interface{}) {
	r.sendNotification(fmt.Sprintf(message, args...))
}

func (r *AzureK8sAutopilot) sendNotification(message string) {
	for _, url := range r.Config.Notification {
		if err := shoutrrr.Send(url, message); err != nil {
			log.Errorf("Unable to send shoutrrr notification: %v", err.Error())
		}
	}
}

func (r *AzureK8sAutopilot) syncNodeLockCache(contextLogger *log.Entry, nodeList *k8s.NodeList, annotationName string, cacheLock *cache.Cache) {
	// lock cache clear
	contextLogger.Debugf("sync node lock cache for annotation %s", annotationName)
	cacheLock.Flush()

	for _, node := range nodeList.GetNodes() {
		if lockDuration, exists := r.k8sGetNodeLockAnnotation(node, annotationName); exists {
			// check if annotation is valid and if node status is ok
			if lockDuration == nil || lockDuration.Seconds() <= 0 {
				// remove annotation
				contextLogger.Debugf("removing lock annotation \"%s\" from node %s", annotationName, node.Name)
				if err := r.k8sRemoveNodeLockAnnotation(node, annotationName); err != nil {
					contextLogger.Error(err)
				}
				continue
			}

			// add to lock cache
			contextLogger.Debugf("found existing lock \"%s\" for node %s, duration: %s", annotationName, node.Name, lockDuration.String())
			cacheLock.Add(node.Name, true, *lockDuration) //nolint:golint,errcheck
		}
	}
}

func (r *AzureK8sAutopilot) autoUncordonExpiredNodes(contextLogger *log.Entry, nodeList *k8s.NodeList, annotationName string) {
	// lock cache clear
	contextLogger.Debugf("checking expired but still cordoned nodes for annotation \"%s\"", annotationName)

	for _, node := range nodeList.GetNodes() {
		if lockDuration, exists := r.k8sGetNodeLockAnnotation(node, annotationName); exists {
			// check if annotation is valid and if node status is ok
			if lockDuration == nil || lockDuration.Seconds() <= 0 {
				// check if node is cordoned
				if node.Spec.Unschedulable {
					contextLogger.Infof("node %s is still cordoned, uncording it", node.Name)

					// uncordon node
					if err := r.k8sUncordonNode(contextLogger, node); err != nil {
						contextLogger.Errorf("node %s failed to uncordon: %v", node.Name, err)
					}
				}
			}
		}
	}
}
