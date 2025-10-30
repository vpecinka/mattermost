# Shrnutí implementace SznSearch bez závislosti na Enterprise

## Problém

Původní implementace `sznsearch` používala:
- ❌ `ElasticsearchSettings` z config.json (který je vázaný na enterprise)
- ❌ `RegisterElasticsearchInterface()` (vyžaduje enterprise licenci)
- ❌ Inicializace závisela na enterprise build a licenci

## Řešení

Vytvořena **kompletně samostatná infrastruktura** pro SznSearch:

### 1. Nová konfigurace (`SznSearchSettings`)

```go
// server/public/model/config.go
type Config struct {
    // ...
    ElasticsearchSettings ElasticsearchSettings  // Enterprise
    SznSearchSettings     SznSearchSettings      // Custom - NOVÉ
    // ...
}
```

**Vlastnosti:**
- Oddělená od `ElasticsearchSettings`
- Žádné enterprise tagy v access control
- Automatické výchozí hodnoty přes `SetDefaults()`

### 2. Samostatný registrační mechanismus

```go
// server/channels/app/platform/enterprise.go
var sznSearchInterface func(*PlatformService) searchengine.SearchEngineInterface

func RegisterSznSearchInterface(f func(*PlatformService) searchengine.SearchEngineInterface) {
    sznSearchInterface = f
}
```

**Výhody:**
- Nezávislá registrace na enterprise kódu
- Volána v `initEnterprise()` bez kontroly licence

### 3. Rozšíření SearchEngine Broker

```go
// server/platform/services/searchengine/searchengine.go
type Broker struct {
    cfg                 *model.Config
    ElasticsearchEngine SearchEngineInterface  // Enterprise
    SznSearchEngine     SearchEngineInterface  // Custom - NOVÉ
}
```

**Priorita engineů:**
1. SznSearchEngine (pokud aktivní)
2. ElasticsearchEngine (fallback)
3. Database search

### 4. Lifecycle management

**StartSearchEngine():**
- Spouští SznSearch engine při startu (bez kontroly licence)
- Config listener reaguje na změny `SznSearchSettings`
- Auto-restart při změně credentials

**StopSearchEngine():**
- Korektní shutdown obou engineů
- Cleanup resources

### 5. Aktualizace SznSearch implementace

**Změny v `sznsearch.go`:**
```go
// PŘED
func (s *SznSearchImpl) IsEnabled() bool {
    return *s.Platform.Config().ElasticsearchSettings.EnableIndexing
}

// PO
func (s *SznSearchImpl) IsEnabled() bool {
    return *s.Platform.Config().SznSearchSettings.EnableIndexing
}
```

**Změny v `init.go`:**
```go
// PŘED
platform.RegisterElasticsearchInterface(func(ps *platform.PlatformService) searchengine.SearchEngineInterface {
    return NewSznSearchEngine(ps)
})

// PO
platform.RegisterSznSearchInterface(func(ps *platform.PlatformService) searchengine.SearchEngineInterface {
    return NewSznSearchEngine(ps)
})
```

## Výsledek

✅ **Žádná závislost na enterprise licenci**  
✅ **Vlastní konfigurace v config.json**  
✅ **Automatická inicializace při startu**  
✅ **Priorita před enterprise ElasticSearch**  
✅ **Hot reload při změně konfigurace**  
✅ **Plná podpora TLS client authentication**

## Konfigurace v praxi

**Minimální config.json:**
```json
{
  "SznSearchSettings": {
    "ConnectionURL": "https://elasticsearch:9200",
    "EnableIndexing": true,
    "EnableSearching": true,
    "ClientCert": "/certs/client.crt",
    "ClientKey": "/certs/client.key",
    "CA": "/certs/ca.crt"
  }
}
```

## Testování

```bash
# Build
cd server
make build

# Spustit
./mattermost --config=config.json

# Ověřit v logu
grep "SznSearch" logs/mattermost.log
```

**Očekávaný výstup:**
```
INFO SznSearch: Creating new engine instance
INFO SznSearch: Engine started successfully
```

## Upravené soubory

| Soubor | Změna |
|--------|-------|
| `server/public/model/config.go` | Přidána `SznSearchSettings` struktura + integrace do `Config` |
| `server/channels/app/platform/enterprise.go` | Přidána `RegisterSznSearchInterface()` |
| `server/platform/services/searchengine/searchengine.go` | Rozšířen `Broker` o `SznSearchEngine` |
| `server/channels/app/platform/searchengine.go` | Lifecycle management pro SznSearch |
| `server/channels/app/platform/service.go` | Registrace v `initEnterprise()` |
| `server/custom/sznsearch/sznsearch/sznsearch.go` | Použití `SznSearchSettings` |
| `server/custom/sznsearch/sznsearch/init.go` | Nová registrace |

## Další dokumentace

- **IMPLEMENTATION.md** - detailní technická dokumentace
- **config.example.json** - příklad konfigurace

---

**Stav:** ✅ Implementováno a připraveno k testování  
**Bez závislostí na:** Enterprise licence, ElasticsearchSettings  
**Kompatibilní s:** Team Edition i Enterprise Edition
