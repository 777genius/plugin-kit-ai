package app

import (
	"fmt"
	"strings"
)

func validateBundleFetchMode(opts PluginBundleFetchOptions) error {
	if strings.TrimSpace(opts.URL) != "" {
		if strings.TrimSpace(opts.Ref) != "" {
			return fmt.Errorf("bundle fetch accepts either --url or owner/repo, not both")
		}
		if strings.TrimSpace(opts.Tag) != "" || opts.Latest {
			return fmt.Errorf("bundle fetch URL mode does not accept --tag or --latest")
		}
		if strings.TrimSpace(opts.AssetName) != "" || strings.TrimSpace(opts.Platform) != "" || strings.TrimSpace(opts.Runtime) != "" {
			return fmt.Errorf("bundle fetch URL mode does not use --asset-name, --platform, or --runtime")
		}
		return nil
	}
	if strings.TrimSpace(opts.Ref) == "" {
		return fmt.Errorf("bundle fetch requires --url or owner/repo")
	}
	if opts.Latest && strings.TrimSpace(opts.Tag) != "" {
		return fmt.Errorf("bundle fetch does not use --tag together with --latest")
	}
	if !opts.Latest && strings.TrimSpace(opts.Tag) == "" {
		return fmt.Errorf("bundle fetch GitHub mode requires --tag or --latest")
	}
	if (strings.TrimSpace(opts.Platform) == "") != (strings.TrimSpace(opts.Runtime) == "") {
		return fmt.Errorf("bundle fetch GitHub mode requires --platform and --runtime together")
	}
	return nil
}
