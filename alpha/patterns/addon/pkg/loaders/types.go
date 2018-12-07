package loaders

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	yaml "gopkg.in/yaml.v2"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

type Repository interface {
	LoadChannel(ctx context.Context, name string) (*Channel, error)
	LoadManifest(ctx context.Context, packageName string, id string) (string, error)
}

// FSRepository is a Repository backed by a filesystem
type FSRepository struct {
	basedir string
}

var _ Repository = &FSRepository{}

// NewFSRepository is the constructor for an FSRepository
func NewFSRepository(basedir string) *FSRepository {
	return &FSRepository{
		basedir: basedir,
	}
}

var safelistChannelName = "abcdefghijklmnopqrstuvwxyz"

// We validate the channel name - keeping it to a small subset helps with path traversal,
// and also ensures that we can back easily this by other stores (e.g. https)
func allowedChannelName(name string) bool {
	if !matchesSafelist(name, safelistChannelName) {
		return false
	}

	// Double check!
	if strings.HasPrefix(name, ".") {
		return false
	}

	return true
}

var safelistVersion = "abcdefghijklmnopqrstuvwxyz0123456789-."

func allowedManifestId(name string) bool {
	if !matchesSafelist(name, safelistVersion) {
		return false
	}

	// Double check!
	if strings.HasPrefix(name, ".") {
		return false
	}

	return true
}

func matchesSafelist(s string, safelist string) bool {
	for _, c := range s {
		if strings.IndexRune(safelist, c) == -1 {
			return false
		}
	}
	return true
}

func (r *FSRepository) LoadChannel(ctx context.Context, name string) (*Channel, error) {
	if !allowedChannelName(name) {
		return nil, fmt.Errorf("invalid channel name: %q", name)
	}

	log := log.Log
	log.WithValues("channel", name).WithValues("base", r.basedir).Info("loading channel")

	p := filepath.Join(r.basedir, name)
	b, err := ioutil.ReadFile(p)
	if err != nil {
		log.WithValues("path", p).Error(err, "error reading channel")
		return nil, fmt.Errorf("error reading channel %s: %v", p, err)
	}

	channel := &Channel{}
	if err := yaml.Unmarshal(b, &channel); err != nil {
		return nil, fmt.Errorf("error parsing channel %s: %v", p, err)
	}

	return channel, nil
}

func (r *FSRepository) LoadManifest(ctx context.Context, packageName string, id string) (string, error) {
	if !allowedManifestId(packageName) {
		return "", fmt.Errorf("invalid package name: %q", id)
	}

	if !allowedManifestId(id) {
		return "", fmt.Errorf("invalid manifest id: %q", id)
	}

	log := log.Log
	log.WithValues("package", packageName).Info("loading package")

	p := filepath.Join(r.basedir, "packages", packageName, id, "manifest.yaml")
	b, err := ioutil.ReadFile(p)
	if err != nil {
		return "", fmt.Errorf("error reading package %s: %v", p, err)
	}

	return string(b), nil
}

type Channel struct {
	Manifests []Version `json:"manifests,omitempty"`
}

type Version struct {
	Version string
}

func (c *Channel) Latest() (*Version, error) {
	var latest *Version
	for i := range c.Manifests {
		v := &c.Manifests[i]
		if latest == nil {
			latest = v
		} else {
			return nil, fmt.Errorf("version selection not implemented")
		}
	}

	return latest, nil
}
