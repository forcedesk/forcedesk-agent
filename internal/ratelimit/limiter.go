package ratelimit

import (
	"sync"
	"time"
)

// Limiter implements a simple token bucket rate limiter.
type Limiter struct {
	mu         sync.Mutex
	tokens     int
	maxTokens  int
	refillRate time.Duration
	lastRefill time.Time
}

// NewLimiter creates a new rate limiter.
// maxTokens: maximum number of tokens in the bucket.
// refillRate: how often to add one token.
func NewLimiter(maxTokens int, refillRate time.Duration) *Limiter {
	return &Limiter{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow returns true if an action is allowed, false if rate limit exceeded.
func (l *Limiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.refill()

	if l.tokens > 0 {
		l.tokens--
		return true
	}

	return false
}

// refill adds tokens based on elapsed time.
func (l *Limiter) refill() {
	now := time.Now()
	elapsed := now.Sub(l.lastRefill)
	tokensToAdd := int(elapsed / l.refillRate)

	if tokensToAdd > 0 {
		l.tokens += tokensToAdd
		if l.tokens > l.maxTokens {
			l.tokens = l.maxTokens
		}
		l.lastRefill = now
	}
}

// Wait blocks until the limiter allows an action.
func (l *Limiter) Wait() {
	for !l.Allow() {
		time.Sleep(100 * time.Millisecond)
	}
}
