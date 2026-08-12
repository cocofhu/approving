package shutdown

import "time"

// gracePeriod returns the configured grace upper bound (test helper).
func (c *Coordinator) gracePeriod() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.grace
}
