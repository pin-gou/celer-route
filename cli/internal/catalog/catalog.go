// Package catalog fetches the live model catalog from a running celer-route
// instance over HTTP. The CLI uses it to mirror what the Web UI does —
// get the available models so it can write explicit `models` blocks into
// each coding agent's config file (opencode, Claude Code, Codex…).
//
// Encoding for /v1/models matches the OpenAI "list models" surface:
//
//	{
//	  "data": [
//	    {"id": "minimax/MiniMax-M2.1", "name": "MiniMax-M2.1",
//	     "context_length": 1000000, "max_input_tokens": …,
//	     "max_output_tokens": 8192, "pricing": {...}}
//	  ]
//	}
//
// The CLI only reads fields it actually uses (id, name, context_length,
// max_output_tokens); extra fields are tolerated.
package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Model is the subset of /v1/models used by the CLI templates.
type Model struct {
	ID             string
	Name           string
	ContextLength  int
	MaxOutput      int
	AdditionalJSON map[string]any // reserved for future flags (e.g. modalities)
}

// List fetches models from baseURL (e.g. http://localhost:8080), optionally
// authenticated with a virtual key in Authorization: Bearer.
//
// Pass an empty apiKey to skip the Authorization header.
func List(ctx context.Context, baseURL, apiKey string) ([]Model, error) {
	url := strings.TrimRight(baseURL, "/") + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned %d: %s", url, resp.StatusCode, summarize(body))
	}

	var parsed struct {
		Data []struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			ContextLength  int    `json:"context_length"`
			MaxInputTokens int    `json:"max_input_tokens"`
			MaxOutput      int    `json:"max_output_tokens"`
			Additional     map[string]any
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode %s: %w", url, err)
	}
	out := make([]Model, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		id := m.ID
		if id == "" {
			// Defensive: if the gateway ever omits id but keeps name+provider,
			// the Web UI constructs `${provider}/${name}`; mirror that.
			continue
		}
		name := m.Name
		if name == "" {
			// Fallback to the part after "/" in id, e.g. "provider/MiniMax-M2.1".
			if i := strings.LastIndex(id, "/"); i >= 0 && i < len(id)-1 {
				name = id[i+1:]
			} else {
				name = id
			}
		}
		ctx := m.ContextLength
		if ctx == 0 {
			ctx = m.MaxInputTokens
		}
		out = append(out, Model{
			ID:             id,
			Name:           name,
			ContextLength:  ctx,
			MaxOutput:      m.MaxOutput,
			AdditionalJSON: m.Additional,
		})
	}
	return out, nil
}

// Filter keeps only models whose id contains any of the substrings in
// patterns. An empty patterns slice is a no-op (returns the input).
func Filter(in []Model, patterns []string) []Model {
	if len(patterns) == 0 {
		return in
	}
	out := make([]Model, 0, len(in))
	for _, m := range in {
		for _, p := range patterns {
			if p == "" {
				continue
			}
			if strings.Contains(m.ID, p) {
				out = append(out, m)
				break
			}
		}
	}
	return out
}

func summarize(b []byte) string {
	const max = 200
	if len(b) > max {
		return string(b[:max]) + "…"
	}
	return string(b)
}
