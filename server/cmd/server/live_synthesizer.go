package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/channels"
	"github.com/cocofhu/approving/internal/models"
)

// synthesisTimeout bounds a reflow synthesis turn. Nobody is waiting on a live
// conversation for this, but a background event that has been pending for a
// minute is stale, and the structured fallback is already good enough to send.
const synthesisTimeout = 20 * time.Second

// newLiveSynthesizer phrases a background event through the conversation's own
// PM agent, so the result lands as part of that conversation rather than as a
// notification about it. The agent already knows what was asked and what has
// been said since; a template does not.
//
// Anything that goes wrong here degrades to the caller's structured fallback,
// which is a complete message in its own right — synthesis improves how an
// outcome reads, it is not what makes it deliverable.
func newLiveSynthesizer(bridge *channels.ChannelBridge) channels.SynthesisFunc {
	if bridge == nil {
		return nil
	}
	return func(ctx context.Context, req channels.SynthesisRequest) (string, error) {
		if strings.TrimSpace(req.Brief) == "" {
			return "", errors.New("synthesis brief is empty")
		}
		turnCtx, cancel := context.WithTimeout(ctx, synthesisTimeout)
		defer cancel()

		// A synthetic inbound: same conversation, same thread, same agent, so
		// the turn sees the history it needs. Progress is not forwarded —
		// intermediate narration from a synthesis turn is not something the
		// user asked to see. The agent's own pm_reply is captured by the
		// Manager for the duration of this turn, so returning empty here is
		// normal and does not mean synthesis failed.
		reply, err := bridge.Handle(turnCtx, channels.ResolvedChannel{
			Type: models.ChannelTypeQQ, ProjectID: req.ProjectID,
			TurnTimeout: synthesisTimeout,
		}, channels.InboundMessage{
			Scene: req.Scene, ConversationID: req.ConversationID,
			UserID: req.ExternalUserID, Text: req.Brief, Timestamp: time.Now(),
		}, nil)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(reply.FinalSummary), nil
	}
}
