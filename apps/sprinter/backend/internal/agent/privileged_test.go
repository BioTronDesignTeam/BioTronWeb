package agent

import (
	"testing"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/model"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/model/fake"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/tools"
)

// rosterTool is a countingTool only the who-has-access guard admits.
type rosterTool struct{ countingTool }

func (*rosterTool) Privileged() bool { return true }

func offeredNames(call fake.Call) map[string]bool {
	names := map[string]bool{}
	for _, spec := range call.Tools {
		names[spec.Name] = true
	}
	return names
}

// A question that may not read the roster is never offered the tool, and a
// model that names it anyway gets "no such tool", not the roster.
func TestAPublicQuestionCannotReachAPrivilegedTool(t *testing.T) {
	roster := &rosterTool{countingTool{name: "who_has_access", out: "- octocat: admin"}}
	public := &countingTool{name: "calendar_upcoming", out: "all good"}
	answering := fake.New(callTurn("who_has_access", `{"app":"sprinter"}`), textTurn("I cannot see that."))
	loop := New(answering, []tools.Tool{public, roster}, Options{})

	result, err := loop.Answer(t.Context(), "who are the managers?")
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if offered := offeredNames(answering.Calls()[0]); offered["who_has_access"] || !offered["calendar_upcoming"] {
		t.Fatalf("offered = %v, want the public tools only", offered)
	}
	if roster.runs != 0 {
		t.Fatal("the privileged tool ran for a public question")
	}
	if result.Private {
		t.Fatal("a public answer was marked private")
	}
	history := answering.Calls()[1].History
	got := history[len(history)-1].ToolResults[0]
	if !got.IsError {
		t.Fatalf("result = %+v, want the unknown-tool error", got)
	}
}

// AnswerPrivileged offers the tool, and an answer that used it is private.
func TestAPrivilegedAnswerIsMarkedPrivate(t *testing.T) {
	roster := &rosterTool{countingTool{name: "who_has_access", out: "- octocat: admin"}}
	answering := fake.New(callTurn("who_has_access", `{"app":"sprinter"}`), textTurn("octocat holds admin."))
	loop := New(answering, []tools.Tool{roster}, Options{})

	result, err := loop.AnswerPrivileged(t.Context(), "who may administer sprinter?")
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if !offeredNames(answering.Calls()[0])["who_has_access"] {
		t.Fatal("the privileged tool was not offered")
	}
	if roster.runs != 1 || !result.Private {
		t.Fatalf("runs = %d, private = %t, want one run and a private answer", roster.runs, result.Private)
	}

	// The same question without a call to the tool is not private.
	plain := New(fake.Text("Nothing to report."), []tools.Tool{roster}, Options{})
	if result, err := plain.AnswerPrivileged(t.Context(), "hello"); err != nil || result.Private {
		t.Fatalf("result = %+v, err = %v, want a public answer", result, err)
	}
}

// A thread is read and replayed by everyone in it, so Continue never offers
// the privileged tools.
func TestContinueNeverOffersAPrivilegedTool(t *testing.T) {
	roster := &rosterTool{countingTool{name: "who_has_access"}}
	answering := fake.Text("ok")
	loop := New(answering, []tools.Tool{roster}, Options{})
	if _, err := loop.Continue(t.Context(), []model.Message{{Role: model.RoleUser, Text: "hi"}}, "and now?"); err != nil {
		t.Fatalf("continue: %v", err)
	}
	if offeredNames(answering.Calls()[0])["who_has_access"] {
		t.Fatal("a thread was offered the privileged tool")
	}
}

func TestWhoHasAccessIsPrivileged(t *testing.T) {
	if !tools.IsPrivileged(tools.WhoHasAccess{}) {
		t.Fatal("who_has_access must be privileged")
	}
	if tools.IsPrivileged(tools.CalendarUpcoming{}) {
		t.Fatal("calendar_upcoming must not be privileged")
	}
}
