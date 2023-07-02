package k8s

import (
	"fmt"
	"os/exec"

	"go.uber.org/zap"
	"go.uber.org/zap/zapio"

	"github.com/webdevopos/azure-k8s-autopilot/config"
)

type (
	Kubectl struct {
		nodeName string
		logger   *zap.SugaredLogger

		Conf config.OptsDrain
	}
)

func (k *Kubectl) SetNode(nodeName string) {
	k.nodeName = nodeName
}

func (k *Kubectl) SetLogger(logger *zap.SugaredLogger) {
	k.logger = logger
}

func (k *Kubectl) NodeDrain() error {
	k.logger.Infof(fmt.Sprintf("drain node %v", k.nodeName))
	kubectlDrainOpts := []string{"drain", k.nodeName}
	kubectlDrainOpts = append(kubectlDrainOpts, fmt.Sprintf("--timeout=%v", k.Conf.Timeout.String()))

	if k.Conf.DeleteEmptydirData {
		kubectlDrainOpts = append(kubectlDrainOpts, "--delete-emptydir-data=true")
	}

	if k.Conf.Force {
		kubectlDrainOpts = append(kubectlDrainOpts, "--force=true")
	}

	if k.Conf.GracePeriod != 0 {
		kubectlDrainOpts = append(kubectlDrainOpts, fmt.Sprintf("--grace-period=%v", k.Conf.GracePeriod))
	}

	if k.Conf.IgnoreDaemonsets {
		kubectlDrainOpts = append(kubectlDrainOpts, "--ignore-daemonsets=true")
	}

	if k.Conf.PodSelector != "" {
		kubectlDrainOpts = append(kubectlDrainOpts, fmt.Sprintf("--pod-selector=%v", k.Conf.PodSelector))
	}

	if k.Conf.DisableEviction {
		kubectlDrainOpts = append(kubectlDrainOpts, "--disable-eviction=true")
	}

	return k.exec(kubectlDrainOpts...)
}

func (k *Kubectl) NodeUncordon() error {
	k.logger.Infof(fmt.Sprintf("uncordon node %v", k.nodeName))
	return k.exec("uncordon", k.nodeName)
}

func (k *Kubectl) exec(args ...string) error {
	if k.Conf.DryRun {
		args = append(args, "--dry-run")
	}

	return k.runComand(exec.Command(k.Conf.KubectlPath, args...)) // #nosec G204
}

func (k *Kubectl) runComand(cmd *exec.Cmd) (err error) {
	cmdLogger := k.logger.With(zap.String("command", "kubectl")).Desugar()
	cmdLogger = cmdLogger.WithOptions(zap.AddStacktrace(zap.PanicLevel), zap.WithCaller(false))
	k.logger.Debugf("EXEC: %v", cmd.String())

	stdOutWriter := &zapio.Writer{Log: cmdLogger, Level: zap.InfoLevel}
	defer stdOutWriter.Close()

	stdErrWriter := &zapio.Writer{Log: cmdLogger, Level: zap.ErrorLevel}
	defer stdErrWriter.Close()

	cmd.Stdout = stdOutWriter
	cmd.Stderr = stdErrWriter
	if cmdErr := cmd.Run(); cmdErr != nil {
		err = cmdErr
	}
	return
}
