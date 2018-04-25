package provision

import (
	"os"
	"sync"

	"github.com/golang/glog"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/kubernetes/pkg/util/mount"
)

// mount manager

type MountManager struct {
	m           map[string]*MountHandler // key: ip:path
	l           sync.RWMutex
	provisioner string
	mountBase   string
	mounter     mount.Interface
}

func NewMountManager(mountBase, provisioner string) (*MountManager, error) {
	if len(provisioner) == 0 {
		return nil, ErrArgsProvisionerEmpty
	}
	fi, e := os.Stat(mountBase)
	if e != nil {
		return nil, e
	}
	if !fi.IsDir() {
		return nil, ErrArgsMountBaseNotDir
	}
	mm := &MountManager{
		m:           make(map[string]*MountHandler),
		provisioner: provisioner,
		mountBase:   mountBase,
		mounter:     mount.New(""),
	}
	return mm, nil
}

func (mm *MountManager) Get(pm map[string]string) (*MountHandler, error) {
	server, exportPath, e := parseNfsPath(pm)
	if e != nil {
		return nil, e
	}
	return mm.GetByPath(server, exportPath)
}
func (mm *MountManager) GetByPath(server, exportPath string) (*MountHandler, error) {
	// check args
	if e := nfsPathCheck(server, exportPath); e != nil {
		return nil, e
	}

	// try get from cache
	mh := mm.getCache(makeNfsPathKey(server, exportPath))
	if mh != nil {
		return mh, nil
	}

	// make new if not cached
	mh, e := NewMountHandler(server, exportPath)
	if e != nil {
		return nil, e
	}
	e = mh.Mount(mm.mountBase, DefaultMountOpts, mm.mounter)
	if e != nil {
		return nil, e
	}
	return mh, nil
}

func (mm *MountManager) getCache(key string) *MountHandler {
	mm.l.RLock()
	mh := mm.m[key]
	mm.l.RUnlock()
	return mh
}

func (mm *MountManager) Return(mh *MountHandler) {
	if mh.GetStatus() != MountStatusMounted {
		return
	}

	key := mh.GetKey()
	prev := mm.getCache(key)
	if prev == nil {
		mm.l.Lock()
		prev = mm.m[key]
		if prev == nil {
			mm.m[key] = mh
		}
		mm.l.Unlock()
	}

	if prev != nil && prev.GetMountPath() != mh.GetMountPath() {
		// clean up extra mounted
		go mm.releaseHandler(mh)
	}
}

func (mm *MountManager) CleanUp(curClasses []*storagev1.StorageClass) {
	scMap := make(map[string]*storagev1.StorageClass, len(curClasses))
	for i := range curClasses {
		if curClasses[i].Provisioner != mm.provisioner {
			continue
		}
		k, e := parsePathKey(curClasses[i].Parameters)
		if e != nil {
			continue
		}
		scMap[k] = curClasses[i]
	}

	var (
		delKeys   []string
		delMounts []*MountHandler
	)
	mm.l.Lock()
	for k, v := range mm.m {
		sc := scMap[v.GetKey()]
		if sc == nil {
			delKeys = append(delKeys, k)
			delMounts = append(delMounts, v)
		}
	}
	for _, key := range delKeys {
		delete(mm.m, key)
	}
	mm.l.Unlock()

	for i := range delMounts {
		go mm.releaseHandler(delMounts[i])
	}
}

func (mm *MountManager) releaseHandler(mh *MountHandler) {
	e := mh.Unmount(mm.mounter)
	if e != nil {
		glog.Errorf("release not used, unmount %s failed, %v", mh.GetKey(), e)
	}
	glog.Infof("release not used, unmount %s done", mh.GetKey())
}

// tools

func nfsPathCheck(server, exportPath string) error {
	if len(server) == 0 {
		return ErrParametersMapServerEmpty
	}
	if len(exportPath) == 0 {
		return ErrParametersMapExportPathEmpty
	}
	return nil
}

func parseNfsPath(pm map[string]string) (server, exportPath string, e error) {
	if pm == nil {
		return "", "", ErrParametersMapNil
	}
	server = GetClassNfsServer(pm)
	exportPath = GetClassNfsExportPath(pm)
	e = nfsPathCheck(server, exportPath)
	return
}

func parsePathKey(pm map[string]string) (string, error) {
	server, exportPath, e := parseNfsPath(pm)
	if e != nil {
		return "", e
	}
	return makeNfsPathKey(server, exportPath), nil
}
