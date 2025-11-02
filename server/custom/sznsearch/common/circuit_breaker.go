package common

import (
	"sync"
	"time"
)

// CircuitBreakerState represents the state of the circuit breaker
type CircuitBreakerState string

const (
	StateClosed   CircuitBreakerState = "closed"
	StateOpen     CircuitBreakerState = "open"
	StateHalfOpen CircuitBreakerState = "half-open"
)

// CircuitBreaker implements the circuit breaker pattern for Elasticsearch operations
type CircuitBreaker struct {
	mu               sync.RWMutex
	state            CircuitBreakerState
	failures         int
	consecutiveFails int
	successes        int
	lastFailTime     time.Time
	lastStateChange  time.Time

	// Configuration
	maxFailures     int           // Failures needed to open circuit
	cooldownPeriod  time.Duration // Time in Open before Half-Open
	halfOpenSuccess int           // Successes in Half-Open to close circuit
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(maxFailures int, cooldownPeriod time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:           StateClosed,
		maxFailures:     maxFailures,
		cooldownPeriod:  cooldownPeriod,
		halfOpenSuccess: 2, // 2 consecutive successes to close
		lastStateChange: time.Now(),
	}
}

// AllowRequest checks if request should proceed
func (cb *CircuitBreaker) AllowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		// Check if cooldown expired
		if time.Since(cb.lastFailTime) > cb.cooldownPeriod {
			cb.state = StateHalfOpen
			cb.successes = 0
			cb.lastStateChange = time.Now()
			return true
		}
		return false
	case StateHalfOpen:
		return true
	default:
		return false
	}
}

// RecordSuccess records successful operation
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecutiveFails = 0

	switch cb.state {
	case StateHalfOpen:
		cb.successes++
		if cb.successes >= cb.halfOpenSuccess {
			cb.state = StateClosed
			cb.failures = 0
			cb.lastStateChange = time.Now()
		}
	case StateClosed:
		// Already closed, nothing to do
	}
}

// RecordFailure records failed operation
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.consecutiveFails++
	cb.lastFailTime = time.Now()

	switch cb.state {
	case StateClosed:
		if cb.consecutiveFails >= cb.maxFailures {
			cb.state = StateOpen
			cb.successes = 0 // Reset successes when opening
			cb.lastStateChange = time.Now()
		}
	case StateHalfOpen:
		// Single failure in half-open immediately reopens circuit
		cb.state = StateOpen
		cb.successes = 0 // Reset successes when reopening
		cb.lastStateChange = time.Now()
	}
}

// GetState returns current state (thread-safe read)
func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetStats returns statistics (thread-safe read)
func (cb *CircuitBreaker) GetStats() (state CircuitBreakerState, failures, consecutiveFails, successes int) {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state, cb.failures, cb.consecutiveFails, cb.successes
}
