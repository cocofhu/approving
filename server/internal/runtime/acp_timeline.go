package runtime

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/sandbox"
)

// acpTimelineEntry is the platform-side in-memory ACP timeline snapshot for one
// run+node while the sandbox session is live. It is the authoritative read path
// for nodeEvents during execution; cold FetchEventLog is a supplement only.
type acpTimelineEntry struct {
	events    []models.AcpEvent
	live      bool
	ready     bool // at least one successful ingest or emit write
	updatedAt time.Time
}

type acpTimelineStore struct {
	mu      sync.Mutex
	entries map[string]*acpTimelineEntry
	ingest  map[string]context.CancelFunc
}

func newAcpTimelineStore() *acpTimelineStore {
	return &acpTimelineStore{
		entries: map[string]*acpTimelineEntry{},
		ingest:  map[string]context.CancelFunc{},
	}
}

func timelineKey(runID, nodeID string) string { return runID + "|" + nodeID }

func (s *acpTimelineStore) begin(runID, nodeID string) {
	key := timelineKey(runID, nodeID)
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[key]
	if !ok {
		e = &acpTimelineEntry{live: true}
		s.entries[key] = e
	} else {
		e.live = true
	}
	e.updatedAt = time.Now()
}

func (s *acpTimelineStore) upsert(runID, nodeID string, events []models.AcpEvent) {
	if s == nil {
		return
	}
	key := timelineKey(runID, nodeID)
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[key]
	if !ok {
		e = &acpTimelineEntry{live: true}
		s.entries[key] = e
	}
	e.ready = true
	e.events = pickLongerEvents(e.events, events)
	e.updatedAt = time.Now()
}

func pickLongerEvents(prev, incoming []models.AcpEvent) []models.AcpEvent {
	if len(incoming) > len(prev) {
		return append([]models.AcpEvent(nil), incoming...)
	}
	if len(incoming) == len(prev) && len(incoming) > 0 {
		if lastT(incoming) >= lastT(prev) {
			return append([]models.AcpEvent(nil), incoming...)
		}
	}
	if len(prev) == 0 && len(incoming) == 0 {
		return nil
	}
	return prev
}

func lastT(events []models.AcpEvent) int {
	if len(events) == 0 {
		return 0
	}
	return events[len(events)-1].T
}

func (s *acpTimelineStore) stop(runID, nodeID string) {
	if s == nil {
		return
	}
	key := timelineKey(runID, nodeID)
	s.mu.Lock()
	if cancel, ok := s.ingest[key]; ok {
		cancel()
		delete(s.ingest, key)
	}
	if e, ok := s.entries[key]; ok {
		e.live = false
		e.updatedAt = time.Now()
	}
	s.mu.Unlock()
}

func (s *acpTimelineStore) startIngest(runID, nodeID, host string, port int) {
	if s == nil || host == "" || port <= 0 {
		return
	}
	key := timelineKey(runID, nodeID)
	s.begin(runID, nodeID)

	s.mu.Lock()
	if cancel, ok := s.ingest[key]; ok {
		cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.ingest[key] = cancel
	s.mu.Unlock()

	go s.ingestLoop(ctx, runID, nodeID, host, port)
}

func (s *acpTimelineStore) ingestLoop(ctx context.Context, runID, nodeID, host string, port int) {
	// Immediate first pull so cold page loads see history without waiting.
	s.refreshFromSandbox(ctx, runID, nodeID, host, port)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshFromSandbox(ctx, runID, nodeID, host, port)
		}
	}
}

func (s *acpTimelineStore) refreshFromSandbox(ctx context.Context, runID, nodeID, host string, port int) {
	res, _, err := sandbox.FetchEventLog(ctx, host, port)
	if err != nil || res == nil {
		return // keep existing snapshot on transient bridge failures
	}
	s.upsert(runID, nodeID, res.AcpEvents())
}

func (s *acpTimelineStore) get(runID, nodeID string) (acpTimelineEntry, bool) {
	if s == nil {
		return acpTimelineEntry{}, false
	}
	key := timelineKey(runID, nodeID)
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[key]
	if !ok || !e.ready {
		return acpTimelineEntry{}, false
	}
	return *e, true
}

func (s *acpTimelineStore) page(runID, nodeID, cursor string, limit int) ([]models.AcpEvent, string, bool, bool, bool) {
	e, ok := s.get(runID, nodeID)
	if !ok {
		return nil, "", false, false, false
	}
	ev, next, more := pageTimelineEvents(e.events, cursor, limit)
	return ev, next, more, e.live, true
}

// pageTimelineEvents slices a chronological event array for cursor/limit
// pagination. The first page returns the most recent limit items.
func pageTimelineEvents(events []models.AcpEvent, cursor string, limit int) ([]models.AcpEvent, string, bool) {
	total := len(events)
	if total == 0 {
		return nil, "", false
	}
	if limit <= 0 {
		limit = 20
	}
	if cursor == "" {
		start := total - limit
		if start < 0 {
			start = 0
		}
		page := events[start:]
		return page, strconv.Itoa(start), start > 0
	}
	end, err := strconv.Atoi(cursor)
	if err != nil || end <= 0 {
		return nil, "", false
	}
	if end > total {
		end = total
	}
	start := end - limit
	if start < 0 {
		start = 0
	}
	page := events[start:end]
	return page, strconv.Itoa(start), start > 0
}
