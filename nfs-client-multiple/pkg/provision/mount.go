package provision

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/kubernetes/pkg/volume/util"
)

// mount handler

type MountHandler struct {
	server     string
	exportPath string

	status    int32
	mountPath string
}

func NewMountHandler(server, exportPath string) (*MountHandler, error) {
	return &MountHandler{
		server:     server,
		exportPath: exportPath,
		status:     MountStatusNew,
	}, nil
}

func (mh *MountHandler) GetKey() string {
	return makeNfsPathKey(mh.server, mh.exportPath)
}

func (mh *MountHandler) GetServer() string {
	return mh.server
}
func (mh *MountHandler) GetExportPath() string {
	return mh.exportPath
}
func (mh *MountHandler) GetMountPath() string {
	return mh.mountPath
}

func (mh *MountHandler) GetStatus() int32 {
	return atomic.LoadInt32(&mh.status)
}
func (mh *MountHandler) SetStatus(status int32) {
	atomic.StoreInt32(&mh.status, status)
}

func (mh *MountHandler) AddVolume(volName string) (string, error) {
	if len(mh.mountPath) == 0 || mh.GetStatus() != MountStatusMounted {
		return "", ErrNotMountedYet
	}
	if len(filepath.Clean(volName)) == 0 {
		return "", ErrVolumeNameEmpty
	}
	path := filepath.Join(mh.mountPath, volName)

	if e := os.Mkdir(path, 0777); e != nil {
		return "", e
	}
	return path, nil
}

func (mh *MountHandler) Mount(mountBase string, mountOptions []string, mounter mount.Interface) error {
	mountDir := string(uuid.NewUUID())
	dir := getMountPoint(mountBase, mountDir)
	notMnt, err := mounter.IsLikelyNotMountPoint(dir)
	glog.V(4).Infof("NFS mount set up: %s %v %v", dir, !notMnt, err)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if !notMnt { // TODO someone else mount here?
		glog.Errorf("IsLikelyNotMountPoint %s mounted", dir)
		return nil
	}
	if err := os.MkdirAll(dir, 0750); err != nil {
		glog.Errorf("MkdirAll %s failed, %v", dir, err)
		return err
	}
	source := fmt.Sprintf("%s:%s", mh.server, mh.exportPath)
	mountOptions = util.JoinMountOptions(mountOptions, nil)
	err = mounter.Mount(source, dir, "nfs", mountOptions)
	if err != nil {
		notMnt, mntErr := mounter.IsLikelyNotMountPoint(dir)
		if mntErr != nil {
			glog.Errorf("IsLikelyNotMountPoint check failed: %v", mntErr)
			return err
		}
		if !notMnt {
			if mntErr = mounter.Unmount(dir); mntErr != nil {
				glog.Errorf("Failed to unmount: %v", mntErr)
				return err
			}
			notMnt, mntErr := mounter.IsLikelyNotMountPoint(dir)
			if mntErr != nil {
				glog.Errorf("IsLikelyNotMountPoint check failed: %v", mntErr)
				return err
			}
			if !notMnt {
				// This is very odd, we don't expect it.  We'll try again next sync loop.
				glog.Errorf("%s is still mounted, despite call to unmount().  Will try again next sync loop.", dir)
				return err
			}
		}
		os.Remove(dir)
		return err
	}
	mh.SetStatus(MountStatusMounted)
	mh.mountPath = dir
	return nil
}

func (mh *MountHandler) Unmount(mounter mount.Interface) error {
	if len(mh.mountPath) == 0 || mh.GetStatus() != MountStatusMounted {
		return ErrNotMountedYet
	}
	e := util.UnmountPath(mh.mountPath, mounter)
	if notMnt, _ := mounter.IsLikelyNotMountPoint(mh.mountPath); notMnt {
		mh.SetStatus(MountStatusUnmounted)
	}
	return e
}

// tools

func makeNfsPathKey(server, exportPath string) string {
	return server + ":" + exportPath
}

func getMountPoint(mountBase, mountDir string) string {
	return filepath.Join(mountBase, mountDir)
}
