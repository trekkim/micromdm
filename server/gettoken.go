package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// GetTokenHTTPCaller implements mdm.GetToken by calling an external HTTP
// service (e.g. NanoDEP /v1/maidjwt/{name}) and returning the JWT bytes.
type GetTokenHTTPCaller struct {
	url    string
	client *http.Client
}

// NewGetTokenHTTPCaller creates a GetTokenHTTPCaller that GETs the given URL
// to obtain the MAID JWT. The full URL (including any credentials and DEP
// instance name) is specified by the caller — e.g.
// http://localhost:9001/v1/maidjwt/trendmicro
func NewGetTokenHTTPCaller(url string, client *http.Client) *GetTokenHTTPCaller {
	return &GetTokenHTTPCaller{url: url, client: client}
}

// GetToken calls the configured URL and returns the response body as the
// token data. The returned bytes are placed into the TokenData plist key
// sent back to the device.
func (c *GetTokenHTTPCaller) GetToken(ctx context.Context, udid, serviceType string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return nil, fmt.Errorf("create GetToken request: %w", err)
	}
	req.Header.Set("X-UDID", udid)
	req.Header.Set("X-Token-Service-Type", serviceType)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GetToken request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read GetToken response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GetToken service returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}
