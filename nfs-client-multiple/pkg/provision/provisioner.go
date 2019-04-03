package provision

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/kubernetes-incubator/external-storage/lib/controller"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type NfsProvisioner struct {
	provisioner string
	kc          kubernetes.Interface
	mm          *MountManager

	failedDeleteThreshold    int
	failedProvisionThreshold int
	cleanUpIntervalSecond    int
}

func NewNfsProvisioner(mountBase, provisioner string, kc kubernetes.Interface) (*NfsProvisioner, error) {
	mm, e := NewMountManager(mountBase, provisioner)
	if e != nil {
		return nil, e
	}
	return &NfsProvisioner{
		provisioner:              provisioner,
		kc:                       kc,
		mm:                       mm,
		failedDeleteThreshold:    DefaultFailedDeleteThreshold,
		failedProvisionThreshold: DefaultFailedProvisionThreshold,
		cleanUpIntervalSecond:    DefaultCleanUpIntervalSecond,
	}, nil
}

func (p *NfsProvisioner) SetFailedDeleteThreshold(v int)    { p.failedDeleteThreshold = v }
func (p *NfsProvisioner) SetFailedProvisionThreshold(v int) { p.failedProvisionThreshold = v }
func (p *NfsProvisioner) SetCleanUpIntervalSecond(v int)    { p.cleanUpIntervalSecond = v }

func (p *NfsProvisioner) Provision(options controller.VolumeOptions) (*corev1.PersistentVolume, error) {
	if options.PVC.Spec.Selector != nil {
		return nil, ErrClaimSelectorNotSupported
	}
	if e := CheckClassNfsReadOnlyFormat(options.Parameters); e != nil {
		return nil, e
	}

	glog.V(4).Infof("nfs provisioner: VolumeOptions %v", options)

	pvcNamespace := options.PVC.Namespace
	pvcName := options.PVC.Name
	scName := GetPVCClass(options.PVC)

	pvName := strings.Join([]string{pvcNamespace, pvcName, options.PVName}, "-")

	mh, e := p.mm.Get(options.Parameters)
	if e != nil {
		glog.Errorf("Get failed, %v", e)
		return nil, e
	}
	defer p.mm.Return(mh)

	dirPath, e := mh.AddVolume(pvName)
	if e != nil {
		glog.Errorf("AddVolume failed, %v", e)
		return nil, errors.New("unable to create directory to provision new pv: " + e.Error())
	}
	os.Chmod(dirPath, 0777)

	path := filepath.Join(mh.GetExportPath(), pvName)

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: options.PVName,
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: options.PersistentVolumeReclaimPolicy,
			AccessModes:                   options.PVC.Spec.AccessModes,
			Capacity: corev1.ResourceList{
				corev1.ResourceName(corev1.ResourceStorage): options.PVC.Spec.Resources.Requests[corev1.ResourceName(corev1.ResourceStorage)],
			},
			StorageClassName: scName,
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				NFS: &corev1.NFSVolumeSource{
					Server:   mh.GetServer(),
					Path:     path,
					ReadOnly: IsClassNfsReadOnly(options.Parameters),
				},
			},
		},
	}
	glog.Infof("done")
	return pv, nil
}

func (p *NfsProvisioner) Delete(volume *corev1.PersistentVolume) error {
	server := volume.Spec.PersistentVolumeSource.NFS.Server
	path := volume.Spec.PersistentVolumeSource.NFS.Path
	exportPath := filepath.Dir(path)
	pvName := filepath.Base(path)

	mh, e := p.mm.GetByPath(server, exportPath)
	if e != nil {
		glog.Errorf("GetByPath failed, %v", e)
		return e
	}
	defer p.mm.Return(mh)

	oldPath := filepath.Join(mh.GetMountPath(), pvName)
	archivePath := filepath.Join(mh.GetMountPath(), "archived-"+pvName)
	glog.V(4).Infof("archiving path %s to %s", oldPath, archivePath)
	e = os.Rename(oldPath, archivePath)
	if e != nil {
		glog.Errorf("Rename failed, %v", e)
		return e
	}
	go func() {
		e := os.RemoveAll(archivePath)
		if e != nil {
			glog.Errorf("RemoveAll %s for %s failed, %v", archivePath, mh.GetKey(), e)
		}
		glog.Infof("RemoveAll %s for %s done", archivePath, mh.GetKey())
	}()
	return nil
}

func (p *NfsProvisioner) cleanUp(stopCh chan struct{}) {
	tk := time.NewTicker(time.Duration(p.cleanUpIntervalSecond) * time.Second)
	for {
		select {
		case <-tk.C:
			scList, e := p.kc.StorageV1().StorageClasses().List(metav1.ListOptions{})
			if e != nil {
				glog.Errorf("List StorageClasses failed, %v", e)
				continue
			}
			scs := make([]*storagev1.StorageClass, 0, len(scList.Items))
			for i := range scList.Items {
				sc := &scList.Items[i]
				if sc.Provisioner == p.provisioner {
					scs = append(scs, sc)
				}
			}
			p.mm.CleanUp(scs)
			glog.Infof("process clean up %d", len(scs))
		case <-stopCh:
			tk.Stop()
			glog.Warningf("clean up stopped")
			return
		}
	}
}

func (p *NfsProvisioner) Run(stopCh chan struct{}) {
	// The controller needs to know what the server version is because out-of-tree
	// provisioners aren't officially supported until 1.5
	serverVersion, err := p.kc.Discovery().ServerVersion()
	if err != nil {
		glog.Fatalf("Error getting server version: %v", err)
	}

	// Start the provision controller which will dynamically provision efs NFS PVs
	c := controller.NewProvisionController(p.kc,
		p.provisioner,
		p,
		serverVersion.GitVersion,
		controller.FailedDeleteThreshold(p.failedDeleteThreshold),
		controller.FailedProvisionThreshold(p.failedProvisionThreshold),
		controller.LeaderElection(false),
	)

	go c.Run(stopCh)
	go p.cleanUp(stopCh)
	<-stopCh
	glog.Warningf("NfsProvisioner Run stopped")
}
