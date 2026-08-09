package main

import (
	"strings"
	"sync/atomic"

	"github.com/cocofhu/approving/internal/channels"
	"github.com/cocofhu/approving/internal/liveagent"
	"github.com/cocofhu/approving/internal/services"
)

// livePromptRelay takes one settings snapshot and hands each part to whoever
// owns it: the routing prompt to the channel manager, the temperature to the
// model client, and the reporting prompt to the synthesizer that lives here.
//
// The fan-out belongs at the composition root rather than in any one of them.
// The alternative is passing the whole snapshot around and having three
// components each ignore two thirds of it, which is how a field ends up read in
// two places and applied in neither.
type livePromptRelay struct {
	mgr    *channels.Manager
	client *liveagent.Client
	// synthesisBody is the operator's replacement for the reporting body.
	// Empty means the built-in one.
	synthesisBody atomic.Pointer[string]
}

func newLivePromptRelay(mgr *channels.Manager, client *liveagent.Client) *livePromptRelay {
	return &livePromptRelay{mgr: mgr, client: client}
}

// SetLivePrompts implements services.LivePromptController.
func (r *livePromptRelay) SetLivePrompts(p services.LivePrompts) {
	if r == nil {
		return
	}
	body := strings.TrimSpace(p.SynthesisBody)
	r.synthesisBody.Store(&body)
	if r.mgr != nil {
		r.mgr.SetLivePrompts(p)
	}
	if r.client != nil {
		r.client.SetDefaultTemperature(p.Temperature)
	}
}

// synthesisPrompt is the reporting prompt as it will actually be sent.
func (r *livePromptRelay) synthesisPrompt() string {
	body := defaultSynthesisPromptBody
	if r != nil {
		if stored := r.synthesisBody.Load(); stored != nil && strings.TrimSpace(*stored) != "" {
			body = *stored
		}
	}
	return channels.ComposeVoicePrompt(body)
}

// livePromptDefaults describes the built-in prompts to the settings page, so a
// blank field can show what it falls back to.
func livePromptDefaults() services.LivePromptDefaults {
	return services.LivePromptDefaults{
		Prefix:        channels.VoicePersonaLead,
		SystemBody:    channels.DefaultLiveSystemPromptBody,
		SynthesisBody: defaultSynthesisPromptBody,
	}
}
