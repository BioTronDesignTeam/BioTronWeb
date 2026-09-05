package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/model"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/readstore"
)

const accessCap = 6000

// WhoHasAccess answers "who may use this app". It reads the grant table, the
// app's permission catalog, and the two operator flags that admit somebody
// without a grant. It never touches sessions or guest keys: who is signed in
// now, and today's guest key, are not access questions and are not the bot's
// to hand out.
type WhoHasAccess struct {
	Reader Reader
}

type accessArgs struct {
	App string `json:"app"`
}

func (t WhoHasAccess) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name: "who_has_access",
		Description: "Who may use one BioTron app: every operator holding a " +
			"permission on it, plus the managers and superusers, who are " +
			"admitted everywhere without a grant. app is an app id such as " +
			"sprinter, logger, exo or calendar. A banned operator is marked and " +
			"is refused whatever the grants say. Returns at most " +
			fmt.Sprint(accessCap) + " characters. It cannot see who is signed " +
			"in, and it cannot see guest keys.",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"app": {"type": "string", "description": "The app id to report on. Required."}
			},
			"required": ["app"]
		}`),
	}
}

func (t WhoHasAccess) Run(ctx context.Context, args json.RawMessage) (string, error) {
	var parsed accessArgs
	if err := decode(args, &parsed); err != nil {
		return "", err
	}
	app := strings.TrimSpace(parsed.App)
	if app == "" {
		return "", errors.New("app is required; name one app id, such as sprinter")
	}

	queryCtx, cancel := dbContext(ctx)
	defer cancel()
	report, err := t.Reader.Access(queryCtx, app)
	if err != nil {
		return "", err
	}

	var out strings.Builder
	if !report.AppFound {
		fmt.Fprintf(&out, "No app is registered with the id %q. "+
			"Any grants below would be orphaned rows.\n", app)
	} else {
		fmt.Fprintf(&out, "App %s (%s).\n", report.AppID, report.AppName)
	}

	if len(report.Permissions) == 0 {
		out.WriteString("Permissions defined: none.\n")
	} else {
		out.WriteString("Permissions defined:\n")
		for _, permission := range report.Permissions {
			fmt.Fprintf(&out, "- %s (%s)\n", permission.Key, fallback(permission.Label, "no label"))
		}
	}

	if len(report.Grants) == 0 {
		out.WriteString("Grants: none.\n")
	} else {
		fmt.Fprintf(&out, "Grants (%d):\n", len(report.Grants))
		for _, grant := range report.Grants {
			fmt.Fprintf(&out, "- %s%s: %s, granted %s%s\n",
				grant.Login, parenthesise(grant.Name), grant.PermissionKey,
				stamp(grant.GrantedAt), flags(grant.IsManager, grant.IsSuperuser, grant.IsBanned))
		}
	}

	writeOperators(&out, "Managers (admitted everywhere)", report.Managers)
	writeOperators(&out, "Superusers (admitted everywhere)", report.Superusers)

	return capResult(strings.TrimRight(out.String(), "\n"), accessCap,
		"Report the counts and name only the operators the question asked about."), nil
}

func writeOperators(out *strings.Builder, heading string, operators []readstore.OperatorRow) {
	if len(operators) == 0 {
		fmt.Fprintf(out, "%s: none.\n", heading)
		return
	}
	fmt.Fprintf(out, "%s (%d):\n", heading, len(operators))
	for _, operator := range operators {
		fmt.Fprintf(out, "- %s%s%s\n", operator.Login, parenthesise(operator.Name),
			flags(false, false, operator.IsBanned))
	}
}

func parenthesise(name string) string {
	if strings.TrimSpace(name) == "" {
		return ""
	}
	return " (" + name + ")"
}

// flags names only what changes the answer. Repeating "manager" on a grant
// matters because that operator would be admitted with or without the grant.
func flags(isManager, isSuperuser, isBanned bool) string {
	var marks []string
	if isSuperuser {
		marks = append(marks, "superuser")
	}
	if isManager {
		marks = append(marks, "manager")
	}
	if isBanned {
		marks = append(marks, "BANNED, refused everywhere")
	}
	if len(marks) == 0 {
		return ""
	}
	return " [" + strings.Join(marks, ", ") + "]"
}
