package channels

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// pushTransport is everything the queue needs from the channel layer, and
// deliberately nothing more: whether a conversation may be interrupted, where a
// project's pushes go, and how to put one on the wire.
//
// The queue is the part with the interesting rules — merging, eviction,
// protected items, deferred receipts — and it used to be readable only by
// reading the manager. Narrowing the dependency to these three questions is
// what lets the queue be read, and tested, on its own.
type pushTransport interface {
	conversationBusy(projectID string, scene Scene, conv string) bool
	pushTarget(category, projectID string) (*runningChannel, error)
	deliverPush(item CronPushItem, target *runningChannel) bool
}

// outboundGateway owns every message the platform sends on its own initiative —
// scheduled pushes and Run notifications — as opposed to replies inside a turn.
// The distinction that matters: these arrive while the user may be mid-turn, so
// they queue and wait for an idle conversation rather than interrupting.
type outboundGateway struct {
	transport pushTransport

	mu     sync.Mutex
	queues map[string]*pushQueue
	// sent fires once a tracked item actually leaves the queue. It is what
	// lets a deferred notification receipt settle to delivered instead of
	// staying deferred forever.
	sent func(id string)
}

func newOutboundGateway(transport pushTransport) *outboundGateway {
	return &outboundGateway{transport: transport, queues: map[string]*pushQueue{}}
}

func (g *outboundGateway) queueFor(key string) *pushQueue {
	g.mu.Lock()
	defer g.mu.Unlock()
	q, ok := g.queues[key]
	if !ok {
		q = &pushQueue{}
		g.queues[key] = q
	}
	return q
}

func (g *outboundGateway) setSentObserver(fn func(id string)) {
	g.mu.Lock()
	g.sent = fn
	g.mu.Unlock()
}

func (g *outboundGateway) notifySent(id string) {
	g.mu.Lock()
	observer := g.sent
	g.mu.Unlock()
	if observer != nil {
		observer(id)
	}
}

// flush drains the conversation's queue and reports, per tracked item id,
// whether that item actually reached the transport. Items without an id are
// fire-and-forget and absent from the result. An id missing from the result was
// neither sent nor dropped — it is still queued, waiting for a later flush.
func (g *outboundGateway) flush(key string) map[string]bool {
	items := g.take(key)
	if len(items) == 0 {
		return nil
	}
	outcome := map[string]bool{}
	for i, item := range items {
		// Re-check busy: a new user message may have arrived. Re-queue the
		// current item AND all remaining — never drop the tail.
		if g.transport.conversationBusy(item.ProjectID, item.Scene, item.Conv) {
			g.requeueAll(key, items[i:])
			return outcome
		}
		target, err := g.transport.pushTarget(item.Category, item.ProjectID)
		if err != nil {
			log.Warn().Err(err).Str("project", item.ProjectID).Str("category", item.Category).
				Msg("push flush: no delivery channel")
			if item.ID != "" {
				outcome[item.ID] = false
			}
			continue
		}
		sent := g.transport.deliverPush(item, target)
		if item.ID == "" {
			continue
		}
		outcome[item.ID] = sent
		if sent {
			g.notifySent(item.ID)
		}
	}
	return outcome
}

// sweep re-attempts every conversation that still holds queued pushes. Without
// it the queue only moves when some other event happens to touch the same
// conversation, so a notification enqueued while the user was mid-turn could
// sit in memory indefinitely.
func (g *outboundGateway) sweep() {
	for _, key := range g.pendingKeys() {
		g.flush(key)
	}
}

// requeueAll puts remaining flush items back ahead of anything that arrived
// while the queue was taken, re-applying merge/depth via enqueue so pending
// never silently exceeds pushQueueDepth.
func (g *outboundGateway) requeueAll(key string, items []CronPushItem) {
	if len(items) == 0 {
		return
	}
	arrived := g.drainRaw(key)
	combined := append(append([]CronPushItem(nil), items...), arrived...)
	for _, item := range combined {
		g.enqueue(key, item)
	}
}

// drainRaw clears pending without priority reordering (used by requeue).
func (g *outboundGateway) drainRaw(key string) []CronPushItem {
	q := g.queueFor(key)
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.pending) == 0 {
		return nil
	}
	out := append([]CronPushItem(nil), q.pending...)
	q.pending = nil
	return out
}

// pendingKeys lists conversation keys that still hold queued pushes, so a
// compensation sweep can find work without walking every conversation.
func (g *outboundGateway) pendingKeys() []string {
	g.mu.Lock()
	queues := make(map[string]*pushQueue, len(g.queues))
	for k, q := range g.queues {
		queues[k] = q
	}
	g.mu.Unlock()
	keys := make([]string, 0, len(queues))
	for k, q := range queues {
		q.mu.Lock()
		pending := len(q.pending)
		q.mu.Unlock()
		if pending > 0 {
			keys = append(keys, k)
		}
	}
	return keys
}

func (g *outboundGateway) take(key string) []CronPushItem {
	q := g.queueFor(key)
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.pending) == 0 {
		return nil
	}
	out := append([]CronPushItem(nil), q.pending...)
	q.pending = nil
	// Priority within a flush: changed/failed before compressible unchanged.
	return sortPushByPriority(out)
}

// The manager side of pushTransport. These three are the only reasons the queue
// still needs to know a channel layer exists.

func (m *Manager) conversationBusy(projectID string, scene Scene, conv string) bool {
	return m.IsConversationBusy(projectID, scene, conv)
}

func (m *Manager) pushTarget(category, projectID string) (*runningChannel, error) {
	if category == runNotifyCategory {
		target, _, _, err := m.lookupRunNotifyTarget(projectID)
		return target, err
	}
	target, _, _, err := m.lookupDeliveryTarget(projectID)
	return target, err
}

func (m *Manager) deliverPush(item CronPushItem, target *runningChannel) bool {
	stripped, urls := splitImageURLs(item.Text)
	ctx, cancel := context.WithTimeout(m.baseCtx, 60*time.Second)
	defer cancel()
	res := m.sendOutboundResult(ctx, target, OutboundMessage{
		Scene: item.Scene, ConversationID: item.Conv, Text: stripped,
		ImageURLs: urls, Envelope: item.Envelope,
	})
	return res.Sent
}

// Manager facade over the gateway. Call sites and tests keep the names they
// had, so the component underneath stayed free to move.

func (m *Manager) pushQueueFor(key string) *pushQueue { return m.outbound.queueFor(key) }
func (m *Manager) enqueuePush(key string, item CronPushItem) {
	m.outbound.enqueue(key, item)
}

func (m *Manager) flushPushQueue(key string) map[string]bool { return m.outbound.flush(key) }
func (m *Manager) takePushQueue(key string) []CronPushItem   { return m.outbound.take(key) }
func (m *Manager) drainPushQueueRaw(key string) []CronPushItem {
	return m.outbound.drainRaw(key)
}
func (m *Manager) pendingPushKeys() []string { return m.outbound.pendingKeys() }
func (m *Manager) requeuePushAll(key string, items []CronPushItem) {
	m.outbound.requeueAll(key, items)
}
func (m *Manager) notifyPushSent(id string) { m.outbound.notifySent(id) }

// SetPushSentObserver registers the callback invoked once a tracked push item
// leaves the queue for real.
func (m *Manager) SetPushSentObserver(fn func(id string)) { m.outbound.setSentObserver(fn) }

// SweepPushQueues re-attempts every conversation that still holds queued
// pushes.
func (m *Manager) SweepPushQueues() { m.outbound.sweep() }
