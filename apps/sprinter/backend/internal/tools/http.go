package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// httpTimeout bounds one call to a sibling service. Both endpoints are inside
// the compose network and answer in milliseconds; a call that does not is a
// service that is down, which is itself the answer.
const httpTimeout = 10 * time.Second

// maxBody is the most a tool will read off the wire. The two endpoints answer
// with kilobytes, so a response larger than this is a wrong URL, not a big
// answer, and reading it all would only waste memory.
const maxBody = 1 << 20

var httpClient = &http.Client{Timeout: httpTimeout}

// fetchJSON gets one public endpoint and returns its body. It sends no cookie,
// no token, and no header of ours: both endpoints are public, and a tool that
// carried a credential would be a way to reach something that is not.
func fetchJSON(ctx context.Context, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("service answered %d", response.StatusCode)
	}
	return body, nil
}

// compact strips the whitespace a pretty printer added. The model pays for
// every byte of a result, and indentation tells it nothing.
func compact(raw []byte) (string, error) {
	var out bytes.Buffer
	if err := json.Compact(&out, raw); err != nil {
		return "", fmt.Errorf("response was not JSON: %w", err)
	}
	return out.String(), nil
}
