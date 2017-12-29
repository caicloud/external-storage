package main

import (
	"flag"
	"os"
	"os/signal"
	"strconv"

	"github.com/golang/glog"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/kubernetes-incubator/external-storage/nfs-client-multiple/pkg/provision"
)

const (
	EnvKubeMaster = "ENV_KUBE_MASTER"
	EnvKubeConfig = "ENV_KUBE_CONFIG"

	EnvLogLevel     = "ENV_LOG_LEVEL"
	EnvLogVerbosity = "ENV_LOG_VERBOSITY"

	DefaultLogLevel     = "info"
	DefaultLogVerbosity = "0"

	GLogLogLevelFlagName     = "stderrthreshold"
	GLogLogVerbosityFlagName = "v"
)

var (
	logLevel     string
	logVerbosity string

	kubeMaster string
	kubeConfig string

	mountBase   string
	provisioner string

	failedDeleteThreshold    int
	failedProvisionThreshold int
	cleanUpIntervalSecond    int

	kc kubernetes.Interface

	sigCh  = make(chan os.Signal, 32)
	stopCh = make(chan struct{})
)

func flagInit() {
	logLevel = LoadEnvVarWithDefault(EnvLogLevel, DefaultLogLevel)
	logVerbosity = LoadEnvVarWithDefault(EnvLogVerbosity, DefaultLogVerbosity)
	kubeMaster = LoadEnvVarWithDefault(EnvKubeMaster, "")
	kubeConfig = LoadEnvVarWithDefault(EnvKubeConfig, "")
	mountBase = LoadEnvVarWithDefault(provision.EnvMountBase, provision.DefaultMountBase)
	provisioner = LoadEnvVarWithDefault(provision.EnvProvisionerName, provision.DefaultProvisionerName)
	failedDeleteThreshold = LoadIntEnvVarWithDefault(provision.EnvFailedDeleteThreshold, provision.DefaultCleanUpIntervalSecond)
	failedProvisionThreshold = LoadIntEnvVarWithDefault(provision.EnvFailedProvisionThreshold, provision.DefaultFailedDeleteThreshold)
	cleanUpIntervalSecond = LoadIntEnvVarWithDefault(provision.EnvCleanUpIntervalSecond, provision.DefaultFailedProvisionThreshold)

	flag.Set(GLogLogLevelFlagName, logLevel)
	flag.Set(GLogLogVerbosityFlagName, logVerbosity)
	flag.Parse()

	glog.Infof("%s: %v", EnvLogLevel, logLevel)
	glog.Infof("%s: %v", EnvLogVerbosity, logVerbosity)
	glog.Infof("%s: %v", EnvKubeMaster, kubeMaster)
	glog.Infof("%s: %v", EnvKubeConfig, kubeConfig)
	glog.Infof("%s: %v", provision.EnvMountBase, mountBase)
	glog.Infof("%s: %v", provision.EnvProvisionerName, provisioner)
	glog.Infof("%s: %v", provision.EnvFailedDeleteThreshold, failedDeleteThreshold)
	glog.Infof("%s: %v", provision.EnvFailedProvisionThreshold, failedProvisionThreshold)
	glog.Infof("%s: %v", provision.EnvCleanUpIntervalSecond, cleanUpIntervalSecond)
}

func kubeInit() {
	cfg, e := clientcmd.BuildConfigFromFlags(kubeMaster, kubeConfig)
	if e != nil {
		glog.Exitf("BuildConfigFromFlags failed, %v", e)
	}
	kc, e = kubernetes.NewForConfig(cfg)
	if e != nil {
		glog.Exitf("NewForConfig failed, %v", e)
	}
}

func main() {
	flagInit()
	kubeInit()

	p, e := provision.NewNfsProvisioner(mountBase, provisioner, kc)
	if e != nil {
		glog.Exitf("NewNfsProvisioner failed, %v", e)
	}

	signal.Notify(sigCh, os.Kill)
	go func() {
		<-sigCh
		signal.Stop(sigCh)
		glog.Warningf("receive kill signal, stop program")
		close(stopCh)
	}()

	glog.Infof("Starting nfs multiple provisioner")
	p.Run(stopCh)
}

func LoadEnvVarWithDefault(name, defaultValue string) string {
	ev, ok := os.LookupEnv(name)
	if !ok {
		return defaultValue
	}
	return ev
}

func LoadIntEnvVarWithDefault(name string, defaultValue int) int {
	s, ok := os.LookupEnv(name)
	if !ok {
		return defaultValue
	}

	i, e := strconv.ParseInt(s, 10, 32)
	if e != nil {
		return defaultValue
	}

	return int(i)
}
