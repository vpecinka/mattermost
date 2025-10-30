# SznSearch - Custom Search Engine Implementation

## Přehled

SznSearch je custom implementace vyhledávacího enginu pro Mattermost, která **NEVYŽADUJE enterprise licenci**. Engine je postavený na ElasticSearch API, ale je kompletně oddělený od enterprise části kódu.

## Klíčové vlastnosti

- ✅ **Žádná závislost na enterprise licenci**
- ✅ **Vlastní konfigurace** - použití `SznSearchSettings` místo `ElasticsearchSettings`
- ✅ **Samostatná registrace** - registrace přes `RegisterSznSearchInterface()`
- ✅ **Plná podpora TLS** - client certificate authentication
- ✅ **Priorita před ElasticSearch** - pokud je SznSearch aktivní, použije se místo enterprise ES

## Architektura změn

### 1. Nová konfigurace (SznSearchSettings)

Přidána nová sekce do `server/public/model/config.go`:

```go
type SznSearchSettings struct {
    ConnectionURL         *string
    Username              *string
    Password              *string
    EnableIndexing        *bool
    EnableSearching       *bool
    EnableAutocomplete    *bool
    PostIndexReplicas     *int
    PostIndexShards       *int
    ChannelIndexReplicas  *int
    ChannelIndexShards    *int
    UserIndexReplicas     *int
    UserIndexShards       *int
    IndexPrefix           *string
    LiveIndexingBatchSize *int
    BatchSize             *int
    RequestTimeoutSeconds *int
    SkipTLSVerification   *bool
    CA                    *string
    ClientCert            *string
    ClientKey             *string
}
```

Sekce je přidána do hlavní `Config` struktury:

```go
type Config struct {
    // ... ostatní nastavení
    ElasticsearchSettings ElasticsearchSettings
    SznSearchSettings     SznSearchSettings  // <-- NOVÉ
    DataRetentionSettings DataRetentionSettings
    // ...
}
```

### 2. Registrační mechanismus

V `server/channels/app/platform/enterprise.go`:

```go
var sznSearchInterface func(*PlatformService) searchengine.SearchEngineInterface

func RegisterSznSearchInterface(f func(*PlatformService) searchengine.SearchEngineInterface) {
    sznSearchInterface = f
}
```

### 3. SearchEngine Broker rozšíření

V `server/platform/services/searchengine/searchengine.go`:

```go
type Broker struct {
    cfg                 *model.Config
    ElasticsearchEngine SearchEngineInterface
    SznSearchEngine     SearchEngineInterface  // <-- NOVÉ
}

func (seb *Broker) RegisterSznSearchEngine(szn SearchEngineInterface) {
    seb.SznSearchEngine = szn
}

func (seb *Broker) GetActiveEngines() []SearchEngineInterface {
    engines := []SearchEngineInterface{}
    
    // Priorita: SznSearch > Elasticsearch
    if seb.SznSearchEngine != nil && seb.SznSearchEngine.IsActive() {
        engines = append(engines, seb.SznSearchEngine)
    } else if seb.ElasticsearchEngine != nil && seb.ElasticsearchEngine.IsActive() {
        engines = append(engines, seb.ElasticsearchEngine)
    }
    
    return engines
}
```

### 4. Lifecycle management

V `server/channels/app/platform/searchengine.go`:

- **StartSearchEngine()** - spouští SznSearch engine při startu serveru
- **StopSearchEngine()** - zastavuje engine při shutdown
- **Config listener** - reaguje na změny konfigurace a restartuje engine

### 5. SznSearch implementace

Upravený `server/custom/sznsearch/sznsearch/sznsearch.go`:

- Používá `Platform.Config().SznSearchSettings` místo `ElasticsearchSettings`
- Všechny metody (`IsEnabled`, `IsActive`, `IsIndexingEnabled`, atd.) čtou z nové konfigurace

Upravený `server/custom/sznsearch/sznsearch/init.go`:

```go
func init() {
    // Registrace bez závislosti na enterprise
    platform.RegisterSznSearchInterface(func(ps *platform.PlatformService) searchengine.SearchEngineInterface {
        return NewSznSearchEngine(ps)
    })
}
```

## Konfigurace

### Příklad config.json

```json
{
  "SznSearchSettings": {
    "ConnectionURL": "https://your-elasticsearch-host:9200",
    "Username": "",
    "Password": "",
    "EnableIndexing": true,
    "EnableSearching": true,
    "EnableAutocomplete": true,
    "PostIndexReplicas": 1,
    "PostIndexShards": 1,
    "ChannelIndexReplicas": 1,
    "ChannelIndexShards": 1,
    "UserIndexReplicas": 1,
    "UserIndexShards": 1,
    "IndexPrefix": "mattermost",
    "LiveIndexingBatchSize": 1,
    "BatchSize": 10000,
    "RequestTimeoutSeconds": 30,
    "SkipTLSVerification": false,
    "CA": "/path/to/ca.crt",
    "ClientCert": "/path/to/client.crt",
    "ClientKey": "/path/to/client.key"
  }
}
```

### Povinné parametry

- `ConnectionURL` - URL ElasticSearch serveru
- `EnableIndexing` - zapne indexování (true/false)
- `EnableSearching` - zapne vyhledávání (true/false)

### TLS autentizace

Pro použití client certificate authentication:

1. Nastavte `ClientCert` na cestu k client certifikátu
2. Nastavte `ClientKey` na cestu k private key
3. Volitelně nastavte `CA` na cestu k CA certifikátu

## Spuštění

### 1. Build serveru s custom modulem

```bash
cd server
go build -o mattermost ./cmd/mattermost
```

### 2. Spuštění s konfigurací

```bash
./mattermost --config=config.json
```

### 3. Ověření v logu

```
INFO SznSearch: Creating new engine instance
INFO SznSearch: Creating ElasticSearch client connection
INFO SznSearch: Testing ElasticSearch connection
INFO SznSearch: Connection test successful
INFO SznSearch: Ensuring indices exist
INFO SznSearch: Indices verified successfully
INFO SznSearch: Engine started successfully, launching background indexer
```

## Výhody tohoto řešení

1. **Žádná enterprise licence** - SznSearch je plně funkční bez licence
2. **Čistá separace** - žádný konflikt s enterprise ElasticSearch
3. **Jednoduchá konfigurace** - vlastní sekce v config.json
4. **Automatická aktivace** - engine se inicializuje automaticky při startu
5. **Priorita** - pokud je aktivní, má přednost před enterprise ES
6. **Hot reload** - změny konfigurace se aplikují za běhu

## Testování

```bash
# Zkontrolovat, že se engine zaregistroval
grep "SznSearch" logs/mattermost.log

# Testovat connection
curl -X POST http://localhost:8065/api/v4/elasticsearch/test

# Zkontrolovat aktivní engine
# V administraci nebo v logu by mělo být "sznsearch"
```

## Troubleshooting

### Engine se nespustí

- Zkontrolujte `EnableIndexing: true` v konfiguraci
- Ověřte `ConnectionURL` ElasticSearch serveru
- Zkontrolujte TLS certifikáty (pokud používáte)

### Konflikt s enterprise ES

- SznSearch má prioritu - pokud je aktivní, enterprise ES se nepoužije
- Můžete vypnout enterprise ES nastavením `ElasticsearchSettings.EnableIndexing: false`

### Indexování nefunguje

- Zkontrolujte logy: `grep "SznSearch" logs/mattermost.log`
- Ověřte připojení k ElasticSearch
- Zkontrolujte práva na indexy

## Soubory upravené v tomto řešení

1. **server/public/model/config.go** - přidána `SznSearchSettings` struktura
2. **server/channels/app/platform/enterprise.go** - přidána `RegisterSznSearchInterface()`
3. **server/platform/services/searchengine/searchengine.go** - rozšířen Broker o `SznSearchEngine`
4. **server/channels/app/platform/searchengine.go** - lifecycle management pro SznSearch
5. **server/channels/app/platform/service.go** - registrace v `initEnterprise()`
6. **server/custom/sznsearch/sznsearch/sznsearch.go** - použití `SznSearchSettings`
7. **server/custom/sznsearch/sznsearch/init.go** - registrace přes `RegisterSznSearchInterface()`

## Další kroky

1. Otestovat build a spuštění
2. Ověřit indexování dat
3. Otestovat vyhledávání v UI
4. Případně přidat další konfigurace specifické pro Seznam.cz ElasticSearch

---

**Autor**: GitHub Copilot  
**Datum**: 30. října 2025
