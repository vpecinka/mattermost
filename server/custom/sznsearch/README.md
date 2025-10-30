# SznSearch - Custom ElasticSearch Engine for Mattermost

Custom implementation of ElasticSearch-based full-text search engine for **Mattermost Team Edition**, inspired by the Enterprise Edition elasticsearch implementation.

**No license required** - works out of the box with Team Edition!

## Features

- **Czech language support**: Custom Hunspell analyzer for Czech language
- **Async indexing**: Background worker processes message queue for non-blocking indexing
- **Channel state tracking**: Persistent indexing state per channel
- **TLS client authentication**: mTLS support via config.json
- **Direct message filtering**: Automatic member-based filtering for DM/GM channels
- **Team Edition compatible**: No Enterprise license required

## Architecture

### Components

1. **SznSearchImpl**: Main engine implementing `searchengine.SearchEngineInterface`
2. **Background Indexer**: Async worker processing message queue every 5 seconds
3. **State Management**: Per-channel indexing state stored in ElasticSearch

### Directory Structure

```
server/custom/sznsearch/
├── common/
│   ├── types.go          # Shared types and constants
│   └── mutex.go          # Thread-safe keyed mutex
└── sznsearch/
    ├── sznsearch.go      # Main engine lifecycle
    ├── indexing.go       # Post indexing/deletion
    ├── search.go         # Search query building
    ├── indexer.go        # Background worker & batch indexing
    ├── indices.go        # Index creation with Czech analyzer
    ├── state.go          # Channel state persistence
    ├── channels_users_files.go  # Stub implementations
    └── init.go           # Engine registration
```

## Configuration

### Mattermost Config

Enable ElasticSearch in `config.json`:

```json
{
  "ElasticsearchSettings": {
    "ConnectionURL": "https://elasticsearch.example.com:9200",
    "Username": "elastic",
    "Password": "changeme",
    "EnableIndexing": true,
    "EnableSearching": true,
    "EnableAutocomplete": false,
    "LiveIndexingBatchSize": 10,
    "SkipTLSVerification": false,
    "CA": "",
    "ClientCert": "",
    "ClientKey": ""
  }
}
```

### Authentication Options

**Option 1: Basic Auth (simpler)**
```json
{
  "ElasticsearchSettings": {
    "ConnectionURL": "http://localhost:9200",
    "Username": "elastic",
    "Password": "changeme",
    "ClientCert": "",
    "ClientKey": "",
    "CA": "",
    "EnableIndexing": true,
    "EnableSearching": true,
    "LiveIndexingBatchSize": 10
  }
}
```

**Option 2: TLS Client Certificate (more secure)**
```json
{
  "ElasticsearchSettings": {
    "ConnectionURL": "https://elasticsearch.example.com:9200",
    "Username": "",
    "Password": "",
    "ClientCert": "/etc/mattermost/certs/client.crt",
    "ClientKey": "/etc/mattermost/certs/client.key",
    "CA": "/etc/mattermost/certs/ca.crt",
    "SkipTLSVerification": false,
    "EnableIndexing": true,
    "EnableSearching": true,
    "LiveIndexingBatchSize": 10
  }
}
```

**Important**: If `ClientCert` and `ClientKey` are set, they take precedence over `Username`/`Password`.

## Usage

### Automatic Registration

The engine automatically registers itself via `init()` in `init.go`. Just import the package in your Mattermost server:

```go
import _ "github.com/mattermost/mattermost/server/v8/custom/sznsearch/sznsearch"
```

### Realtime Indexing

Posts are automatically indexed when saved through the SearchLayer wrapper:

- `IndexPost()` is called automatically by `searchlayer.PostStore`
- Messages are queued for async indexing (default when `LiveIndexingBatchSize > 1`)
- Background worker processes queue every 5 seconds
- Synchronous indexing if `LiveIndexingBatchSize <= 1`

### No Bulk Indexing Job (Yet)

Bulk indexing job is not implemented in this version. All indexing happens in realtime as posts are created.

## ElasticSearch Indices

### Message Index: `messages`

Settings:
- 3 shards, 2 replicas
- Czech Hunspell analyzer with stop words

Mapping:
```json
{
  "Message": "text",      // Analyzed with Czech
  "Payload": "text",      // Attachment text
  "CreatedAt": "long",    // Unix timestamp (milliseconds)
  "ChannelId": "keyword", 
  "ChannelType": "byte",  // 0=Public, 1=Private, 2=DM, 3=GM
  "TeamId": "keyword",
  "UserId": "keyword",
  "Members": "keyword"    // Array of user IDs (DM/GM only)
}
```

### State Index: `state`

Stores per-channel indexing state:
```json
{
  "ChannelID": "string",
  "State": "string",       // stopped|starting|stopping|idle|indexing|done|error
  "StartedAt": "long",
  "FinishedAt": "long",
  "IndexedCount": "long",
  "Error": "string"
}
```

## Search Query Features

- **Full-text search**: Message and Payload fields
- **Channel filtering**: Search within specific channels
- **User filtering**: Search posts by specific users
- **Date range**: `on:`, `before:`, `after:` date filters
- **Phrase matching**: Quoted terms for exact phrase
- **Term matching**: Multiple terms with OR logic
- **Highlighting**: Results include highlighted matches
- **Permission-aware**: Automatic filtering by channel membership

## Development

### Testing

```bash
# Build server with custom search
cd server
make build

# Run tests (if any)
go test ./custom/sznsearch/...
```

### Debugging

Enable debug logging in `config.json`:

```json
{
  "LogSettings": {
    "ConsoleLevel": "DEBUG"
  }
}
```

Look for logs prefixed with `SznSearch`:
- `SznSearch engine started successfully`
- `SznSearch indexing batch size=100`

## Requirements

- **Mattermost Team Edition** (no license required)
- **ElasticSearch 8.x** cluster
- **Go 1.21+** for building

## Credits

Inspired by:
- Mattermost Enterprise Edition ElasticSearch implementation
- Custom ElasticSearch plugin in `server/tmp_plugin/server/esaas/`
