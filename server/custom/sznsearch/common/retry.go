package common

import (
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

	for attempt := 0; attempt < maxAttempts; attempt++ {
		lastErr = operation()
		if lastErr == nil {
			if attempt > 0 {
				logger.Debug("Operation succeeded after retry", mlog.Int("attempt", attempt+1))
			}
			return nil
		}

		if attempt < maxAttempts-1 {
			// Calculate exponential backoff
			backoff := time.Duration(math.Min(
				float64(baseDelay)*math.Pow(2, float64(attempt)),
				float64(maxDelay),
			))

			// Only log error if it's not nil
			if lastErr != nil {
				logger.Debug("Operation failed, retrying",
					mlog.Int("attempt", attempt+1),
					mlog.Duration("backoff", backoff),
					mlog.Err(lastErr),
				)
			} else {
				logger.Debug("Operation failed (nil error), retrying",
					mlog.Int("attempt", attempt+1),
					mlog.Duration("backoff", backoff),
				)
			}

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
