# Референс публичного API SDK CLIProxyAPI v7

> **Назначение:** полный справочник экспортируемых сущностей ядра
> `github.com/router-for-me/CLIProxyAPI/v7` (ветка `main`), которые использует
> бизнес-слой CLIProxyNew. Основан на анализе исходников.
> **Связанные:** [ADR-9](adr/ADR-9-sdk-contracts.md), [architecture.md](architecture.md).
>
> Путь импорта: `github.com/router-for-me/CLIProxyAPI/v7/sdk/...`

## Содержание

- [sdk/cliproxy](#sdkcliproxy—точка-входа) — `Service`, `Builder`, `Hooks`, types
- [sdk/cliproxy/auth](#sdkcliproxyauth—ядро-аутентификации-и-исполнения) — `Auth`, `Manager`, `Store`, `Selector`, `Hook`
- [sdk/cliproxy/executor](#sdkcliproxyexecutor—типы-запросовresponses) — `Request`, `Options`, `Response`
- [sdk/cliproxy/usage](#sdkcliproxyusage—аналитика) — `Plugin`, `Record`
- [sdk/auth](#sdkauth—upstream-credential-lifecycle) — OAuth-login authenticators
- [sdk/access](#sdkaccess—проверка-клиентских-api-keys) — `Provider`, `Manager`
- [sdk/api](#sdkapi—http-server-options--handlers) — `ServerOption`, `BaseAPIHandler`
- [sdk/config](#sdkconfig—конфигурация) — `Config`, `LoadConfig`

---

## `sdk/cliproxy` — точка входа

Главный пакет. `Service` оборачивает lifecycle прокси-сервера для встраивания.

### `Service` (`service.go`)

```go
type Service struct { /* unexported fields */ }

func (s *Service) Run(ctx context.Context) error              // блокирует до ctx.Done или ошибки сервера
func (s *Service) Shutdown(ctx context.Context) error         // идемпотентный (sync.Once)
func (s *Service) RegisterUsagePlugin(plugin usage.Plugin)   // регистрация плагина аналитики
```

`Run` внутри: `usage.StartDefault`, `coreManager.Load(ctx)`, `tokenProvider.Load`,
`apiKeyProvider.Load`, `api.NewServer`, запуск watcher,
`coreManager.StartAutoRefresh(ctx, 15*time.Minute)`,
`registerModelRefreshCallback()`, блок на `select { ctx.Done / serverErr }`.

### `Builder` (`builder.go`)

```go
type Builder struct { /* unexported fields */ }

func NewBuilder() *Builder

func (b *Builder) WithConfig(cfg *config.Config) *Builder
func (b *Builder) WithConfigPath(path string) *Builder                    // ОБЯЗАТЕЛЕН
func (b *Builder) WithTokenClientProvider(provider TokenClientProvider) *Builder
func (b *Builder) WithAPIKeyClientProvider(provider APIKeyClientProvider) *Builder
func (b *Builder) WithWatcherFactory(factory WatcherFactory) *Builder
func (b *Builder) WithHooks(h Hooks) *Builder
func (b *Builder) WithAuthManager(mgr *sdkAuth.Manager) *Builder
func (b *Builder) WithRequestAccessManager(mgr *sdkaccess.Manager) *Builder
func (b *Builder) WithCoreAuthManager(mgr *coreauth.Manager) *Builder
func (b *Builder) WithPluginHost(host *pluginhost.Host) *Builder
func (b *Builder) WithServerOptions(opts ...api.ServerOption) *Builder
func (b *Builder) WithLocalManagementPassword(password string) *Builder
func (b *Builder) WithPostAuthHook(hook coreauth.PostAuthHook) *Builder
func (b *Builder) Build() (*Service, error)   // ошибки если cfg==nil или configPath==""
```

`Build()` применяет дефолты: `NewFileTokenClientProvider()`,
`NewAPIKeyClientProvider()`, `defaultWatcherFactory`, `newDefaultAuthManager()`,
`sdkaccess.NewManager()`, `pluginhost.New()`,
`coreManager.SetRoundTripperProvider(newDefaultRoundTripperProvider())`.

### `Hooks` (`builder.go`)

```go
type Hooks struct {
    OnBeforeStart func(*config.Config)   // до старта сервиса
    OnAfterStart  func(*Service)         // после успешного старта
}
```

### Types (`types.go`)

```go
// Провайдеры клиентов
type TokenClientProvider interface {
    Load(ctx context.Context, cfg *config.Config) (*TokenClientResult, error)
}
type TokenClientResult struct { SuccessfulAuthed int }

type APIKeyClientProvider interface {
    Load(ctx context.Context, cfg *config.Config) (*APIKeyClientResult, error)
}
type APIKeyClientResult struct {
    GeminiKeyCount, VertexCompatKeyCount, ClaudeKeyCount int
    CodexKeyCount, OpenAICompatCount                      int
}

// Watcher
type WatcherFactory func(configPath, authDir string, reload func(*config.Config)) (*WatcherWrapper, error)

// Plugin auth parsers
type PluginAuthParser interface {
    ParseAuth(context.Context, pluginapi.AuthParseRequest) (*coreauth.Auth, bool, error)
}
type PluginMultiAuthParser interface {
    ParseAuths(context.Context, pluginapi.AuthParseRequest) ([]*coreauth.Auth, bool, error)
}
```

### `WatcherWrapper` (`types.go`) — публичная обёртка над `internal/watcher`

```go
type WatcherWrapper struct { /* unexported func fields */ }

func (w *WatcherWrapper) Start(ctx context.Context) error
func (w *WatcherWrapper) Stop() error
func (w *WatcherWrapper) SetConfig(cfg *config.Config)
func (w *WatcherWrapper) ReloadConfigIfChanged() bool
func (w *WatcherWrapper) SetPluginAuthParser(parser PluginAuthParser)
func (w *WatcherWrapper) DispatchRuntimeAuthUpdate(update watcher.AuthUpdate) bool
func (w *WatcherWrapper) DispatchPersistedAuthUpdate(update watcher.AuthUpdate) bool
func (w *WatcherWrapper) SnapshotAuths() []*coreauth.Auth
func (w *WatcherWrapper) SetAuthUpdateQueue(queue chan<- watcher.AuthUpdate)
```

### Model registry (`model_registry.go`)

```go
type ModelInfo = registry.ModelInfo    // re-export
type ModelRegistryHook = registry.ModelRegistryHook

type ModelRegistry interface {
    RegisterClient(clientID, clientProvider string, models []*ModelInfo)
    UnregisterClient(clientID string)
    SetModelQuotaExceeded(clientID, modelID string)
    ClearModelQuotaExceeded(clientID, modelID string)
    ClientSupportsModel(clientID, modelID string) bool
    GetAvailableModels(handlerType string) []map[string]any
    GetAvailableModelsByProvider(provider string) []*ModelInfo
}

func GlobalModelRegistry() ModelRegistry
func SetGlobalModelRegistryHook(hook ModelRegistryHook)

// ModelRegistryHook — точка интеграции бизнес-слоя (зеркало в Postgres)
type ModelRegistryHook interface {
    OnModelsRegistered(ctx context.Context, provider, clientID string, models []*ModelInfo)
    OnModelsUnregistered(ctx context.Context, provider, clientID string)
}
```

### Провайдеры по умолчанию (`providers.go`)

```go
func NewFileTokenClientProvider() TokenClientProvider   // stateless no-op
func NewAPIKeyClientProvider() APIKeyClientProvider     // watcher.BuildAPIKeyClients(cfg)
```

---

## `sdk/cliproxy/auth` — ядро аутентификации и исполнения

Критический пакет: `Manager`, `Store`, `Selector`, `Hook`, `Auth`.

### `Auth` (`types.go`) — центральная сущность credential

```go
type Auth struct {
    ID               string                 `json:"id"`                  // управляется ядром
    Index            string                 `json:"-"`                   // runtime-ид
    Provider         string                 `json:"provider"`
    Prefix           string                 `json:"prefix,omitempty"`
    FileName         string                 `json:"-"`
    Storage          baseauth.TokenStorage  `json:"-"`                   // реализация persistence токенов
    Label            string                 `json:"label,omitempty"`
    Status           Status                 `json:"status"`
    StatusMessage    string                 `json:"status_message,omitempty"`
    Disabled         bool                   `json:"disabled"`
    Unavailable      bool                   `json:"unavailable"`
    ProxyURL         string                 `json:"proxy_url,omitempty"` // per-account explicit SDK override
    Attributes       map[string]string      `json:"attributes,omitempty"` // immutable: base_url, api_key, ...
    Metadata         map[string]any         `json:"metadata,omitempty"`   // mutable: токены, email, expired
    Quota            QuotaState             `json:"quota"`
    LastError        *Error                 `json:"last_error,omitempty"`
    CreatedAt        time.Time              `json:"created_at"`
    UpdatedAt        time.Time              `json:"updated_at"`
    LastRefreshedAt  time.Time              `json:"last_refreshed_at"`
    NextRefreshAfter time.Time              `json:"next_refresh_after"`
    NextRetryAfter   time.Time              `json:"next_retry_after"`
    ModelStates      map[string]*ModelState `json:"model_states,omitempty"`
    Runtime          any                    `json:"-"`                    // non-serializable
    Success          int64                  `json:"-"`
    Failed           int64                  `json:"-"`
}

// Методы *Auth
func (a *Auth) Clone() *Auth
func (a *Auth) EnsureIndex() string
func (a *Auth) ProxyInfo() string
func (a *Auth) AccountInfo() (authType, identity string)   // ("oauth"|"api_key", identity)
func (a *Auth) ExpirationTime() (time.Time, bool)
func (a *Auth) AuthKind() string                            // "apikey"|"oauth"|""
func (a *Auth) AuthSourceKind() string
func (a *Auth) RecentRequestsSnapshot(now time.Time) []RecentRequestBucket
```

Вспомогательные типы:
```go
type Status string
const (
    StatusUnknown    Status = "unknown"
    StatusActive     Status = "active"
    StatusPending    Status = "pending"
    StatusRefreshing Status = "refreshing"
    StatusError      Status = "error"
    StatusDisabled   Status = "disabled"
)

type Error struct {
    Code       string `json:"code,omitempty"`
    Message    string `json:"message"`
    Retryable  bool   `json:"retryable"`
    HTTPStatus int    `json:"http_status,omitempty"`
}
func (e *Error) Error() string
func (e *Error) StatusCode() int

type QuotaState struct {
    Exceeded      bool      `json:"exceeded"`
    Reason        string    `json:"reason,omitempty"`
    NextRecoverAt time.Time `json:"next_recover_at"`
    BackoffLevel  int       `json:"backoff_level,omitempty"`
}

type ModelState struct {
    Status         Status     `json:"status"`
    StatusMessage  string     `json:"status_message,omitempty"`
    Unavailable    bool       `json:"unavailable"`
    NextRetryAfter time.Time  `json:"next_retry_after"`
    LastError      *Error     `json:"last_error,omitempty"`
    Quota          QuotaState `json:"quota"`
    UpdatedAt      time.Time  `json:"updated_at"`
}

type PostAuthHook func(context.Context, *Auth) error   // после создания Auth, до persistence

type RecentRequestBucket struct {
    Time    string
    Success int64
    Failed  int64
}
```

### `Store` (`store.go`) — **контракт 1 бизнес-слоя** (ADR-9)

```go
type Store interface {
    List(ctx context.Context) ([]*Auth, error)
    Save(ctx context.Context, auth *Auth) (string, error)
    Delete(ctx context.Context, id string) error
}
```

### `Manager` (`conductor.go`) — оркестратор

```go
type Manager struct { /* unexported */ }

func NewManager(store Store, selector Selector, hook Hook) *Manager
// nil selector → RoundRobinSelector{}; nil hook → NoopHook{}
```

#### Методы исполнения

```go
func (m *Manager) Execute(ctx context.Context, providers []string, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error)
func (m *Manager) ExecuteCount(ctx context.Context, providers []string, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error)
func (m *Manager) ExecuteStream(ctx context.Context, providers []string, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error)
func (m *Manager) MarkResult(ctx context.Context, result Result)
```

#### Реестр auth/executors

```go
func (m *Manager) Register(ctx context.Context, auth *Auth) (*Auth, error)
func (m *Manager) Update(ctx context.Context, auth *Auth) (*Auth, error)
func (m *Manager) Remove(ctx context.Context, id string)
func (m *Manager) Load(ctx context.Context) error
func (m *Manager) List() []*Auth
func (m *Manager) GetByID(id string) (*Auth, bool)
func (m *Manager) RegisterExecutor(executor ProviderExecutor)
func (m *Manager) UnregisterExecutor(provider string)
func (m *Manager) Executor(provider string) (ProviderExecutor, bool)
func (m *Manager) HasProviderAuth(provider string) bool
func (m *Manager) AvailableProviders() []string
```

#### Auto-refresh

```go
func (m *Manager) StartAutoRefresh(parent context.Context, interval time.Duration)  // R7: ядро само
func (m *Manager) StopAutoRefresh()
func (m *Manager) RefreshSchedulerEntry(authID string)
func (m *Manager) RefreshSchedulerAll()
```

⚠️ `StartAutoRefresh` вызывает `ProviderExecutor.Refresh` **минуя `Selector.Pick`**.
В проекте R10 это не меняет proxy policy: пустой `Auth.ProxyURL` оставляет
system proxy процесса для refresh и inference. См. [ADR-10](adr/ADR-10-per-call-type-proxy.md).

#### HTTP / RoundTripper

```go
func (m *Manager) SetRoundTripperProvider(p RoundTripperProvider)
func (m *Manager) InjectCredentials(req *http.Request, authID string) error
func (m *Manager) PrepareHttpRequest(ctx context.Context, auth *Auth, req *http.Request) error
func (m *Manager) NewHttpRequest(ctx context.Context, auth *Auth, method, targetURL string, body []byte, headers http.Header) (*http.Request, error)
func (m *Manager) HttpRequest(ctx context.Context, auth *Auth, req *http.Request) (*http.Response, error)
```

#### Конфигурация

```go
func (m *Manager) SetSelector(selector Selector)
func (m *Manager) SetStore(store Store)
func (m *Manager) SetConfig(cfg *internalconfig.Config)
func (m *Manager) SetRetryConfig(retry int, maxRetryInterval time.Duration, maxRetryCredentials int)
func (m *Manager) SetOAuthModelAlias(aliases map[string][]internalconfig.OAuthModelAlias)
func (m *Manager) ResetQuota(ctx context.Context, authID string) (*Auth, []string, error)
```

### Интерфейсы (`conductor.go`)

```go
// Контракт 2 бизнес-слоя (ADR-9) — выбор auth под запрос
type Selector interface {
    Pick(ctx context.Context, provider, model string, opts cliproxyexecutor.Options, auths []*Auth) (*Auth, error)
}

// Контракт 3 бизнес-слоя (ADR-9) — аналитика/наблюдение
type Hook interface {
    OnAuthRegistered(ctx context.Context, auth *Auth)
    OnAuthUpdated(ctx context.Context, auth *Auth)
    OnResult(ctx context.Context, result Result)
}
type NoopHook struct{}  // встраивается для частичной реализации

// Исполнитель провайдера (реализуется ядром, не бизнес-слоем)
type ProviderExecutor interface {
    Identifier() string
    Execute(ctx context.Context, auth *Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error)
    ExecuteStream(ctx context.Context, auth *Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error)
    Refresh(ctx context.Context, auth *Auth) (*Auth, error)
    CountTokens(ctx context.Context, auth *Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error)
    HttpRequest(ctx context.Context, auth *Auth, req *http.Request) (*http.Response, error)
}

type RoundTripperProvider interface {
    RoundTripperFor(auth *Auth) http.RoundTripper   // выбирает transport по auth
}

type RefreshEvaluator interface {
    ShouldRefresh(now time.Time, auth *Auth) bool
}

type Result struct {
    AuthID     string
    Provider   string
    Model      string
    Success    bool
    RetryAfter *time.Duration
    Error      *Error
}
```

### Готовые реализации Selector (`selector.go`)

```go
type RoundRobinSelector struct{}
func (s *RoundRobinSelector) Pick(ctx context.Context, provider, model string, opts cliproxyexecutor.Options, auths []*Auth) (*Auth, error)

type FillFirstSelector struct{}
func (s *FillFirstSelector) Pick(ctx context.Context, provider, model string, opts cliproxyexecutor.Options, auths []*Auth) (*Auth, error)

type SessionAffinitySelector struct{}
func NewSessionAffinitySelector(fallback Selector) *SessionAffinitySelector
func (s *SessionAffinitySelector) Pick(...) (*Auth, error)
func (s *SessionAffinitySelector) Stop()
func (s *SessionAffinitySelector) InvalidateAuth(authID string)
```

### Cooldown persistence (`cooldown_state.go`)

```go
type CooldownStateRecord struct {
    Provider, AuthID, AuthFile, Model, Status, Reason string
    NextRetryAfter                                     time.Time
    Quota                                              QuotaState
    LastError                                          *Error
    UpdatedAt                                          time.Time
}

type CooldownStateStore interface {
    Load(context.Context) ([]CooldownStateRecord, error)
    Save(context.Context, []CooldownStateRecord) error
}

type FileCooldownStateStore struct{}
func NewFileCooldownStateStore(dir string) *FileCooldownStateStore
```

### Константы классификации (`classification.go`)

```go
const (
    AuthKindAPIKey = "apikey"
    AuthKindOAuth  = "oauth"
    AuthSourceConfig      = "config"
    AuthSourceFile        = "file"
    AuthSourcePostgres    = "postgres"   // ← наш случай
    AttributeAPIKey       = "api_key"
    AttributeAuthKind     = "auth_kind"
    AttributeSource       = "source"
)
```

---

## `sdk/cliproxy/executor` — типы запросов/ответов

### Types (`types.go`)

```go
type Request struct {
    Model    string               // upstream-модель после трансляции
    Payload  []byte               // provider-specific JSON
    Format   sdktranslator.Format
    Metadata map[string]any
}

type Options struct {
    Stream                      bool
    Alt                         string
    Headers                     http.Header
    Query                       url.Values
    OriginalRequest             []byte
    SourceFormat                sdktranslator.Format
    ResponseFormat              sdktranslator.Format   // "" → SourceFormat
    Metadata                    map[string]any
    RequestAfterAuthInterceptor RequestAfterAuthInterceptor
}

type Response struct {
    Payload  []byte
    Metadata map[string]any
    Headers  http.Header
}

type StreamChunk struct {
    Payload []byte
    Err     error
}

type StreamResult struct {
    Headers http.Header
    Chunks  <-chan StreamChunk
}

type StatusError interface {
    error
    StatusCode() int
}
```

### Константы метадата-ключей

```go
const RequestedModelMetadataKey     = "requested_model"
const RequestPathMetadataKey        = "request_path"
const DisallowFreeAuthMetadataKey   = "disallow_free_auth"
const AuthSelectionModelMetadataKey = "auth_selection_model"
const ReasoningEffortMetadataKey    = "reasoning_effort"
const ServiceTierMetadataKey        = "service_tier"
const PinnedAuthMetadataKey         = "pinned_auth_id"
const SelectedAuthMetadataKey       = "selected_auth_id"
const ExecutionSessionMetadataKey   = "execution_session_id"
```

### RequestAfterAuthInterceptor

```go
type RequestAfterAuthInterceptor func(context.Context, RequestAfterAuthInterceptRequest) RequestAfterAuthInterceptResponse

type RequestAfterAuthInterceptRequest struct {
    SourceFormat, ToFormat sdktranslator.Format
    Model, RequestedModel  string
    Stream                 bool
    Headers                http.Header
    Body                   []byte
    Metadata               map[string]any
}

type RequestAfterAuthInterceptResponse struct {
    Headers      http.Header
    Body         []byte
    ClearHeaders []string
}
```

---

## `sdk/cliproxy/usage` — аналитика

### `Plugin` и `Record` (`manager.go`) — **контракт 3 бизнес-слоя** (ADR-9)

```go
type Plugin interface {
    HandleUsage(ctx context.Context, record Record)
}

type Record struct {
    Provider            string
    ExecutorType        string
    Model               string
    Alias               string
    APIKey              string
    AuthID              string
    AuthIndex           string
    AuthType            string
    Source              string
    ReasoningEffort     string
    ServiceTier         string
    RequestServiceTier  string
    ResponseServiceTier string
    RequestedAt         time.Time
    Latency             time.Duration
    TTFT                time.Duration
    Failed              bool
    Fail                Failure
    Detail              Detail
    ResponseHeaders     http.Header
}

type Failure struct {
    StatusCode int
    Body       string
}

type Detail struct {
    InputTokens         int64
    OutputTokens        int64
    ReasoningTokens     int64
    CachedTokens        int64
    CacheReadTokens     int64
    CacheCreationTokens int64
    TotalTokens         int64
    ResponseServiceTier string
}
```

### Manager и free functions

```go
type Manager struct { /* unexported */ }

func NewManager(buffer int) *Manager                    // NB: buffer фактически не используется
func DefaultManager() *Manager                          // синглтон
func (m *Manager) Start(ctx context.Context)
func (m *Manager) Stop()
func (m *Manager) Register(plugin Plugin)
func (m *Manager) RegisterNamed(name string, plugin Plugin)
func (m *Manager) Publish(ctx context.Context, record Record)

// Пакетные функции (поверх DefaultManager)
func RegisterPlugin(plugin Plugin)
func RegisterNamedPlugin(name string, plugin Plugin)
func PublishRecord(ctx context.Context, record Record)
func StartDefault(ctx context.Context)
func StopDefault()
```

### Context-хелперы

```go
func WithRequestedModelAlias(ctx context.Context, alias string) context.Context
func RequestedModelAliasFromContext(ctx context.Context) string
func WithReasoningEffort(ctx context.Context, effort string) context.Context
func ReasoningEffortFromContext(ctx context.Context) string
func WithServiceTier(ctx context.Context, tier string) context.Context
func ServiceTierFromContext(ctx context.Context) string
```

⚠️ **R3 стриминг:** `HandleUsage` может вызываться асинхронно после отмены context.
Principal/user_id копируется ядром в `Record.APIKey` при старте запроса, не из context в момент `HandleUsage`.

**Совместимость (SDK v7.2.71):** публичный `usage.Record` не содержит отдельного
поля API-key ID, но переносит `Record.APIKey` из access principal. Бизнес-слой
кодирует в нём versioned пару `user_id/api_key_id` и декодирует её в
`usage.Plugin`; это не требует upstream `internal/*` или reflect-обходов (R12).

---

## `sdk/auth` — upstream credential lifecycle

OAuth login/refresh для провайдеров (Codex, Claude, Gemini/Antigravity, Kimi, xAI).

### `Manager` (`manager.go`)

```go
type Manager struct { /* unexported */ }

func NewManager(store coreauth.Store, authenticators ...Authenticator) *Manager
func (m *Manager) Register(a Authenticator)
func (m *Manager) SetStore(store coreauth.Store)
func (m *Manager) Login(ctx context.Context, provider string, cfg *config.Config, opts *LoginOptions) (*coreauth.Auth, string, error)
// возвращает (auth, savedPath, error). При store==nil — без сохранения.
```

### `Authenticator` interface (`interfaces.go`)

```go
var ErrRefreshNotSupported = errors.New("cliproxy auth: refresh not supported")

type LoginOptions struct {
    NoBrowser    bool
    ProjectID    string
    CallbackPort int
    Metadata     map[string]string   // напр. codex_login_mode=device
    Prompt       func(prompt string) (string, error)  // ручной ввод callback URL
}

type Authenticator interface {
    Provider() string
    Login(ctx context.Context, cfg *config.Config, opts *LoginOptions) (*coreauth.Auth, error)
    RefreshLead() *time.Duration    // опережение refresh; nil = без refresh
}
```

### Готовые authenticators (R9.A.1)

```go
// Codex (OpenAI) — callback-порт 1455, RefreshLead 5 дней, поддерживает device flow
type CodexAuthenticator struct { CallbackPort int }
func NewCodexAuthenticator() *CodexAuthenticator

// Claude (Anthropic) — callback-порт 54545, RefreshLead 4 часа
type ClaudeAuthenticator struct { CallbackPort int }
func NewClaudeAuthenticator() *ClaudeAuthenticator

// Antigravity (Google) — RefreshLead 5 минут
func NewAntigravityAuthenticator() Authenticator

// Kimi (Moonshot) — device-code flow, RefreshLead 5 минут
func NewKimiAuthenticator() Authenticator

// xAI (Grok) — device-code flow
func NewXAIAuthenticator() Authenticator

func FetchAntigravityProjectID(ctx context.Context, accessToken string, httpClient *http.Client) (string, error)
```

### Глобальный token store (`store_registry.go`)

```go
func RegisterTokenStore(store coreauth.Store)   // регистрация ДО Builder.Build()
func GetTokenStore() coreauth.Store             // lazy FileTokenStore если не зарегистрирован
```

### `FileTokenStore` (`filestore.go`)

```go
type FileTokenStore struct { /* unexported */ }

func NewFileTokenStore() *FileTokenStore
func (s *FileTokenStore) SetBaseDir(dir string)
func (s *FileTokenStore) Save(ctx context.Context, auth *cliproxyauth.Auth) (string, error)
func (s *FileTokenStore) List(ctx context.Context) ([]*cliproxyauth.Auth, error)
func (s *FileTokenStore) Delete(ctx context.Context, id string) error
```

### Plugin auth parsers

```go
type PluginAuthParser interface {
    ParseAuth(context.Context, pluginapi.AuthParseRequest) (*cliproxyauth.Auth, bool, error)
}
type PluginMultiAuthParser interface {
    ParseAuths(context.Context, pluginapi.AuthParseRequest) ([]*cliproxyauth.Auth, bool, error)
}
func RegisterPluginAuthParser(parser PluginAuthParser)
```

---

## `sdk/access` — проверка клиентских API-keys

### `Provider` (`registry.go`) — **контракт 4 бизнес-слоя** (ADR-9)

```go
type Provider interface {
    Identifier() string
    Authenticate(ctx context.Context, r *http.Request) (*Result, *AuthError)
}

func RegisterProvider(typ string, provider Provider)
func UnregisterProvider(typ string)
func SetExclusiveProvider(typ string)
func ClearExclusiveProvider()
func RegisteredProviders() []Provider
```

### `Manager` (`manager.go`)

```go
type Manager struct { /* unexported */ }

func NewManager() *Manager
func (m *Manager) SetProviders(providers []Provider)
func (m *Manager) Providers() []Provider
func (m *Manager) Authenticate(ctx context.Context, r *http.Request) (*Result, *AuthError)
// итерация по провайдерам до первого успеха; AuthErrorCodeNotHandled → skip
```

### `Result` и типы конфигурации (`types.go`)

```go
type Result struct {
    Provider  string
    Principal string            // ← user_id для аналитики (R3)
    Metadata  map[string]string
}

type AccessProvider struct {
    Name    string
    Type    string
    SDK     string
    APIKeys []string
    Config  map[string]any
}

const AccessProviderTypeConfigAPIKey = "config-api-key"
const DefaultAccessProviderName      = "config-inline"

func MakeInlineAPIKeyProvider(keys []string) *AccessProvider
```

### `AuthError` (`errors.go`)

```go
type AuthErrorCode string

const (
    AuthErrorCodeNoCredentials     AuthErrorCode = "no_credentials"      // 401
    AuthErrorCodeInvalidCredential AuthErrorCode = "invalid_credential"  // 401
    AuthErrorCodeNotHandled        AuthErrorCode = "not_handled"         // skip
    AuthErrorCodeInternal          AuthErrorCode = "internal_error"      // 500
)

type AuthError struct {
    Code       AuthErrorCode
    Message    string
    StatusCode int
    Cause      error
}

func (e *AuthError) Error() string
func (e *AuthError) Unwrap() error
func (e *AuthError) HTTPStatusCode() int

func NewNoCredentialsError() *AuthError
func NewInvalidCredentialError() *AuthError
func NewNotHandledError() *AuthError
func NewInternalAuthError(message string, cause error) *AuthError

func IsAuthErrorCode(authErr *AuthError, code AuthErrorCode) bool
```

---

## `sdk/api` — HTTP server options + handlers

### ServerOption (`options.go`)

```go
type ServerOption = internalapi.ServerOption

func WithMiddleware(mw ...gin.HandlerFunc) ServerOption
func WithEngineConfigurator(fn func(*gin.Engine)) ServerOption
func WithRouterConfigurator(fn func(*gin.Engine, *handlers.BaseAPIHandler, *config.Config)) ServerOption
func WithLocalManagementPassword(password string) ServerOption
func WithKeepAliveEndpoint(timeout time.Duration, onTimeout func()) ServerOption
func WithRequestLoggerFactory(factory func(*config.Config, string) logging.RequestLogger) ServerOption
```

⚠️ В `sdk/api` (main) **нет** `WithPostAuthHook`/`WithPostAuthPersistHook` —
`WithPostAuthHook` доступен только на `Builder` (см. [sdk/cliproxy](#sdkcliproxy—точка-входа)).

### Management (`management.go`) — OAuth-flow helpers (R9.A.1)

```go
type Handler = internalmanagement.Handler

type ManagementTokenRequester interface {
    RequestAnthropicToken(*gin.Context)
    RequestCodexToken(*gin.Context)
    RequestAntigravityToken(*gin.Context)
    RequestKimiToken(*gin.Context)
    GetAuthStatus(c *gin.Context)
    PostOAuthCallback(c *gin.Context)
}

func NewHandler(cfg *config.Config, configFilePath string, manager *coreauth.Manager) *Handler
func NewHandlerWithoutConfigFilePath(cfg *config.Config, manager *coreauth.Manager) *Handler
func NewManagementTokenRequester(cfg *config.Config, manager *coreauth.Manager) ManagementTokenRequester

// OAuth sessions
func RegisterOAuthSession(state, provider string)
func SetOAuthSessionError(state, message string)
func CompleteOAuthSession(state string)
func CompleteOAuthSessionsByProvider(provider string) int
func GetOAuthSession(state string) (provider, status string, ok bool)
func IsOAuthSessionPending(state, provider string) bool
func ValidateOAuthState(state string) error
func NormalizeOAuthProvider(provider string) (string, error)
func WriteOAuthCallbackFile(authDir, provider, state, code, errorMessage string) (string, error)
```

### `BaseAPIHandler` (`handlers/handlers.go`)

```go
type BaseAPIHandler struct {
    AuthManager     *coreauth.Manager
    Cfg             *config.SDKConfig
    PluginHost      PluginInterceptorHost
    ModelRouterHost PluginModelRouterHost
}

func NewBaseAPIHandlers(cfg *config.SDKConfig, authManager *coreauth.Manager) *BaseAPIHandler
func (h *BaseAPIHandler) UpdateClients(cfg *config.SDKConfig)
func (h *BaseAPIHandler) SetPluginHost(host PluginInterceptorHost)
func (h *BaseAPIHandler) SetModelRouterHost(host PluginModelRouterHost)
```

#### Методы исполнения

```go
func (h *BaseAPIHandler) ExecuteWithAuthManager(ctx context.Context, handlerType, modelName string, rawJSON []byte, alt string) ([]byte, http.Header, *interfaces.ErrorMessage)
func (h *BaseAPIHandler) ExecuteStreamWithAuthManager(ctx context.Context, handlerType, modelName string, rawJSON []byte, alt string) (<-chan []byte, http.Header, <-chan *interfaces.ErrorMessage)
func (h *BaseAPIHandler) ExecuteCountWithAuthManager(ctx context.Context, handlerType, modelName string, rawJSON []byte, alt string) ([]byte, http.Header, *interfaces.ErrorMessage)
func (h *BaseAPIHandler) ExecuteImageWithAuthManager(ctx context.Context, handlerType, modelName string, rawJSON []byte, alt string) ([]byte, http.Header, *interfaces.ErrorMessage)

// Programmatic model execution (для плагинов/callbacks)
func (h *BaseAPIHandler) ExecuteModel(ctx context.Context, req ModelExecutionRequest) (ModelExecutionResponse, *interfaces.ErrorMessage)
func (h *BaseAPIHandler) ExecuteModelStream(ctx context.Context, req ModelExecutionRequest) (ModelExecutionStream, *interfaces.ErrorMessage)
```

### Plugin-host интерфейсы (`handlers/handlers.go`)

```go
type PluginInterceptorHost interface {
    InterceptRequestBeforeAuth(context.Context, pluginapi.RequestInterceptRequest) pluginapi.RequestInterceptResponse
    InterceptRequestAfterAuth(context.Context, pluginapi.RequestInterceptRequest) pluginapi.RequestInterceptResponse
    InterceptResponse(context.Context, pluginapi.ResponseInterceptRequest) pluginapi.ResponseInterceptResponse
    InterceptStreamChunk(context.Context, pluginapi.StreamChunkInterceptRequest) pluginapi.StreamChunkInterceptResponse
}

type PluginModelRouterHost interface {
    RouteModel(context.Context, pluginapi.ModelRouteRequest) (pluginapi.ModelRouteResponse, bool)
}

type PluginExecutorHost interface {
    ExecutePluginExecutor(context.Context, string, coreexecutor.Request, coreexecutor.Options) (coreexecutor.Response, error)
    ExecutePluginExecutorStream(context.Context, string, coreexecutor.Request, coreexecutor.Options) (*coreexecutor.StreamResult, error)
    CountPluginExecutor(context.Context, string, coreexecutor.Request, coreexecutor.Options) (coreexecutor.Response, error)
}
```

⚠️ Все интерсепторы меняют только headers/body — **не дают доступа к HTTP-client/прокси**
(подтверждение ADR-10: per-request override прокси невозможен).

### Context-хелперы execution (`handlers/handlers.go`)

```go
func WithPinnedAuthID(ctx context.Context, authID string) context.Context
func WithSelectedAuthIDCallback(ctx context.Context, callback func(string)) context.Context
func WithExecutionSessionID(ctx context.Context, sessionID string) context.Context
func WithDisallowFreeAuth(ctx context.Context) context.Context
```

### Helpers (`handlers/`)

```go
// header_filter.go
func FilterUpstreamHeaders(src http.Header) http.Header
func WriteUpstreamHeaders(dst http.Header, src http.Header)

// request_body.go
func ReadRequestBody(c *gin.Context) ([]byte, error)   // поддерживает zstd

// stream_forwarder.go
type StreamForwardOptions struct {
    KeepAliveInterval  *time.Duration
    WriteChunk         func(chunk []byte)
    WriteTerminalError func(errMsg *interfaces.ErrorMessage)
    WriteDone          func()
    WriteKeepAlive     func()
}
func (h *BaseAPIHandler) ForwardStream(c *gin.Context, flusher http.Flusher, cancel func(error), data <-chan []byte, errs <-chan *interfaces.ErrorMessage, opts StreamForwardOptions)

// Утилиты интервалов
func StreamingKeepAliveInterval(cfg *config.SDKConfig) time.Duration
func NonStreamingKeepAliveInterval(cfg *config.SDKConfig) time.Duration
func StreamingBootstrapRetries(cfg *config.SDKConfig) int
func PassthroughHeadersEnabled(cfg *config.SDKConfig) bool
```

---

## `sdk/config` — конфигурация

Фасад над `internal/config` через type aliases.

### Функции

```go
func LoadConfig(configFile string) (*Config, error)
func LoadConfigOptional(configFile string, optional bool) (*Config, error)
func ParseConfigBytes(data []byte) (*Config, error)
func SaveConfigPreserveComments(configFile string, cfg *Config) error
func SaveConfigPreserveCommentsUpdateNestedScalar(configFile string, path []string, value string) error
```

### Type aliases

```go
type Config       = internalconfig.Config       // главный тип
type SDKConfig    = internalconfig.SDKConfig    // встраивается в Config
type StreamingConfig = internalconfig.StreamingConfig
type TLSConfig    = internalconfig.TLSConfig
```

### Ключевые поля `Config` (важные для бизнес-слоя)

| Поле | Тип | yaml | Назначение |
|------|-----|------|-----------|
| `Host` | string | `host` | сетевой интерфейс |
| `Port` | int | `port` | порт сервера |
| `AuthDir` | string | `auth-dir` | каталог токенов (default `~/.cli-proxy-api`) |
| `ProxyURL` | string | `proxy-url` | глобальный explicit proxy override SDK |
| `APIKeys` | []string | `api-keys` | inline клиентские API-keys |
| `RequestRetry` | int | `request-retry` | число ретраев |
| `MaxRetryCredentials` | int | `max-retry-credentials` | лимит кред на ретрай |
| `AuthAutoRefreshWorkers` | int | `auth-auto-refresh-workers` | размер пула auto-refresh |
| `Routing` | RoutingConfig | `routing` | Strategy (`round-robin`/`fill-first`), SessionAffinity |
| `Streaming` | StreamingConfig | `streaming` | KeepAliveSeconds, BootstrapRetries |
| `RemoteManagement` | RemoteManagement | `remote-management` | AllowRemote, SecretKey |
| `OAuthExcludedModels` | map[string][]string | `oauth-excluded-models` | per-provider исключения |
| `OAuthModelAlias` | map[string][]OAuthModelAlias | `oauth-model-alias` | алиасы моделей |

```go
type RoutingConfig struct {
    Strategy           string // "round-robin" (default) / "fill-first"
    SessionAffinity    bool
    SessionAffinityTTL string // default "1h"
}
```

---

## Сводка контрактов бизнес-слоя (ADR-9)

| Контракт | Интерфейс | Пакет | Где реализуем |
|----------|-----------|-------|---------------|
| Persist credentials | `coreauth.Store` | `sdk/cliproxy/auth` | `internal/store` |
| Выбор auth | `coreauth.Selector` | `sdk/cliproxy/auth` | `internal/auth/selector` |
| Аналитика | `usage.Plugin` + `coreauth.Hook` | `sdk/cliproxy/usage`, `sdk/cliproxy/auth` | `internal/usage` |
| Клиентский auth | `access.Provider` | `sdk/access` | `internal/access` |
| Auth-изменения | `cliproxy.WatcherFactory` | `sdk/cliproxy` | `internal/watcher` |
| Зеркало моделей | `cliproxy.ModelRegistryHook` | `sdk/cliproxy` | `internal/modelregistry` |
| Middleware/роуты | `api.With*` | `sdk/api` | `internal/httpapi` |

## Замечания

- **R10 проекта:** `Auth.ProxyURL` и `Config.ProxyURL` намеренно остаются
  пустыми. Пустой transport SDK использует `http.DefaultTransport` и system
  proxy через `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY`; это проверяется при
  compatibility gate обновления SDK.
- **`sdk/config`** — полностью type-aliases над `internal/config`; методы `Config`
  (`CloneForRuntime`, и др.) наследуются через alias.
- **`usage.NewManager(buffer)`** — аргумент `buffer` фактически не используется;
  дефолтный синглтон `defaultManager = NewManager(512)`.
- **Home-режим** (`Config.Home.Enabled`) кардинально меняет поведение `Service.Run`
  (пропускает file-watcher, включает baseline-executors). Поле `yaml:"-"` — runtime-only.
