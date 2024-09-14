package autopilot

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/containrrr/shoutrrr"
	"github.com/operator-framework/operator-lib/leader"
	"github.com/patrickmn/go-cache"
	"github.com/prometheus/client_golang/prometheus"
	cron "github.com/robfig/cron/v3"
	"github.com/webdevops/go-common/azuresdk/armclient"
	"github.com/webdevops/go-common/azuresdk/azidentity"
	"go.uber.org/zap"
	"golang.org/x/net/context"
	"k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/zapr"

	"github.com/webdevopos/azure-k8s-autopilot/config"
	"github.com/webdevopos/azure-k8s-autopilot/k8s"
)

type (
	AzureK8sAutopilot struct {
		ctx    context.Context
		Config config.Opts

		UserAgent string

		Logger *zap.SugaredLogger

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

		azureClient *armclient.ArmClient
		k8sClient   *kubernetes.Clientset

		cache *cache.Cache

		nodeList *k8s.NodeList

		repair struct {
			nodeLock *cache.Cache
		}

		update struct {
			nodeLock *cache.Cache
		}
	}

	AzureK8sAutopilotLogger struct {
		logger *zap.SugaredLogger
	}
)

func (l *AzureK8sAutopilotLogger) Printf(msg string, args ...any) {
	l.logger.Infof(msg, args)
}

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
		AzureClient:       r.azureClient,
		Client:            r.k8sClient,
		UserAgent:         r.UserAgent,
		Logger:            r.Logger,
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

	if r.Config.Azure.Environment != nil {
		if err := os.Setenv(azidentity.EnvAzureEnvironment, *r.Config.Azure.Environment); err != nil {
			r.Logger.Warnf(`unable to set envvar "%s": %v`, azidentity.EnvAzureEnvironment, err.Error())
		}
	}

	r.azureClient, err = armclient.NewArmClientFromEnvironment(r.Logger)
	if err != nil {
		r.Logger.Panic(err.Error())
	}

	r.azureClient.SetUserAgent(r.UserAgent)

	if err := r.azureClient.Connect(); err != nil {
		r.Logger.Panic(err.Error())
	}
}

func (r *AzureK8sAutopilot) initK8s() {
	var err error
	var restConfig *rest.Config

	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		// KUBECONFIG
		restConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			panic(err.Error())
		}
	} else {
		// K8S in cluster
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
	}

	r.k8sClient, err = kubernetes.NewForConfig(restConfig)
	if err != nil {
		panic(err.Error())
	}

	log.SetLogger(zapr.NewLogger(r.Logger.Desugar()))
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
		r.Logger.Infof("starting autopilot")

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
					&AzureK8sAutopilotLogger{logger: r.Logger},
				),
			),
		),
	)

	_, err := r.cron.repair.AddFunc(r.Config.Repair.Crontab, func() {
		r.wg.Add(1)
		defer r.wg.Done()

		contextLogger := r.Logger.With(zap.String("job", "repair"))

		// update node lock cache
		r.syncNodeLockCache(contextLogger, r.nodeList.NodeList(), r.Config.Repair.NodeLockAnnotation, r.repair.nodeLock)

		// concurrency repair limit
		if r.Config.Repair.Limit > 0 && r.repair.nodeLock.ItemCount() >= r.Config.Repair.Limit {
			contextLogger.Infof("concurrent repair limit reached, skipping run")
		} else {
			start := time.Now()
			contextLogger.Infoln("starting repair check")
			r.repairRun(contextLogger)
			runtime := time.Since(start)
			r.prometheus.repair.duration.WithLabelValues().Set(runtime.Seconds())
			contextLogger.With(zap.Float64("duration", runtime.Seconds())).Infof("finished after %s", runtime.String())
		}
	})
	if err != nil {
		r.Logger.Panic(err)
	}

	r.cron.repair.Start()
}

func (r *AzureK8sAutopilot) startAutopilotUpdate() {
	r.cron.update = cron.New(
		cron.WithChain(
			cron.SkipIfStillRunning(
				cron.PrintfLogger(
					&AzureK8sAutopilotLogger{logger: r.Logger},
				),
			),
		),
	)

	_, err := r.cron.update.AddFunc(r.Config.Update.Crontab, func() {
		r.wg.Add(1)
		defer r.wg.Done()

		contextLogger := r.Logger.With(zap.String("job", "update"))

		// automatic remove cordon state on nodes
		r.autoUncordonExpiredNodes(contextLogger, r.nodeList.NodeList(), r.Config.Update.NodeLockAnnotation)

		// update node lock cache
		r.syncNodeLockCache(contextLogger, r.nodeList.NodeList(), r.Config.Update.NodeLockAnnotation, r.update.nodeLock)

		// concurrency repair limit
		if r.Config.Update.Limit > 0 && r.update.nodeLock.ItemCount() >= r.Config.Update.Limit {
			contextLogger.Infof("concurrent update limit reached, skipping run")
		} else {
			contextLogger.Infoln("starting update check")
			start := time.Now()
			r.updateRun(contextLogger)
			runtime := time.Since(start)
			r.prometheus.update.duration.WithLabelValues().Set(runtime.Seconds())
			contextLogger.With(zap.Float64("duration", runtime.Seconds())).Infof("finished after %s", runtime.String())
		}
	})
	if err != nil {
		r.Logger.Panic(err)
	}

	r.cron.update.Start()
}

func (r *AzureK8sAutopilot) leaderElect() {
	if r.Config.Lease.Enabled {
		r.Logger.Info("trying to become leader")
		if r.Config.Instance.Pod != nil && os.Getenv("POD_NAME") == "" {
			err := os.Setenv("POD_NAME", *r.Config.Instance.Pod)
			if err != nil {
				r.Logger.Panic(err)
			}
		}

		time.Sleep(15 * time.Second)
		err := leader.Become(r.ctx, r.Config.Lease.Name)
		if err != nil {
			r.Logger.Error(err, "Failed to retry for leader lock")
			os.Exit(1)
		}
		r.Logger.Info("aquired leader lock, continue")
	}
}

func (r *AzureK8sAutopilot) checkSelfEviction(node *k8s.Node) bool {
	if r.Config.Instance.Nodename == nil || r.Config.Instance.Namespace == nil || r.Config.Instance.Pod == nil {
		return false
	}

	if *r.Config.Instance.Nodename == node.Name {
		r.Logger.Infof("azure-k8s-autopilot is running on an affected node, self evicting")
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
			r.Logger.Errorf("unable to evict instance: %v", err)
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
		r.Logger.Errorf("Unable to send shoutrrr notification: %v", err.Error())
	}

	if sender != nil {
		sender.Send(message, nil)
	}
}

func (r *AzureK8sAutopilot) syncNodeLockCache(contextLogger *zap.SugaredLogger, nodeList []*k8s.Node, annotationName string, cacheLock *cache.Cache) {
	// lock cache clear
	contextLogger.Debugf("sync node lock cache for annotation %s", annotationName)
	cacheLock.Flush()

	for _, node := range nodeList {
		if lockDuration, exists := node.AnnotationLockCheck(annotationName); exists {
			// check if annotation is valid and if node status is ok
			if lockDuration == nil || lockDuration.Seconds() <= 0 {
				// remove annotation
				contextLogger.Debugf("removing lock annotation \"%s\" from node %s", annotationName, node.Name)
				if err := node.AnnotationLockRemove(annotationName); err != nil {
					contextLogger.Error(err)
				}
				continue
			}

			// add to lock cache
			contextLogger.Debugf("found existing lock \"%s\" for node %s, duration: %s", annotationName, node.Name, lockDuration.String())
			// lock vm for next redeploy, can take up to 15 mins
			if err := cacheLock.Add(node.Name, true, *lockDuration); err != nil {
				contextLogger.Error(err)
			}
		} else {
			cacheLock.Delete(node.Name)
		}
	}

	cacheLock.DeleteExpired()
}

func (r *AzureK8sAutopilot) autoUncordonExpiredNodes(contextLogger *zap.SugaredLogger, nodeList []*k8s.Node, annotationName string) {
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
