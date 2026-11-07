package discord

import (
	"context"
	"errors"
	"testing"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/model"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/store"
)

// recordingRunner remembers the request and answers with reply.
type recordingRunner struct {
	reply Reply
	got   *Request
}

func (r recordingRunner) Answer(_ context.Context, req Request) (Reply, error) {
	*r.got = req
	return r.reply, nil
}

func (r recordingRunner) Continue(_ context.Context, req Request, _ []model.Message) (Reply, error) {
	*r.got = req
	return r.reply, nil
}

const managers = "300000000000000009"

// Only an /agent question from someone the who-has-access guard admits may
// use the privileged tools. No guard means nobody.
func TestThePrivilegedToolsNeedTheWhoHasAccessGuard(t *testing.T) {
	roster := store.Guard{Subject: store.SubjectWhoHasAccess, GuildID: guild, RoleIDs: []string{managers}}
	withRoster := func(subject string) map[string]store.Guard {
		return map[string]store.Guard{subject: openGuard(subject), store.SubjectWhoHasAccess: roster}
	}
	cases := []struct {
		name    string
		subject string
		guards  map[string]store.Guard
		roles   []string
		want    bool
	}{
		{"no who-has-access guard", commandAgent, map[string]store.Guard{commandAgent: openGuard(commandAgent)}, []string{leads, managers}, false},
		{"a member without the role", commandAgent, withRoster(commandAgent), []string{leads}, false},
		{"a member with the role", commandAgent, withRoster(commandAgent), []string{leads, managers}, true},
		{"a thread, even with the role", commandAgentThread, withRoster(commandAgentThread), []string{leads, managers}, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var got Request
			bot := testBot(newFakeAPI(), &fakeStore{bySubject: testCase.guards},
				recordingRunner{reply: Reply{Text: "ok"}, got: &got})
			interaction, data := command(testCase.subject, general, testCase.roles...)
			bot.handleCommand(interaction, data)
			if got.Privileged != testCase.want {
				t.Fatalf("Privileged = %t, want %t", got.Privileged, testCase.want)
			}
		})
	}
}

// An answer that read the roster goes to the asker by direct message, and
// the channel sees only where it went.
func TestAPrivateAnswerGoesByDirectMessageOnly(t *testing.T) {
	api := newFakeAPI()
	bot := testBot(api, &fakeStore{guard: openGuard(commandAgent)},
		scriptedRunner{reply: Reply{Text: "octocat is a manager.", Private: true}})
	interaction, data := command(commandAgent, general, leads)
	bot.handleCommand(interaction, data)

	if posts := api.only("followup"); len(posts) != 0 {
		t.Fatalf("the roster was posted to the channel: %+v", posts)
	}
	dms := api.only("dm")
	if len(dms) != 1 || dms[0].channel != "400000000000000001" || dms[0].content != "octocat is a manager." {
		t.Fatalf("dms = %+v", dms)
	}
	if edits := api.only("edit"); len(edits) != 1 || edits[0].content != privateSentMessage {
		t.Fatalf("edits = %+v", edits)
	}
}

// When the DM cannot be sent, the answer is dropped, never posted.
func TestAPrivateAnswerIsNeverPostedWhenTheDMFails(t *testing.T) {
	api := newFakeAPI()
	api.dmErr = errors.New("cannot send messages to this user")
	bot := testBot(api, &fakeStore{guard: openGuard(commandAgent)},
		scriptedRunner{reply: Reply{Text: "octocat is a manager.", Private: true}})
	interaction, data := command(commandAgent, general, leads)
	bot.handleCommand(interaction, data)

	if posts := api.only("followup"); len(posts) != 0 {
		t.Fatalf("the roster was posted to the channel: %+v", posts)
	}
	if edits := api.only("edit"); len(edits) != 1 || edits[0].content != privateFailedMessage {
		t.Fatalf("edits = %+v", edits)
	}
}
