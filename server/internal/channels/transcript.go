package channels

import (
	"time"

	"github.com/cocofhu/approving/internal/models"
)

// ConversationRef identifies one IM conversation.
//
// Every layer that records or reads the conversation addresses it by this, so
// the routing model, the sandbox agent and the stored thread are always looking
// at the same exchange rather than at three private reconstructions of it.
type ConversationRef struct {
	ProjectID      string
	ChannelType    string
	Scene          Scene
	ConversationID string
}

// TranscriptEntry is one turn as the user experienced it: something they sent,
// or something a channel confirmed it delivered to them.
type TranscriptEntry struct {
	ID     string
	Role   string // "user" | "assistant"
	Text   string
	Images []models.PromptImage
	At     time.Time
}

// Transcript is the conversation as it actually happened.
//
// It exists because the layers that answer a message do not otherwise share a
// memory of it. The routing model is stateless, the sandbox agent remembers
// only the turns it personally ran, and neither can see what the other said —
// so a question answered outside the sandbox simply never happened as far as
// the sandbox is concerned, and the user has to repeat themselves. Writing both
// sides here, once, is what makes the conversation continuous.
//
// Only two kinds of thing belong in it: what the user sent, and what a channel
// confirmed it delivered. Draft replies, reasoning and tool narration are not
// part of the conversation no matter how much they look like it.
type Transcript interface {
	// RecordInbound stores a message the user sent, attachments included, and
	// returns the entry so later layers can refer to it by id.
	RecordInbound(ref ConversationRef, in InboundMessage) (TranscriptEntry, error)
	// RecordOutbound stores text a channel has confirmed it delivered.
	RecordOutbound(ref ConversationRef, text string) (TranscriptEntry, error)
	// Window returns the most recent turns, oldest first.
	Window(ref ConversationRef, limit int) ([]TranscriptEntry, error)
}

// conversationRefFor builds the transcript key for an inbound message.
func conversationRefFor(rc *runningChannel, in InboundMessage) ConversationRef {
	return ConversationRef{
		ProjectID:      rc.cfg.ProjectID,
		ChannelType:    rc.cfg.Type,
		Scene:          in.Scene,
		ConversationID: in.ConversationID,
	}
}
