package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// httpClient is shared by every tool that calls an external API. A timeout
// is set so a slow/unreachable upstream can't hang a reply indefinitely.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// getJSON fetches rawURL and decodes its JSON body into dest.
func getJSON(rawURL string, dest any) error {
	return getJSONWithHeaders(rawURL, nil, dest)
}

// getJSONWithHeaders is getJSON with extra request headers, for APIs that
// require one (e.g. GitHub's API rejects requests with no User-Agent).
func getJSONWithHeaders(rawURL string, headers map[string]string, dest any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
