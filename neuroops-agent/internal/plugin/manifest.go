package plugin

// manifest.go — Plugin binary integrity verification.
//
// Every plugin directory may contain a manifest.json file that records the
// SHA-256 hash of the plugin binary. The engine verifies this hash on discovery
// and again before each execution, preventing binary substitution attacks.
//
// manifest.json format:
//
//	{
//	  "plugin_id": "linux_cpu",
//	  "sha256":    "e3b0c44298fc1c149afbf4c8996fb924...",
//	  "version":   "1.0.0",
//	  "signed":    false
//	}
//
// To generate a manifest for an existing plugin:
//
//	sha256sum /path/to/plugin/plugin > sha256.txt
//	# then create manifest.json manually with the hex hash.
//
// Environment controls:
//   NEUROOPS_STRICT_MANIFESTS=true  — reject plugins that have no manifest.json
//                                     (default: false; warns but allows execution)

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// PluginManifest describes the expected integrity state of one plugin binary.
type PluginManifest struct {
	PluginID string `json:"plugin_id"`
	SHA256   string `json:"sha256"`  // lower-case hex-encoded SHA-256 of the binary
	Version  string `json:"version"`
	Signed   bool   `json:"signed"`  // future: cryptographic signing
}

// loadManifest reads manifest.json from dir.
// Returns (nil, nil) when the file does not exist so callers can decide
// whether to treat absence as an error.
func loadManifest(dir string) (*PluginManifest, error) {
	path := filepath.Join(dir, "manifest.json")
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil // not present — caller decides
	}
	if err != nil {
		return nil, fmt.Errorf("open manifest: %w", err)
	}
	defer f.Close()

	var m PluginManifest
	if err := json.NewDecoder(f).Decode(&m); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	if m.SHA256 == "" {
		return nil, fmt.Errorf("manifest.json has no sha256 field")
	}
	return &m, nil
}

// verifyBinaryIntegrity hashes binaryPath with SHA-256 and compares to expected.
func verifyBinaryIntegrity(binaryPath, expectedHex string) error {
	f, err := os.Open(binaryPath)
	if err != nil {
		return fmt.Errorf("open plugin binary: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash plugin binary: %w", err)
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expectedHex {
		return fmt.Errorf("integrity mismatch — expected %s got %s", expectedHex, actual)
	}
	return nil
}

// strictManifestsEnabled returns true when NEUROOPS_STRICT_MANIFESTS=true.
func strictManifestsEnabled() bool {
	return os.Getenv("NEUROOPS_STRICT_MANIFESTS") == "true"
}
