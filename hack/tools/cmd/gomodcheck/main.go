package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"go.uber.org/zap"
	"sigs.k8s.io/yaml"
)

const (
	modFile = "./go.mod"
)

type config struct {
	UpstreamRefs    []string `yaml:"upstreamRefs"`
	ExcludedModules []string `yaml:"excludedModules"`
}

type upstream struct {
	Ref     string `json:"ref"`
	Version string `json:"version"`
}

// representation of an out of sync module
type oosMod struct {
	Name      string     `json:"name"`
	Version   string     `json:"version"`
	Upstreams []upstream `json:"upstreams"`
}

func main() {
	l, _ := zap.NewProduction()
	logger := l.Sugar()

	if len(os.Args) < 2 {
		fmt.Printf("USAGE: %s [PATH_TO_CONFIG_FILE]\n", os.Args[0])
		os.Exit(1)
	}

	// --- 1. parse config
	b, err := os.ReadFile(os.Args[1])
	if err != nil {
		logger.Fatal(err.Error())
	}

	cfg := new(config)
	if err := yaml.Unmarshal(b, cfg); err != nil {
		logger.Fatal(err.Error())
	}

	excludedMods := make(map[string]any)
	for _, mod := range cfg.ExcludedModules {
		excludedMods[mod] = nil
	}

	// --- 2. project mods
	deps, err := parseModFile()
	if err != nil {
		logger.Fatal(err.Error())
	}

	// --- 3. upstream mods (holding upstream refs)
	upstreamModGraph, err := getUpstreamModGraph(cfg.UpstreamRefs)
	if err != nil {
		logger.Fatal(err.Error())
	}

	oosMods := make([]oosMod, 0)

	// --- 4. validate
	// for each module in our project,
	// if it matches an upstream module,
	// then for each upstream module,
	// if project module version doesn't match upstream version,
	// then we add the version and the ref to the list of out of sync modules.
	for mod, version := range deps {
		if _, ok := excludedMods[mod]; ok {
			logger.Infof("skipped excluded module: %s", mod)
			continue
		}

		if versionToRef, ok := upstreamModGraph[mod]; ok {
			upstreams := make([]upstream, 0)

			for upstreamVersion, upstreamRef := range versionToRef {
				if version != upstreamVersion {
					upstreams = append(upstreams, upstream{
						Ref:     upstreamRef,
						Version: upstreamVersion,
					})
				}
			}

			if len(upstreams) > 0 {
				oosMods = append(oosMods, oosMod{
					Name:      mod,
					Version:   version,
					Upstreams: upstreams,
				})
			}
		}
	}

	if len(oosMods) == 0 {
		fmt.Println("Success! 🎉")
		os.Exit(0)
	}

	b, err = json.MarshalIndent(map[string]any{"outOfSyncModules": oosMods}, "", "  ")
	if err != nil {
		panic(err)
	}

	fmt.Println(string(b))
	os.Exit(1)
}

var (
	cleanMods     = regexp.MustCompile(`\t| *//.*`)
	modDelimStart = regexp.MustCompile(`^require.*`)
	modDelimEnd   = ")"
)

func parseModFile() (map[string]string, error) {
	b, err := os.ReadFile(modFile)
	if err != nil {
		return nil, err
	}

	in := string(cleanMods.ReplaceAll(b, []byte("")))
	out := make(map[string]string)

	start := false
	for _, s := range strings.Split(in, "\n") {
		switch {
		case modDelimStart.MatchString(s) && !start:
			start = true
		case s == modDelimEnd:
			return out, nil
		case start:
			kv := strings.SplitN(s, " ", 2)
			if len(kv) < 2 {
				return nil, fmt.Errorf("unexpected format for module: %q", s)
			}

			out[kv[0]] = kv[1]
		}
	}

	return out, nil
}

func getUpstreamModGraph(upstreamRefs []string) (map[string]map[string]string, error) {
	b, err := exec.Command("go", "mod", "graph").Output()
	if err != nil {
		return nil, err
	}

	graph := string(b)
	o1Refs := make(map[string]bool)
	for _, upstreamRef := range upstreamRefs {
		o1Refs[upstreamRef] = false
	}

	modToVersionToUpstreamRef := make(map[string]map[string]string)

	for _, line := range strings.Split(graph, "\n") {
		upstreamRef := strings.SplitN(line, "@", 2)[0]
		if _, ok := o1Refs[upstreamRef]; ok {
			o1Refs[upstreamRef] = true
			kv := strings.SplitN(strings.SplitN(line, " ", 2)[1], "@", 2)
			name := kv[0]
			version := kv[1]

			if m, ok := modToVersionToUpstreamRef[kv[0]]; ok {
				m[version] = upstreamRef
			} else {
				versionToRef := map[string]string{version: upstreamRef}
				modToVersionToUpstreamRef[name] = versionToRef
			}
		}
	}

	notFound := ""
	for ref, found := range o1Refs {
		if !found {
			notFound = fmt.Sprintf("%s%s, ", notFound, ref)
		}
	}

	if notFound != "" {
		return nil, fmt.Errorf("cannot verify modules;"+
			"the following specified upstream module cannot be found in go.mod: [ %s ]",
			strings.TrimSuffix(notFound, ", "))
	}

	return modToVersionToUpstreamRef, nil
}
