package datasheet

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
)

// Bundled copies of the upstream datasheet JSON files, embedded into the
// binary so the gateway can still boot with a usable catalog when the remote
// URLs are unreachable. These are a startup safety net only — the background
// sync refreshes them from the live URL once the network is available again.
//
// datasheet.json is embedded raw (~1.8MB); model-parameters.json is embedded
// gzip-compressed (~19MB raw → ~350KB) to keep the binary lean.
//
//go:embed fallback/datasheet.json
var bundledPricingJSON []byte

//go:embed fallback/model-parameters.json.gz
var bundledModelParamsGZ []byte

// loadBundledPricing parses and returns the bundled pricing datasheet.
func loadBundledPricing() (map[string]Entry, error) {
	var data map[string]Entry
	if err := json.Unmarshal(bundledPricingJSON, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal bundled pricing data: %w", err)
	}
	return data, nil
}

// loadBundledModelParams decompresses and parses the bundled model-parameters
// datasheet.
func loadBundledModelParams() (map[string]json.RawMessage, error) {
	zr, err := gzip.NewReader(bytes.NewReader(bundledModelParamsGZ))
	if err != nil {
		return nil, fmt.Errorf("failed to open bundled model parameters gzip: %w", err)
	}
	defer zr.Close()
	raw, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress bundled model parameters: %w", err)
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal bundled model parameters: %w", err)
	}
	return data, nil
}

// shouldFallbackToBundled reports whether a failed URL fetch may fall back to
// the bundled copy: only when fetching the default URL (a custom URL is an
// operator decision, so its failures should surface as errors) and no existing
// data is available to serve instead.
func shouldFallbackToBundled(rawURL, defaultURL string, hasExistingData bool) bool {
	return rawURL == defaultURL && !hasExistingData
}
