package autopilot

import (
	"fmt"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/containrrr/shoutrrr"
	"github.com/patrickmn/go-cache"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"regexp"
	"strings"
	"time"
)

var (
	azureSubscriptionRegexp   = regexp.MustCompile("^azure:///subscriptions/([^/]+)/resourceGroups/.*")
	azureResourceGroupRegexp  = regexp.MustCompile("^azure:///subscriptions/[^/]+/resourceGroups/([^/]+)/.*")
	azureVmssNameRegexp       = regexp.MustCompile("/providers/Microsoft.Compute/virtualMachineScaleSets/([^/]+)/.*")
	azureVmssInstanceIdRegexp = regexp.MustCompile("/providers/Microsoft.Compute/virtualMachineScaleSets/[^/]+/virtualMachines/([^/]+)$")
	azureVmNameRegexp         = regexp.MustCompile("/providers/Microsoft.Compute/virtualMachines/([^/]+)$")
)

type (
	AzureK8sAutopilot struct {
		Interval          *time.Duration
		NotReadyThreshold *time.Duration
		LockDuration      *time.Duration
		LockDurationError *time.Duration
		Limit             int
		DryRun            bool

		K8s struct {
			NodeLabelSelector string
		}

		Repair struct {
			VmssAction string
			VmAction   string

			ProvisioningState    []string
			provisioningStateAll bool
		}

		prometheus struct {
			repairCount *prometheus.CounterVec
			nodeStatus  *prometheus.GaugeVec
		}

		Notification []string

		azureAuthorizer autorest.Authorizer
		k8sClient       *kubernetes.Clientset

		nodeCache *cache.Cache
	}

	AzureK8sAutopilotNodeAzureInfo struct {
		NodeName       string
		NodeProviderId string
		ProviderId     string

		Subscription  string
		ResourceGroup string

		IsVmss         bool
		VMScaleSetName string
		VMInstanceID   string

		VMname string
	}
)

func (r *AzureK8sAutopilot) Init() {
	r.initAzure()
	r.initK8s()
	r.initMetrics()
	r.nodeCache = cache.New(15*time.Minute, 1*time.Minute)

	r.Repair.provisioningStateAll = false
	for key, val := range r.Repair.ProvisioningState {
		val = strings.ToLower(val)
		r.Repair.ProvisioningState[key] = val

		if val == "*" {
			r.Repair.provisioningStateAll = true
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

func (r *AzureK8sAutopilot) initMetrics() {
	r.prometheus.nodeStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "autopilot_node_status",
			Help: "autopilot node status",
		},
		[]string{"nodeName"},
	)
	prometheus.MustRegister(r.prometheus.nodeStatus)

	r.prometheus.repairCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "autopilot_node_repair_count",
			Help: "autopilot node repair action counter",
		},
		[]string{},
	)
	prometheus.MustRegister(r.prometheus.repairCount)
}

func (r *AzureK8sAutopilot) Run() {
	log.Infof("starting cluster check loop")

	if r.DryRun {
		log.Infof(" - DRY-RUN active")
	}

	log.Infof(" - general settings")
	log.Infof("   interval: %v", r.Interval)
	log.Infof("   notReady threshold: %v", r.NotReadyThreshold)
	log.Infof("   lock duration (repair): %v", r.LockDuration)
	log.Infof("   lock duration (error): %v", r.LockDurationError)
	log.Infof("   limit: %v", r.Limit)

	log.Infof(" - kubernetes settings")
	log.Infof("   node labelselector: %v", r.K8s.NodeLabelSelector)

	log.Infof(" - repair settings")
	log.Infof("   vmss action: %v", r.Repair.VmssAction)
	log.Infof("   vm action: %v", r.Repair.VmAction)
	if r.Repair.provisioningStateAll {
		log.Infof("   provisioningStates: * (all accepted)")
	} else {
		log.Infof("   provisioningStates: %v", r.Repair.ProvisioningState)
	}

	go func() {
		for {
			time.Sleep(*r.Interval)
			log.Infoln("checking cluster nodes")
			start := time.Now()
			r.repairRun()
			runtime := time.Now().Sub(start)
			log.Infof("finished after %s", runtime.String())
		}
	}()
}

func (r *AzureK8sAutopilot) sendNotificationf(message string, args ...interface{}) {
	r.sendNotification(fmt.Sprintf(message, args...))
}

func (r *AzureK8sAutopilot) sendNotification(message string) {
	for _, url := range r.Notification {
		if err := shoutrrr.Send(url, message); err != nil {
			log.Errorf("Unable to send shoutrrr notification: %v", err.Error())
		}
	}
}
