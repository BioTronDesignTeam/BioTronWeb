package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/model"
)

const (
	healthCap       = 6000
	healthRowLimit  = 200
	healthDetailCap = 200
)

// HealthHistory reads the recorded checks for one component, with the detail
// string the public status page hides. The detail is the dial error, which is
// the difference between "it was down" and "why it was down".
type HealthHistory struct {
	Reader Reader
}

type healthArgs struct {
	Service string `json:"service"`
	Hours   *int   `json:"hours"`
}

func (t HealthHistory) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name: "health_history",
		Description: "Recorded health checks for one component, newest first, " +
			"with the failure detail. Use it to say when a component went down " +
			"and what the check reported. service is a component id such as " +
			"logger-api, oauth-manager, exo-web or postgres. Returns at most " +
			fmt.Sprint(healthRowLimit) + " checks and " + fmt.Sprint(healthCap) +
			" characters.",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"service": {"type": "string", "description": "The component id to look at. Required."},
				"hours": {"type": "integer", "description": "How far back to look, 1 to 168 hours. Defaults to 24."}
			},
			"required": ["service"]
		}`),
	}
}

func (t HealthHistory) Run(ctx context.Context, args json.RawMessage) (string, error) {
	var parsed healthArgs
	if err := decode(args, &parsed); err != nil {
		return "", err
	}
	service := strings.TrimSpace(parsed.Service)
	if service == "" {
		return "", errors.New("service is required; name one component id, such as logger-api")
	}
	hours, err := clampInt(parsed.Hours, 24, 1, 168)
	if err != nil {
		return "", fmt.Errorf("hours %w", err)
	}

	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	queryCtx, cancel := dbContext(ctx)
	defer cancel()
	rows, err := t.Reader.HealthHistory(queryCtx, service, since, healthRowLimit)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return fmt.Sprintf("No health checks recorded for %q in the last %d hours. "+
			"Check the component id against platform_status.", service, hours), nil
	}

	failures := 0
	var out strings.Builder
	for _, row := range rows {
		if !row.OK {
			failures++
		}
	}
	fmt.Fprintf(&out, "%s: %d checks in the last %d hours, %d failed. Newest first.\n",
		service, len(rows), hours, failures)
	for _, row := range rows {
		state := "up"
		if !row.OK {
			state = "down"
		}
		fmt.Fprintf(&out, "%s %s", stamp(row.CheckedAt), state)
		if detail := strings.TrimSpace(row.Detail); detail != "" {
			fmt.Fprintf(&out, " | %s", truncate(detail, healthDetailCap))
		}
		out.WriteString("\n")
	}
	return capResult(strings.TrimRight(out.String(), "\n"), healthCap,
		"Ask for fewer hours, or report only the failed checks."), nil
}
