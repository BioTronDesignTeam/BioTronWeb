package tools

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/model"
)

const statusCap = 6000

// PlatformStatus reads Logger's public status document: the same thing the
// status page shows. It is the first tool to reach for, because it answers
// "is anything broken right now" in one call.
type PlatformStatus struct {
	LoggerURL string
}

func (t PlatformStatus) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name: "platform_status",
		Description: "Current health of every BioTron application and component: " +
			"the overall state, each application's state, each component's state, " +
			"and uptime over 24 hours, 7 days and 90 days. This is the live " +
			"picture. Use health_history for what happened earlier, and " +
			"recent_logs for why. Takes no arguments.",
		Schema: json.RawMessage(`{"type":"object","properties":{}}`),
	}
}

func (t PlatformStatus) Run(ctx context.Context, _ json.RawMessage) (string, error) {
	if t.LoggerURL == "" {
		return "", errors.New("no Logger URL is configured")
	}
	body, err := fetchJSON(ctx, t.LoggerURL+"/v1/status")
	if err != nil {
		return "", err
	}
	compacted, err := compact(body)
	if err != nil {
		return "", err
	}
	return capResult(compacted, statusCap,
		"The status document is longer than the cap. Report the overall state and the components that are not operational."), nil
}
