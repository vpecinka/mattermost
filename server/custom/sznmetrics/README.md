# SznMetrics - Custom Prometheus Metrics Implementation

**Copyright (c) 2024-present Seznam.cz, a.s.**
**All Rights Reserved.**

This is a custom implementation of Prometheus metrics support for Mattermost, developed by Seznam.cz for internal use. This implementation does NOT require an Enterprise license and is completely independent of the Enterprise metrics implementation.

## Overview

SznMetrics provides comprehensive Prometheus metrics collection for Mattermost server, covering:

- **Posts**: Post creation, webhooks, emails, push notifications, broadcasts
- **HTTP**: Request counts, errors, websocket connections, API endpoint durations
- **Cluster**: Cluster requests, event types, request durations
- **Login**: Successful and failed login attempts
- **Cache**: Memory cache hits/misses, ETag hits/misses, Redis operations
- **Websocket**: Events, broadcasts, buffer sizes, user registrations
- **Search**: Post/file searches, indexing operations, store method durations
- **Plugins**: Hook durations, API call durations
- **Remote Clusters**: Messages sent/received, ping durations, clock skew
- **Shared Channels**: Sync operations, queue sizes, collection/send durations
- **Jobs**: Active jobs, replica lag metrics
- **Notifications**: Notification counts by type and platform
- **Clients**: Web, mobile, and desktop client performance metrics
- **Access Control**: Search queries, expression compilation, evaluation durations

## Architecture

The implementation is split into multiple files for maintainability:

```
server/custom/sznmetrics/
├── init.go              # Registration of metrics interface
├── metrics.go           # Main structure and initialization
├── posts.go             # Post-related metrics
├── http.go              # HTTP and API metrics
├── cluster.go           # Cluster metrics
├── login.go             # Login metrics
├── cache.go             # Cache and ETag metrics
├── websocket.go         # WebSocket metrics
├── search.go            # Search and database metrics
├── plugin.go            # Plugin metrics
├── remote_cluster.go    # Remote cluster metrics
├── shared_channels.go   # Shared channels metrics
├── jobs.go              # Jobs and system metrics
├── notifications.go     # Notification metrics
├── clients.go           # Client performance metrics
├── access_control.go    # Access control metrics
└── README.md            # This file
```

## Configuration

Metrics are configured through Mattermost's standard configuration:

```json
{
  "MetricsSettings": {
    "Enable": true,
    "ListenAddress": ":8067"
  }
}
```

## Usage

Once the server is running with metrics enabled, Prometheus metrics are available at:

```
http://your-server:8067/metrics
```

You can also access profiling endpoints at the same address:
- `/debug/pprof/` - Profiling root
- `/debug/pprof/goroutine` - Goroutine profiles
- `/debug/pprof/heap` - Memory heap profiles
- etc.

## Prometheus Configuration

Example Prometheus scrape configuration:

```yaml
scrape_configs:
  - job_name: 'mattermost'
    static_configs:
      - targets: ['your-mattermost-server:8067']
    scrape_interval: 15s
```

## License Compliance

This implementation is:
- Completely independent from Mattermost Enterprise code
- Does NOT use any code from `server/enterprise/` directory
- Implements the public interface defined in `server/einterfaces/metrics.go`
- Developed from scratch by Seznam.cz for internal use
- Does NOT violate Mattermost's licensing terms

The Enterprise metrics implementation remains in `server/enterprise/metrics/` but is not imported or used when this custom implementation is active.

## Integration

The metrics interface is registered in `init.go` via:

```go
platform.RegisterMetricsInterface(func(ps *platform.PlatformService, driver, dataSource string) einterfaces.MetricsInterface {
    return New(ps, driver, dataSource)
})
```

This registration happens at package initialization time, before the Enterprise package is loaded (if present), ensuring our implementation takes precedence.

## Maintenance

When updating Mattermost to newer versions, check if new methods have been added to `einterfaces.MetricsInterface` and implement them accordingly in the appropriate file.

## Support

This is an internal Seznam.cz implementation. For issues or questions, contact the internal infrastructure team.

---

**Built with ❤️ by Seznam.cz Infrastructure Team**
