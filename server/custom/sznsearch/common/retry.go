package common

import (
	"errors"
	"math"
	"time"

	"github.com/mattermost/mattermost/server/public/shared/mlog"
)

// RetryWithBackoff retries function with exponential backoff
func RetryWithBackoff(
	maxAttempts int,
	baseDelay time.Duration,
	maxDelay time.Duration,
	logger mlog.LoggerIFace,
	operation func() error,
) error {
	var lastErr error

	for attempt := range maxAttempts {
		lastErr = operation()
		if lastErr == nil {
			if attempt > 0 {
				logger.Debug("Operation succeeded after retry", mlog.Int("attempt", attempt+1))
			}
			return nil
		}

		// Check if error is non-retryable
		var nonRetryable *NonRetryableError
		if errors.As(lastErr, &nonRetryable) {
			logger.Debug("Non-retryable error encountered, stopping retries",
				mlog.Int("attempt", attempt+1),
				mlog.Err(lastErr),
			)
			return nonRetryable.Unwrap()
		}

		if attempt < maxAttempts-1 {
			// Calculate exponential backoff
			backoff := time.Duration(math.Min(
				float64(baseDelay)*math.Pow(2, float64(attempt)),
				float64(maxDelay),
			))

			logger.Debug("Operation failed, retrying",
				mlog.Int("attempt", attempt+1),
				mlog.Duration("backoff", backoff),
				mlog.Err(lastErr),
			)

			time.Sleep(backoff)
		}
	}

	// Log final failure with error (defensive check for nil pointer)
	if lastErr != nil {
		logger.Error("Operation failed after all retries",
			mlog.Int("max_attempts", maxAttempts),
			mlog.Err(lastErr),
		)
	} else {
		logger.Error("Operation failed after all retries with no error details",
			mlog.Int("max_attempts", maxAttempts),
		)
	}

	return lastErr
}
