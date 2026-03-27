package github

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/plugin-kit-ai/plugin-kit-ai/plugininstall/domain"
	"github.com/plugin-kit-ai/plugin-kit-ai/plugininstall/internal/httpconfig"
)

// DownloadAsset downloads the full body from browser_download_url (follows redirects; see httpconfig.DownloadClient).
func (c *Client) DownloadAsset(ctx context.Context, url string) ([]byte, string, error) {
	if c.DLClient == nil {
		c.DLClient = httpconfig.DownloadClient()
	}
	max := c.MaxBytes
	if max <= 0 {
		max = httpconfig.DefaultMaxDownloadBytes
	}

	for attempt := 0; attempt <= httpconfig.MaxRetries429; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, "", domain.NewError(domain.ExitNetwork, ctx.Err().Error())
			case <-time.After(backoff(attempt)):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, "", domain.NewError(domain.ExitNetwork, "download request: "+err.Error())
		}
		if c.Token != "" {
			req.Header.Set("Authorization", "Bearer "+c.Token)
		}
		resp, err := c.DLClient.Do(req)
		if err != nil {
			return nil, "", domain.NewError(domain.ExitNetwork, "download: "+err.Error())
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if sec, _ := strconv.Atoi(ra); sec > 0 {
					time.Sleep(time.Duration(sec) * time.Second)
				}
			}
			if attempt == httpconfig.MaxRetries429 {
				return nil, "", domain.NewError(domain.ExitNetwork, "github rate limited (429); set GITHUB_TOKEN or retry later")
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			return nil, "", domain.NewError(domain.ExitNetwork, fmt.Sprintf("download: status %s: %s", resp.Status, truncate(string(b), 200)))
		}
		cl := resp.ContentLength
		if cl > max {
			resp.Body.Close()
			return nil, "", domain.NewError(domain.ExitNetwork, fmt.Sprintf("download: content-length %d exceeds limit %d", cl, max))
		}
		lim := max
		if cl > 0 {
			lim = cl
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, lim+1))
		resp.Body.Close()
		if err != nil {
			return nil, "", domain.NewError(domain.ExitNetwork, "download read: "+err.Error())
		}
		if int64(len(data)) > max {
			return nil, "", domain.NewError(domain.ExitNetwork, fmt.Sprintf("download exceeds limit %d bytes", max))
		}
		return data, resp.Header.Get("Content-Type"), nil
	}
	return nil, "", domain.NewError(domain.ExitNetwork, "download failed")
}

func backoff(attempt int) time.Duration {
	d := time.Duration(1<<uint(attempt-1)) * time.Second
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d + time.Duration(attempt*100)*time.Millisecond
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
