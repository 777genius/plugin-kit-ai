package app

import (
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
)

func resolvePublicationBaseContext(rootInput, targetInput, destInput, unsupportedMessage, missingDestMessage string) (publicationContext, error) {
	root := strings.TrimSpace(rootInput)
	if root == "" {
		root = "."
	}
	target := strings.TrimSpace(targetInput)
	switch target {
	case "codex-package", "claude":
	default:
		return publicationContext{}, fmt.Errorf(unsupportedMessage, "codex-package", "claude")
	}
	dest := strings.TrimSpace(destInput)
	if dest == "" {
		return publicationContext{}, fmt.Errorf(missingDestMessage)
	}

	graph, _, err := pluginmanifest.Discover(root)
	if err != nil {
		return publicationContext{}, err
	}
	if _, err := graph.Manifest.SelectedTargets(target); err != nil {
		return publicationContext{}, err
	}
	inspection, _, err := pluginmanifest.Inspect(root, target)
	if err != nil {
		return publicationContext{}, err
	}
	return publicationContext{
		root:       root,
		target:     target,
		dest:       dest,
		graph:      graph,
		inspection: inspection,
	}, nil
}
