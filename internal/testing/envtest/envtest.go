package envtest

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// FindBinaryAssetsDir locates the envtest binary assets directory under
// the repository's bin/k8s/ tree. It resolves the repo root relative to
// this source file (internal/testing/envtest/), then looks for versioned
// subdirectories matching the current OS and architecture
// (e.g. "1.32.0-darwin-arm64").
//
// Returns the path to the highest version found, or an empty string if
// none is available (envtest will fall back to KUBEBUILDER_ASSETS or
// /usr/local/kubebuilder/bin).
func FindBinaryAssetsDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}

	// This file lives at internal/testing/envtest/envtest.go — repo root is 3 levels up.
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")

	suffix := fmt.Sprintf("-%s-%s", runtime.GOOS, runtime.GOARCH)
	k8sDir := filepath.Join(root, "bin", "k8s")

	entries, err := os.ReadDir(k8sDir)
	if err != nil {
		return ""
	}

	var matches []string
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), suffix) {
			matches = append(matches, e.Name())
		}
	}
	if len(matches) == 0 {
		return ""
	}

	sort.Strings(matches)
	return filepath.Join(k8sDir, matches[len(matches)-1])
}
