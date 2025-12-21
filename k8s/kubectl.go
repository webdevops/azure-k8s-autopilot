package k8s

import (
	"fmt"
	"log/slog"
	"os/exec"

	"github.com/utkuozdemir/go-slogio"
	"github.com/webdevopos/azure-k8s-autopilot/config"
	"github.com/webdevops/go-common/log/slogger"
)

type (
	Kubectl struct {
		nodeName string
		logger   *slogger.Logger

		Conf config.OptsDrain
	}
)

func (k *Kubectl) SetNode(nodeName string) {
	k.nodeName = nodeName
}

func (k *Kubectl) SetLogger(logger *slogger.Logger) {
	k.logger = logger
}

func (k *Kubectl) NodeDrain() error {
	k.logger.Info("drain node", slog.String("node", k.nodeName))
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
	k.logger.Info("uncordon node", slog.String("node", k.nodeName))
	return k.exec("uncordon", k.nodeName)
}

func (k *Kubectl) exec(args ...string) error {
	if k.Conf.DryRun {
		args = append(args, "--dry-run")
	}

	return k.runComand(exec.Command(k.Conf.KubectlPath, args...)) // #nosec G204
}

func (k *Kubectl) runComand(cmd *exec.Cmd) (err error) {
	cmdLogger := k.logger.With(slog.String("command", "kubectl"))
	writer := &slogio.Writer{Log: cmdLogger.Slog(), Level: slogger.LevelInfo}
	defer writer.Close()

	cmd.Stdout = writer
	cmd.Stderr = writer

	k.logger.Debugf("EXEC: %v", cmd.String())
	if cmdErr := cmd.Run(); cmdErr != nil {
		err = cmdErr
	}
	return
}
