package agentpluginscli

import (
	"context"
	"fmt"
	"sort"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

// preflightSelectedTargets resolves the complete caller-selected client set
// before any package or Directory work begins. ChatGPT is the sole synthetic
// client: signed compatibility may authorize preparing its local package even
// though the remote product cannot be detected locally.
func preflightSelectedTargets(ctx context.Context, app App, targets []domain.ClientID, clients []domain.DetectedClient) ([]domain.DetectedClient, map[domain.ClientID]domain.DetectedClient, error) {
	if clients == nil {
		detected, err := app.Detector.Detect(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("detect AI clients: %w", err)
		}
		clients = detected
	}
	detected := make(map[domain.ClientID]domain.DetectedClient, len(clients)+1)
	for _, client := range clients {
		detected[client.ClientID] = client
	}
	selected := make([]domain.DetectedClient, 0, len(targets))
	for _, target := range targets {
		client, ok := detectedSharedClient(target, detected)
		if target == domain.ClientChatGPT && !ok {
			client = domain.DetectedClient{ClientID: target, DisplayName: "ChatGPT", Status: domain.DetectionNotDetected}
			detected[target] = client
			ok = true
		}
		if !ok || (client.Status != domain.DetectionDetected && target != domain.ClientChatGPT) {
			return nil, nil, fmt.Errorf("target %q was not detected; no target was changed", target)
		}
		detected[target] = client
		selected = append(selected, client)
	}
	return selected, detected, nil
}

func detectedClientValues(clients map[domain.ClientID]domain.DetectedClient) []domain.DetectedClient {
	values := make([]domain.DetectedClient, 0, len(clients))
	for _, client := range clients {
		values = append(values, client)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ClientID < values[j].ClientID })
	return values
}
