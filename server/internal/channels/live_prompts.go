package channels

import (
	"strings"

	"github.com/cocofhu/approving/internal/services"
)

// SetLivePrompts applies the settings-page prompt overrides.
//
// Only the routing prompt's body lands here; the synthesis body belongs to the
// component that phrases outcomes, and temperature belongs to the model client.
// The composition root fans one settings snapshot out to all three rather than
// making each of them read the whole thing.
func (m *Manager) SetLivePrompts(p services.LivePrompts) {
	if m == nil {
		return
	}
	body := strings.TrimSpace(p.SystemBody)
	m.liveSystemBody.Store(&body)
}

// systemPrompt is the routing prompt as it will actually be sent: the fixed
// persona, then whichever body is in effect.
func (m *Manager) systemPrompt() string {
	if m == nil {
		return liveSystemPrompt
	}
	if body := m.liveSystemBody.Load(); body != nil && strings.TrimSpace(*body) != "" {
		return ComposeVoicePrompt(*body)
	}
	return liveSystemPrompt
}
