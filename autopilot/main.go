package autopilot

import (
	"fmt"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/containrrr/shoutrrr"
	"github.com/operator-framework/operator-sdk/pkg/leader"
	"github.com/patrickmn/go-cache"
	"github.com/prometheus/client_golang/prometheus"
	cron "github.com/robfig/cron/v3"
	log "github.com/sirupsen/logrus"
	"github.com/webdevopos/azure-k8s-autopilot/config"
	"github.com/webdevopos/azure-k8s-autopilot/k8s"
	"golang.org/x/net/context"
	"k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"strings"
	"sync"
	"time"
)

type (
	AzureK8sAutopilot struct {
		DryRun bool

		ctx    context.Context
		Config config.Opts

		cron struct {
			repair *cron.Cron
			update *cron.Cron
		}

		wg sync.WaitGroup

		prometheus struct {
			general struct {
				errors         *prometheus.CounterVec
				candidateNodes *prometheus.GaugeVec
				failedNodes    *prometheus.GaugeVec
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

		cache *cache.Cache

		nodeList *k8s.NodeList

		repair struct {
			nodeLock *cache.Cache
		}

		update struct {
			nodeLock *cache.Cache
		}
	}
)

func (r *AzureK8sAutopilot) Init() {
	r.initAzure()
	r.initK8s()
	r.initMetricsGeneral()
	r.initMetricsRepair()
	r.initMetricsUpdate()
	r.cache = cache.New(1*time.Minute, 1*time.Minute)
	r.repair.nodeLock = cache.New(15*time.Minute, 1*time.Minute)
	r.update.nodeLock = cache.New(15*time.Minute, 1*time.Minute)
	r.ctx = context.Background()

	r.nodeList = &k8s.NodeList{
		NodeLabelSelector: r.Config.K8S.NodeLabelSelector,
		AzureAuthorizer:   r.azureAuthorizer,
		Client:            r.k8sClient,
	}

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

	r.prometheus.general.failedNodes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "autopilot_failed_nodes",
			Help: "azure_k8s_autopilot count of nodes which are failed",
		},
		[]string{"type"},
	)
	prometheus.MustRegister(r.prometheus.general.failedNodes)

	r.prometheus.general.candidateNodes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "autopilot_candidate_nodes",
			Help: "azure_k8s_autopilot count of nodes which are considred as candidates",
		},
		[]string{"type"},
	)

	prometheus.MustRegister(r.prometheus.general.candidateNodes)
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

func (r *AzureK8sAutopilot) Start() {
	go func() {
		r.leaderElect()
		log.Infof("starting autopilot")

		r.nodeList.Start()

		if r.Config.Repair.Crontab != "" {
			r.startAutopilotRepair()
		}

		if r.Config.Update.Crontab != "" {
			r.startAutopilotUpdate()
		}
	}()
}

func (r *AzureK8sAutopilot) Stop() {
	if r.cron.repair != nil {
		r.cron.repair.Stop()
	}

	if r.cron.update != nil {
		r.cron.update.Stop()
	}

	r.wg.Wait()
	r.nodeList.Stop()
}

func (r *AzureK8sAutopilot) startAutopilotRepair() {
	// repair job
	r.cron.repair = cron.New(
		cron.WithChain(
			cron.SkipIfStillRunning(
				cron.PrintfLogger(
					log.StandardLogger(),
				),
			),
		),
	)

	_, err := r.cron.repair.AddFunc(r.Config.Repair.Crontab, func() {
		r.wg.Add(1)
		defer r.wg.Done()

		contextLogger := log.WithField("job", "repair")

		// concurrency repair limit
		if r.Config.Repair.Limit > 0 && r.repair.nodeLock.ItemCount() >= r.Config.Repair.Limit {
			contextLogger.Infof("concurrent repair limit reached, skipping run")
		} else {
			start := time.Now()
			contextLogger.Infoln("starting repair check")
			r.repairRun(contextLogger)
			runtime := time.Now().Sub(start)
			r.prometheus.repair.duration.WithLabelValues().Set(runtime.Seconds())
			contextLogger.WithField("duration", runtime.Seconds()).Infof("finished after %s", runtime.String())
		}
	})
	if err != nil {
		log.Panic(err)
	}

	r.cron.repair.Start()
}

func (r *AzureK8sAutopilot) startAutopilotUpdate() {
	r.cron.update = cron.New(
		cron.WithChain(
			cron.SkipIfStillRunning(
				cron.PrintfLogger(
					log.StandardLogger(),
				),
			),
		),
	)

	_, err := r.cron.update.AddFunc(r.Config.Update.Crontab, func() {
		r.wg.Add(1)
		defer r.wg.Done()

		contextLogger := log.WithField("job", "update")

		// concurrency repair limit
		if r.Config.Update.Limit > 0 && r.update.nodeLock.ItemCount() >= r.Config.Update.Limit {
			contextLogger.Infof("concurrent update limit reached, skipping run")
		} else {
			contextLogger.Infoln("starting update check")
			start := time.Now()
			r.updateRun(contextLogger)
			runtime := time.Now().Sub(start)
			r.prometheus.update.duration.WithLabelValues().Set(runtime.Seconds())
			contextLogger.WithField("duration", runtime.Seconds()).Infof("finished after %s", runtime.String())
		}
	})
	if err != nil {
		log.Panic(err)
	}

	r.cron.update.Start()
}

func (r *AzureK8sAutopilot) leaderElect() {
	if r.Config.Lease.Enabled {
		log.Info("trying to become leader")
		if r.Config.Instance.Pod != nil && os.Getenv("POD_NAME") == "" {
			err := os.Setenv("POD_NAME", *r.Config.Instance.Pod)
			if err != nil {
				log.Panic(err)
			}
		}

		time.Sleep(15 * time.Second)
		err := leader.Become(r.ctx, r.Config.Lease.Name)
		if err != nil {
			log.Error(err, "Failed to retry for leader lock")
			os.Exit(1)
		}
		log.Info("aquired leader lock, continue")
	}
}

func (r *AzureK8sAutopilot) checkSelfEviction(node *k8s.Node) bool {
	if r.Config.Instance.Nodename == nil || r.Config.Instance.Namespace == nil || r.Config.Instance.Pod == nil {
		return false
	}

	if *r.Config.Instance.Nodename == node.Name {
		log.Infof("azure-k8s-autopilot is running on an affected node, self evicting")
		if r.cron.repair != nil {
			r.cron.repair.Stop()
		}

		if r.cron.repair != nil {
			r.cron.update.Stop()
		}

		eviction := v1beta1.Eviction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      *r.Config.Instance.Pod,
				Namespace: *r.Config.Instance.Namespace,
			},
		}
		err := r.k8sClient.CoreV1().Pods(*r.Config.Instance.Namespace).Evict(r.ctx, &eviction)
		if err != nil {
			log.Errorf("unable to evict instance: %v", err)
		}
		return true
	}

	return false
}

func (r *AzureK8sAutopilot) sendNotificationf(message string, args ...interface{}) {
	r.sendNotification(fmt.Sprintf(message, args...))
}

func (r *AzureK8sAutopilot) sendNotification(message string) {
	sender, err := shoutrrr.CreateSender(r.Config.Notification...)
	if err != nil {
		log.Errorf("Unable to send shoutrrr notification: %v", err.Error())
	}

	if sender != nil {
		sender.Send(message, nil)
	}
}

func (r *AzureK8sAutopilot) syncNodeLockCache(contextLogger *log.Entry, nodeList []*k8s.Node, annotationName string, cacheLock *cache.Cache) {
	// lock cache clear
	contextLogger.Debugf("sync node lock cache for annotation %s", annotationName)
	cacheLock.Flush()

	for _, node := range nodeList {
		if lockDuration, exists := node.AnnotationLockCheck(annotationName); exists {
			// check if annotation is valid and if node status is ok
			if lockDuration == nil || lockDuration.Seconds() <= 0 {
				// remove annotation
				contextLogger.Debugf("removing lock annotation \"%s\" from node %s", annotationName, node.Name)
				if err := node.AnnotationRemove(annotationName); err != nil {
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

func (r *AzureK8sAutopilot) autoUncordonExpiredNodes(contextLogger *log.Entry, nodeList []*k8s.Node, annotationName string) {
	// lock cache clear
	contextLogger.Debugf("checking expired but still cordoned nodes for annotation \"%s\"", annotationName)

	for _, node := range nodeList {
		if lockDuration, exists := node.AnnotationLockCheck(annotationName); exists {
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
