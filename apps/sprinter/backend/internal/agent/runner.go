package agent

import (
	"context"

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/discord"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/model"
)

// Runner puts the loop behind the bot's Runner interface.
//
// The adapter lives here rather than in the discord package so that the bot
// keeps knowing nothing about tools, models, or token counts. It carries the
// dependency the other way — this file imports discord, discord imports
// nothing of the agent — which is what lets the bot's tests run with a fake
// runner and no model at all.
type Runner struct {
	agent *Agent
}

func NewRunner(agent *Agent) Runner { return Runner{agent: agent} }

func (r Runner) Answer(ctx context.Context, req discord.Request) (discord.Reply, error) {
	return toReply(r.agent.Answer(ctx, req.Question))
}

func (r Runner) Continue(ctx context.Context, req discord.Request, history []model.Message) (discord.Reply, error) {
	return toReply(r.agent.Continue(ctx, history, req.Question))
}

func toReply(result Result, err error) (discord.Reply, error) {
	if err != nil {
		return discord.Reply{}, err
	}
	return discord.Reply{
		Text: result.Text,
		// A rate-limited question was never answered, so it leaves no turns to
		// store. The person still reads the sentence and can try again.
		Messages:     result.Messages,
		Tools:        result.Tools,
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
	}, nil
}
