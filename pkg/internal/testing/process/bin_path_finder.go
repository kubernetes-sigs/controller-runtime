package process

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// EnvAssetsPath is the environment variable that stores the global test
	// binary location override.
	EnvAssetsPath = "KUBEBUILDER_ASSETS"
	// EnvAssetOverridePrefix is the environment variable prefix for per-binary
	// location overrides.
	EnvAssetOverridePrefix = "TEST_ASSET_"
	// AssetsDefaultPath is the default location to look for test binaries in,
	// if no override was provided.
	AssetsDefaultPath = "/usr/local/kubebuilder/bin"
)

// BinPathFinder finds the path to the given named binary, using the following locations
// in order of precedence (highest first).  Notice that the various env vars only need
// to be set -- the asset is not checked for existence on the filesystem.
//
// 1. TEST_ASSET_{tr/a-z-/A-Z_/} (if set; asset overrides -- EnvAssetOverridePrefix)
// 1. KUBEBUILDER_ASSETS (if set; global asset path -- EnvAssetsPath)
// 3. assetDirectory (if set; per-config asset directory)
// 4. /usr/local/kubebuilder/bin (AssetsDefaultPath)
func BinPathFinder(symbolicName, assetDirectory string) (binPath string) {
	punctuationPattern := regexp.MustCompile("[^A-Z0-9]+")
	sanitizedName := punctuationPattern.ReplaceAllString(strings.ToUpper(symbolicName), "_")
	leadingNumberPattern := regexp.MustCompile("^[0-9]+")
	sanitizedName = leadingNumberPattern.ReplaceAllString(sanitizedName, "")
	envVar := EnvAssetOverridePrefix + sanitizedName

	// TEST_ASSET_XYZ
	if val, ok := os.LookupEnv(envVar); ok {
		return val
	}

	// KUBEBUILDER_ASSETS
	if val, ok := os.LookupEnv(EnvAssetsPath); ok {
		return filepath.Join(val, symbolicName)
	}

	// assetDirectory
	if assetDirectory != "" {
		return filepath.Join(assetDirectory, symbolicName)
	}

	// default path
	return filepath.Join(AssetsDefaultPath, symbolicName)
}
