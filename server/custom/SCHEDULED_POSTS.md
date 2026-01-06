# Scheduled Posts - Seznam.cz Custom Modifications

## Overview

Scheduled posts are a feature that allows users to schedule messages to be sent at a later time. While Mattermost officially requires an enterprise license for this feature, **the entire implementation is in the open source part of the codebase**.

## License Compliance

This modification is **fully compliant with AGPL v3.0** because:

1. The scheduled posts functionality is **100% implemented in open source code** (no enterprise-only code exists)
2. We only removed artificial license checks from open source code
3. No proprietary/closed-source code is involved
4. All modifications are documented and available

## What Was Changed

We disabled license checks in 3 files by commenting out the validation code:

### 1. Server API Layer
**File:** `server/channels/api4/scheduled_post.go`

```go
func requireScheduledPostsEnabled(c *Context) {
    if !*c.App.Srv().Config().ServiceSettings.ScheduledPosts {
        c.Err = model.NewAppError("", "api.scheduled_posts.feature_disabled", nil, "", http.StatusBadRequest)
        return
    }

    // SZN: Disabled license check for scheduled posts - feature is fully implemented in open source
    // if c.App.Channels().License() == nil {
    //     c.Err = model.NewAppError("", "api.scheduled_posts.license_error", nil, "", http.StatusBadRequest)
    //     return
    // }
}
```

### 2. Server Job Layer
**File:** `server/channels/app/scheduled_post_job.go`

```go
func (a *App) ProcessScheduledPosts(rctx request.CTX) {
    rctx = rctx.WithLogger(rctx.Logger().With(mlog.String("component", "scheduled_post_job")))

    if !*a.Config().ServiceSettings.ScheduledPosts {
        return
    }

    // SZN: Disabled license check for scheduled posts - feature is fully implemented in open source
    // if a.License() == nil {
    //     return
    // }
    
    // ... rest of implementation
}
```

### 3. Webapp Selector
**File:** `webapp/channels/src/packages/mattermost-redux/src/selectors/entities/scheduled_posts.ts`

```typescript
export const isScheduledPostsEnabled: (a: GlobalState) => boolean = createSelector(
    'isScheduledPostsEnabled',
    getConfig,
    getLicense,
    (config: Partial<ClientConfig>, license: ClientLicense): boolean => {
        // SZN: Removed license check - scheduled posts work without enterprise license
        return config.ScheduledPosts === 'true';
    },
);
```

## What Was NOT Changed (No Custom Implementation Needed)

The following components work out-of-the-box without any custom code:

✅ **Database Schema** - `ScheduledPosts` table already exists  
✅ **API Endpoints** - All REST API endpoints are fully implemented  
✅ **Store Layer** - Complete CRUD operations in `SqlScheduledPostStore`  
✅ **App Layer** - Full business logic in `scheduled_post.go`  
✅ **Job Processing** - Background job for sending scheduled posts  
✅ **WebSocket Events** - Real-time updates for scheduled posts  
✅ **Webapp UI** - Complete user interface for managing scheduled posts  

## Architecture

### How Scheduled Posts Work

```
┌─────────────────────────────────────────────────────────────┐
│                     Scheduled Posts Flow                     │
└─────────────────────────────────────────────────────────────┘

1. User creates scheduled post via webapp
   └─> POST /api/v4/posts/schedule
       └─> api4.createSchedulePost()
           └─> requireScheduledPostsEnabled() [SZN: license check disabled]
           └─> app.SaveScheduledPost()
               └─> store.CreateScheduledPost()
                   └─> Saves to ScheduledPosts table

2. Background job processes scheduled posts (every 5 minutes)
   └─> runScheduledPostJob() [runs on cluster leader only]
       └─> ProcessScheduledPosts() [SZN: license check disabled]
           └─> GetPendingScheduledPosts() [fetch posts due for sending]
           └─> For each scheduled post:
               ├─> Validate permissions
               ├─> Convert to regular post
               └─> CreatePost() [publish to channel]

3. Webapp displays scheduled posts
   └─> isScheduledPostsEnabled() [SZN: license check disabled]
       └─> Shows scheduled posts in UI
```

### Key Components

**Server-side:**
- `server/channels/api4/scheduled_post.go` - REST API handlers
- `server/channels/app/scheduled_post.go` - Business logic (CRUD)
- `server/channels/app/scheduled_post_job.go` - Background processing job
- `server/channels/store/sqlstore/scheduled_post_store.go` - Database operations

**Webapp:**
- `webapp/channels/src/packages/mattermost-redux/src/selectors/entities/scheduled_posts.ts` - State selectors
- `webapp/channels/src/components/drafts/scheduled_post_list/` - UI components
- `webapp/channels/src/components/advanced_text_editor/scheduled_post_indicator/` - Indicators

## Configuration

Add to your `config.json`:

```json
{
  "ServiceSettings": {
    "ScheduledPosts": true
  }
}
```

Or via environment variable:
```bash
MM_SERVICESETTINGS_SCHEDULEDPOSTS=true
```

## Job Scheduling Details

- **Interval:** 5 minutes in production (`scheduledPostJobInterval`)
- **Debug Mode:** 2 seconds when `EnableTesting` is true
- **Cluster Mode:** Only runs on the leader node
- **Batch Size:** Processes 100 scheduled posts per batch
- **Timeout Window:** Processes posts scheduled within last 24 hours
- **Error Handling:** Failed posts are marked with error codes and not retried

## Database Schema

The `ScheduledPosts` table includes:
- Basic post fields (Message, ChannelId, UserId, etc.)
- `ScheduledAt` - When to send (UNIX timestamp in milliseconds)
- `ProcessedAt` - When it was actually sent
- `ErrorCode` - Error status if sending failed

## Maintenance

When updating Mattermost:
1. Check if license validation code changed in these 3 files
2. Re-apply the commented-out license checks if needed
3. Verify no new license checks were added elsewhere

## Testing

```bash
# Run server tests
cd server
make test-server TESTFLAGS="-run TestProcessScheduledPosts"

# Test scheduled posts API
curl -X POST http://localhost:8065/api/v4/posts/schedule \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "channel_id": "CHANNEL_ID",
    "message": "This is a scheduled post",
    "scheduled_at": 1704556800000
  }'
```

## No Custom Code Required

Unlike the cluster implementation (see `server/custom/szncluster/`), scheduled posts **do not require any custom code** in `server/custom/`. The feature is fully implemented in the open source codebase - we only removed artificial license restrictions.

## References

- API Spec: `api/v4/source/scheduled_post.yaml`
- Store Tests: `server/channels/store/storetest/scheduled_post_store.go`
- App Tests: `server/channels/app/scheduled_post_test.go`
- Job Tests: `server/channels/app/scheduled_post_job_test.go`

---

**Modified by:** Seznam.cz, a.s.  
**Date:** January 2026  
**Purpose:** Enable scheduled posts without enterprise license for internal use
