package audio

import "time"

const failedStartCooldown = time.Second

type startGuard struct {
	now          func() time.Time
	retryDelay   time.Duration
	blockedUntil time.Time
	lastErr      error
}

func newStartGuard() startGuard {
	return startGuard{
		now:        time.Now,
		retryDelay: failedStartCooldown,
	}
}

func (g *startGuard) beforeStart() error {
	if g.lastErr != nil && g.now().Before(g.blockedUntil) {
		return g.lastErr
	}
	return nil
}

func (g *startGuard) recordFailure(err error) error {
	g.lastErr = err
	g.blockedUntil = g.now().Add(g.retryDelay)
	return err
}

func (g *startGuard) clear() {
	g.lastErr = nil
	g.blockedUntil = time.Time{}
}
