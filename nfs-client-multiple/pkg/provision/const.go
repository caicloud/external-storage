package provision

import (
	"fmt"
	"strings"

	"k8s.io/api/core/v1"

	"github.com/kubernetes-incubator/external-storage/lib/controller"
)

const (
	EnvMountBase                = "ENV_MOUNT_BASE"
	EnvProvisionerName          = "ENV_PROVISION_NAME"
	EnvFailedDeleteThreshold    = "ENV_FAILED_DELETE_THRESHOLD"
	EnvFailedProvisionThreshold = "ENV_FAILED_PROVISION_THRESHOLD"
	EnvCleanUpIntervalSecond    = "ENV_CLEAN_UP_INTERVAL_SECOND"
)

const (
	DefaultMountBase                = "/caicloud/external-storage/mnt-nfs"
	DefaultProvisionerName          = "nfs"
	DefaultCleanUpIntervalSecond    = 600
	DefaultFailedDeleteThreshold    = controller.DefaultFailedDeleteThreshold
	DefaultFailedProvisionThreshold = controller.DefaultFailedProvisionThreshold

	LabelKeyNfsServer     = "server"
	LabelKeyNfsExportPath = "exportPath"
	LabelKeyNfsReadOnly   = "readOnly"

	LabelKeyKubeStorageClass = v1.BetaStorageClassAnnotation

	MountStatusNew       = 0
	MountStatusMounted   = 1
	MountStatusUnmounted = 2
)

var (
	ErrArgsMountBaseNotDir          = fmt.Errorf("mount base not dir")
	ErrArgsProvisionerEmpty         = fmt.Errorf("provisioner empty")
	ErrParametersMapNil             = fmt.Errorf("parameters map nil")
	ErrParametersMapServerEmpty     = fmt.Errorf("server empty")
	ErrParametersMapExportPathEmpty = fmt.Errorf("exportPath empty")
	ErrNotMountedYet                = fmt.Errorf("not mounted yet")
	ErrVolumeNameEmpty              = fmt.Errorf("volume name empty")
	ErrClaimSelectorNotSupported    = fmt.Errorf("claim Selector is not supported")
)

func GetClassNfsServer(pm map[string]string) string {
	return pm[LabelKeyNfsServer]
}
func GetClassNfsExportPath(pm map[string]string) string {
	return pm[LabelKeyNfsExportPath]
}
func GetClassNfsReadOnly(pm map[string]string) string {
	return pm[LabelKeyNfsReadOnly]
}
func IsClassNfsReadOnly(pm map[string]string) bool { // default is false
	ro := GetClassNfsReadOnly(pm)
	return len(ro) > 0 && strings.ToLower(ro) == "true"
}

func CheckClassNfsReadOnlyFormat(pm map[string]string) error {
	ro := GetClassNfsReadOnly(pm)
	if len(ro) == 0 {
		return nil
	}
	switch strings.ToLower(ro) {
	case "true", "false":
		return nil
	default:
		return fmt.Errorf("bad value %s for readOnly boolean", ro)
	}
}

func GetPVCClass(pvc *v1.PersistentVolumeClaim) string {
	scName := pvc.Annotations[LabelKeyKubeStorageClass]
	if len(scName) == 0 && pvc.Spec.StorageClassName != nil {
		scName = *pvc.Spec.StorageClassName
	}
	return scName
}
