package versioner

import (
	"path/filepath"

	"github.com/flavio/kuberlr/internal/downloader"
	"github.com/flavio/kuberlr/internal/kubehelper"

	"github.com/blang/semver"
	"k8s.io/klog"
)

type localCache interface {
	LocalDownloadDir() string
	SetupLocalDirs() error
	LocalKubectlVersions() (semver.Versions, error)
	FindCompatibleKubectlAlreadyDownloaded(requestedVersion semver.Version) (semver.Version, error)
}

type downloadHelper interface {
	GetKubectlBinary(version semver.Version, destination string) error
	UpstreamStableVersion() (semver.Version, error)
}

type kubeAPIHelper interface {
	Version() (semver.Version, error)
}

// Versioner is used to manage the local kubectl binaries used by kuberlr
type Versioner struct {
	cache      localCache
	downloader downloadHelper
	apiServer  kubeAPIHelper
}

// NewVersioner is an helper function that creates a new Versioner instance
func NewVersioner() *Versioner {
	return &Versioner{
		cache:      &localCacheHandler{},
		downloader: &downloader.Downloder{},
		apiServer:  &kubehelper.KubeAPI{},
	}
}

// KubectlVersionToUse returns the kubectl version to be used to interact with
// the remote server. The method takes into account different failure scenarios
// and acts accordingly.
func (v *Versioner) KubectlVersionToUse() (semver.Version, error) {
	version, err := v.apiServer.Version()
	if err != nil {
		if isTimeout(err) {
			// the remote server is unreachable, let's get
			// the latest version of kubectl that is available on the system
			klog.Info("Remote kubernetes server unreachable")
		} else {
			klog.Info(err)
		}
		version, err = v.MostRecentKubectlDownloaded()
		if err != nil && isNoVersionFound(err) {
			klog.Info("No local kubectl binary found, fetching latest stable release version")
			version, err = v.downloader.UpstreamStableVersion()
		}
	}
	return version, err
}

func (v *Versioner) kubectlBinary(version semver.Version) string {
	return filepath.Join(
		v.cache.LocalDownloadDir(),
		BuildKubectNameFromVersion(version))
}

// EnsureKubectlIsAvailable ensures the kubectl binary with the specified
// version is available on the system. It will return the full path to the
// binary
func (v *Versioner) EnsureKubectlIsAvailable(version semver.Version) (string, error) {
	compatibleVersion, err := v.cache.FindCompatibleKubectlAlreadyDownloaded(version)
	if err == nil {
		return v.kubectlBinary(compatibleVersion), nil
	}

	klog.Infof("Right kubectl missing, downloading version %s", version.String())

	//download the right kubectl to the local cache
	if err := v.cache.SetupLocalDirs(); err != nil {
		return "", err
	}

	filename := v.kubectlBinary(version)
	if err := v.downloader.GetKubectlBinary(version, filename); err != nil {
		return "", err
	}

	return filename, nil
}

// MostRecentKubectlDownloaded returns the most recent version of
// kubectl downloaded by kuberlr
func (v *Versioner) MostRecentKubectlDownloaded() (semver.Version, error) {
	versions, err := v.cache.LocalKubectlVersions()
	if err != nil {
		return semver.Version{}, err
	}

	return versions[len(versions)-1], nil
}

func isTimeout(err error) bool {
	t, ok := err.(interface {
		Timeout() bool
	})
	return ok && t.Timeout()
}

func isNoVersionFound(err error) bool {
	t, ok := err.(noVersionFound)
	return ok && t.NoVersionFound()
}
