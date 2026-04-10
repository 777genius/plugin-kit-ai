package homebrewformula

import (
	"fmt"
	"strings"
)

func mustAsset(assets []Asset, goos, goarch string) Asset {
	for _, asset := range assets {
		if asset.GOOS == goos && asset.GOARCH == goarch {
			return asset
		}
	}
	panic(fmt.Sprintf("missing asset for %s/%s", goos, goarch))
}

func normalizeTag(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ""
	}
	if strings.HasPrefix(tag, "v") {
		return tag
	}
	return "v" + tag
}
