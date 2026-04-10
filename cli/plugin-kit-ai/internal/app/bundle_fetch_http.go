package app

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
)

type bundleHTTPClientConfig struct {
	AdditionalRootsFile string
}

func newDefaultBundleFetchHTTPClient() (bundleHTTPClient, *http.Client, error) {
	client, err := newBundleHTTPClient(bundleHTTPClientConfig{
		AdditionalRootsFile: strings.TrimSpace(os.Getenv(bundleFetchTestCAFileEnv)),
	})
	if err != nil {
		return bundleHTTPClient{}, nil, err
	}
	if strings.TrimSpace(os.Getenv(bundleFetchTestCAFileEnv)) == "" {
		return client, nil, nil
	}
	return client, client.Client, nil
}

func newBundleHTTPClient(cfg bundleHTTPClientConfig) (bundleHTTPClient, error) {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = (&net.Dialer{Timeout: defaultBundleFetchConnect}).DialContext
	if strings.TrimSpace(cfg.AdditionalRootsFile) != "" {
		pool, err := loadBundleFetchAdditionalRoots(cfg.AdditionalRootsFile)
		if err != nil {
			return bundleHTTPClient{}, err
		}
		if t.TLSClientConfig == nil {
			t.TLSClientConfig = &tls.Config{}
		}
		t.TLSClientConfig.RootCAs = pool
	}
	return bundleHTTPClient{
		Client: &http.Client{
			Timeout:   defaultBundleFetchTimeout,
			Transport: t,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= defaultBundleFetchMaxRedirects {
					return http.ErrUseLastResponse
				}
				if req.URL.Scheme != "https" {
					return http.ErrUseLastResponse
				}
				prev := via[len(via)-1].URL
				if prev.Scheme != "https" {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
		MaxBytes: defaultBundleFetchMaxBytes,
	}, nil
}

func loadBundleFetchAdditionalRoots(path string) (*x509.CertPool, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("bundle fetch test root CA file %q: %w", path, err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(body) {
		return nil, fmt.Errorf("bundle fetch test root CA file %q does not contain valid PEM certificates", path)
	}
	return pool, nil
}

func (c bundleHTTPClient) Download(ctx context.Context, url string) ([]byte, string, error) {
	if c.Client == nil {
		defaultClient, _, err := newDefaultBundleFetchHTTPClient()
		if err != nil {
			return nil, "", err
		}
		c = defaultClient
	}
	max := c.MaxBytes
	if max <= 0 {
		max = defaultBundleFetchMaxBytes
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("bundle fetch request: %w", err)
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("bundle fetch download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, "", fmt.Errorf("bundle fetch download: status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if resp.ContentLength > max {
		return nil, "", fmt.Errorf("bundle fetch download: content-length %d exceeds limit %d", resp.ContentLength, max)
	}
	limit := max
	if resp.ContentLength > 0 {
		limit = resp.ContentLength
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", fmt.Errorf("bundle fetch download read: %w", err)
	}
	if int64(len(body)) > max {
		return nil, "", fmt.Errorf("bundle fetch download exceeds limit %d bytes", max)
	}
	return body, resp.Header.Get("Content-Type"), nil
}
