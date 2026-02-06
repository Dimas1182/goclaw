# Features to Port from OpenClaw Pi Agent

This document outlines features from OpenClaw's Pi agent integration that could be valuable to port to goclaw, organized by priority and category.

---

## High Priority

### 1. Session Management Enhancements

#### 1.1 Tree-Structured Sessions with Branching
**Status**: Not implemented in goclaw

**OpenClaw Reference**: `session-manager.ts` - Uses `id/parentId` tree structure

**Description**:
- Support for conversation branching and parallel exploration
- Each message can have a parent, creating a tree structure
- Enables rollback to any previous state

**Implementation Considerations**:
- Extend `session.Message` struct with `ParentID` field
- Modify `SessionManager.GetHistory()` to support tree traversal
- Add methods for branch creation and switching

```go
type Message struct {
    // ... existing fields ...
    ParentID    string    `json:"parent_id,omitempty"`
    BranchID    string    `json:"branch_id,omitempty"`
}
```

---

#### 1.2 Auto-Compaction
**Status**: Not implemented in goclaw

**OpenClaw Reference**: `pi-extensions/compaction-safeguard.ts`

**Description**:
- Automatically compress old conversation history when approaching token limits
- Preserves key information while reducing token count
- Customizable compaction strategy

**Implementation Considerations**:
- Detect context overflow errors from LLM
- Implement compaction algorithm (summary generation, message merging)
- Add compaction safeguard extension
- Configurable compaction triggers (token count, message count)

---

#### 1.3 Context Pruning (Cache-TTL)
**Status**: Not implemented in goclaw

**OpenClaw Reference**: `pi-extensions/context-pruning.ts`

**Description**:
- Track "time-to-live" for information in context
- Automatically prune stale or less relevant information
- Helps maintain focused conversations

**Implementation Considerations**:
- Add `TTL` field to messages
- Implement TTL-based filtering in `ContextBuilder`
- Configurable TTL defaults per message type

---

#### 1.4 DM vs Group Chat History Limits
**Status**: Partial (single limit only)

**OpenClaw Reference**: `history.ts`

**Description**:
- Different history limits for direct messages vs group chats
- Group chats: shorter history (more noise)
- Direct messages: longer history (more focused)

**Implementation Considerations**:
- Extend session key to track conversation type
- Add configuration: `dm_max_history`, `group_max_history`
- Modify `SessionManager.GetHistory()` to apply appropriate limit

---

### 2. Authentication & Failover

#### 2.1 Multi-Profile Auth Rotation
**Status**: Not implemented in goclaw

**OpenClaw Reference**: `auth-profiles.ts`, `model-auth.ts`

**Description**:
- Support multiple authentication profiles per provider
- Automatic rotation on failure
- Cooldown tracking for failed profiles
- Failover between providers

**Implementation Considerations**:
```go
type AuthProfile struct {
    ID          string
    Provider    string
    APIKey      string
    Priority    int
    CooldownUntil time.Time
    FailureCount int
}

type AuthProfileStore struct {
    profiles map[string][]*AuthProfile
}
```

---

#### 2.2 Error Classification System
**Status**: Not implemented in goclaw

**OpenClaw Reference**: `pi-embedded-helpers/errors.ts`

**Description**:
- Classify errors for intelligent retry/failover decisions
- Error types: auth, rate_limit, quota, timeout, context_overflow

**Implementation Considerations**:
```go
type FailoverReason string

const (
    FailoverAuth          FailoverReason = "auth"
    FailoverRateLimit     FailoverReason = "rate_limit"
    FailoverQuota         FailoverReason = "quota"
    FailoverTimeout       FailoverReason = "timeout"
    FailoverContextOverflow FailoverReason = "context_overflow"
)

func ClassifyFailoverReason(errorText string) FailoverReason {
    // Pattern matching logic
}
```

---

#### 2.3 Automatic Retry with Exponential Backoff
**Status**: Not implemented in goclaw

**OpenClaw Reference**: Built into Pi SDK `createAgentSession()`

**Description**:
- Retry failed requests automatically
- Exponential backoff between retries
- Configurable max retry count

**Implementation Considerations**:
- Add retry middleware in Provider implementations
- Per-reason retry strategies (auth: rotate profile, rate_limit: backoff)

---

### 3. Tool System Enhancements

#### 3.1 Hierarchical Policy Filtering
**Status**: Single-level (allow/deny only)

**OpenClaw Reference**: `tool-policy.ts`, `pi-tools.policy.ts`

**Description**:
- Five-level policy resolution: profile → provider → agent → group → sandbox
- More granular control over tool availability

**Implementation Considerations**:
```go
type PolicyLevel string

const (
    PolicyProfile  PolicyLevel = "profile"
    PolicyProvider PolicyLevel = "provider"
    PolicyAgent    PolicyLevel = "agent"
    PolicyGroup    PolicyLevel = "group"
    PolicySandbox  PolicyLevel = "sandbox"
)

type ToolPolicy struct {
    Allow []string
    Deny  []string
}

type PolicyResolver struct {
    policies map[PolicyLevel]map[string]*ToolPolicy
}

func (r *PolicyResolver) Resolve(toolName string) bool {
    // Check from most specific (sandbox) to least specific (profile)
}
```

---

#### 3.2 Owner-Only Tools
**Status**: Not implemented in goclaw

**OpenClaw Reference**: `pi-tools.ts` - owner-only concept

**Description**:
- Mark certain tools as owner-only (privileged operations)
- Only allowed users can execute these tools
- Configurable owner list per session

**Implementation Considerations**:
```go
type Tool struct {
    // ... existing fields ...
    OwnerOnly bool `json:"owner_only"`
}

type ToolPolicy struct {
    // ... existing fields ...
    OwnerIDs []string `json:"owner_ids"`
}
```

---

#### 3.3 AbortSignal for Tool Cancellation
**Status**: Not implemented in goclaw

**OpenClaw Reference**: `pi-tools.ts` - AbortSignal wrapping

**Description**:
- Allow long-running tools to be cancelled
- Propagate cancellation through tool execution

**Implementation Considerations**:
```go
type Tool interface {
    Name() string
    Description() string
    Parameters() map[string]interface{}
    Execute(ctx context.Context, params map[string]interface{}) (string, error)
}

// Tool implementations should check ctx.Done() for cancellation
```

---

#### 3.4 Message Tool Deduplication
**Status**: Not implemented in goclaw

**OpenClaw Reference**: `pi-embedded-messaging.ts`

**Description**:
- Track sent messages to prevent duplicates
- When agent uses message tool, suppress direct response to avoid double-send

**Implementation Considerations**:
```go
type MessagingTracker struct {
    sentMessages map[string]time.Time
    mu           sync.RWMutex
}

func (t *MessagingTracker) ShouldSend(chatID, content string) bool {
    // Check if similar message was just sent via tool
}
```

---

## Medium Priority

### 4. Streaming Response Improvements

#### 4.1 Block Chunking
**Status**: Not implemented in goclaw

**OpenClaw Reference**: `pi-embedded-block-chunker.ts`

**Description**:
- Intelligent paragraph-based chunking for streaming responses
- Prefer breaking at sentence/paragraph boundaries
- Better UX for long responses

**Implementation Considerations**:
- Parse markdown for natural break points
- Configure chunk size (character count, paragraph count)

---

#### 4.2 Thinking Tag Handling
**Status**: Not implemented in goclaw

**OpenClaw Reference**: `pi-embedded-subscribe.ts`

**Description**:
- Strip `<thinking>...</thinking>` tags from responses
- Support thinking modes: off, on, stream
- Separate thinking from final output

**Implementation Considerations**:
```go
type ThinkingMode string

const (
    ThinkingOff   ThinkingMode = "off"
    ThinkingOn    ThinkingMode = "on"
    ThinkingStream ThinkingMode = "stream"
)

func StripThinkingTags(content string) (thinking, content string)
```

---

#### 4.3 Final Block Handling
**Status**: Not implemented in goclaw

**OpenClaw Reference**: `pi-embedded-subscribe.handlers.ts`

**Description**:
- Strip `<final>...</final>` blocks from responses
- Used for internal control directives

**Implementation Considerations**:
- Parse and remove `<final>` blocks
- Log final block contents for debugging

---

### 5. System Prompt Enhancements

#### 5.1 Sandbox Information
**Status**: Not implemented in goclaw

**OpenClaw Reference**: `sandbox-info.ts`

**Description**:
- Inject Docker sandbox information into system prompt
- Container ID, limits, available resources
- Helps agent understand execution environment

**Implementation Considerations**:
```go
type SandboxInfo struct {
    ContainerID   string
    IsSandboxed   bool
    Limits        ResourceLimits
    Mounts        []Mount
}

type ResourceLimits struct {
    CPU    string
    Memory string
}
```

---

#### 5.2 Reply Tags
**Status**: Not implemented in goclaw

**OpenClaw Reference**: `system-prompt.ts`

**Description**:
- Special syntax for reply behavior:
  - `<reply_to:message_id>` - Reply to specific message
  - `<media:image_url>` - Include media in response
  - `<voice:text>` - Text-to-speech output

**Implementation Considerations**:
- Parse reply tags in agent responses
- Implement tag handling in OutboundMessage
- Support custom tag extensions

---

#### 5.3 CLI Reference
**Status**: Not implemented in goclaw

**OpenClaw Reference**: `system-prompt.ts`

**Description**:
- Include CLI command reference in system prompt
- Help agent understand available slash commands

**Implementation Considerations**:
- Scan for available CLI commands
- Format as markdown reference
- Inject into system prompt

---

#### 5.4 Runtime Metadata
**Status**: Not implemented in goclaw

**OpenClaw Reference**: `system-prompt.ts`

**Description**:
- Include runtime information in system prompt:
  - Agent version
  - Session ID
  - Model being used
  - Current time, timezone

**Implementation Considerations**:
- Build metadata object
- Format as structured markdown section
- Update dynamically per session

---

### 6. Provider Support

#### 6.1 Google Gemini Provider
**Status**: Not implemented in goclaw

**OpenClaw Reference**: `pi-embedded-runner/google.ts`

**Description**:
- Full support for Google Gemini models
- Handle Gemini-specific quirks (turn ordering)
- Streaming support

**Implementation Considerations**:
- Implement `providers/gemini.go`
- Use Gemini API directly or via langchaingo
- Handle tool calling format differences

---

#### 6.2 Provider-Specific Quirks Handling
**Status**: Minimal (basic only)

**OpenClaw Reference**: `pi-embedded-runner/google.ts`, `extra-params.ts`

**Description**:
- Provider-specific workarounds and optimizations
- Custom stream parameters per provider

**Implementation Considerations**:
```go
type ProviderQuirks struct {
    TurnOrderFix    bool    // For Gemini
    CustomParams    map[string]interface{}
}

type ChatOption func(*ChatOptions)

func WithProviderQuirks(quirks ProviderQuirks) ChatOption
```

---

## Low Priority

### 7. Additional Tools

#### 7.1 Canvas Tool
**Status**: Not implemented in goclaw

**OpenClaw Reference**: `tools/canvas-tool.ts`

**Description**:
- Generate visual diagrams and drawings
- Output as images for display

**Implementation Considerations**:
- Integrate with diagram-as-code tools (mermaid, plantuml)
- Or use drawing library for free-form canvas
- Convert to image format for channel sending

---

#### 7.2 Enhanced Image Generation
**Status**: Basic (via web tool)

**OpenClaw Reference**: `tools/image-tool.ts`

**Description**:
- Dedicated image generation tool
- Support multiple providers
- Better error handling and retry

**Implementation Considerations**:
- Implement `tools/image.go`
- Support DALL-E, Midjourney, Stable Diffusion APIs
- Image editing and manipulation

---

### 8. Extensions System

#### 8.1 Pi Extensions API
**Status**: Not implemented in goclaw

**OpenClaw Reference**: `pi-extensions/`

**Description**:
- Plugin system for session behavior customization
- Extensions for compaction, context pruning, etc.

**Implementation Considerations**:
```go
type Extension interface {
    Name() string
    BeforePrompt(ctx *PromptContext) error
    AfterResponse(ctx *ResponseContext) error
}

type ExtensionManager struct {
    extensions []Extension
}
```

---

### 9. Performance

#### 9.1 SessionManager Caching
**Status**: Not implemented in goclaw

**OpenClaw Reference**: `session-manager-cache.ts`

**Description**:
- Cache session data in memory
- Reduce disk I/O for frequently accessed sessions
- Configurable TTL for cache entries

**Implementation Considerations**:
```go
type SessionCache struct {
    entries map[string]*cacheEntry
    ttl     time.Duration
    mu      sync.RWMutex
}

type cacheEntry struct {
    session *session.Session
    expiry  time.Time
}
```

---

## Implementation Notes

### Dependency Considerations

| Feature | External Dependencies |
|---------|----------------------|
| Tree-Structured Sessions | None (data structure change) |
| Auto-Compaction | LLM API for summarization |
| Multi-Profile Auth | None |
| Error Classification | None (pattern matching) |
| Block Chunking | None (markdown parsing) |
| Thinking Tags | None (text parsing) |
| Canvas Tool | mermaid, plantuml, or drawing lib |
| Image Tool | DALL-E/Midjourney/Stable Diffusion APIs |

### Estimated Complexity

| Feature | Complexity |
|---------|------------|
| Tree-Structured Sessions | High |
| Auto-Compaction | Medium |
| Multi-Profile Auth | Medium |
| Error Classification | Low |
| Block Chunking | Low |
| Thinking Tags | Low |
| Hierarchical Policy | Medium |
| Owner-Only Tools | Low |
| Extensions System | High |

### Integration Points

| Feature | Files to Modify |
|---------|-----------------|
| Tree-Structured Sessions | `session/manager.go`, `session/message.go`, `agent/context.go` |
| Auto-Compaction | `agent/loop.go`, `agent/context.go` |
| Multi-Profile Auth | `providers/factory.go`, `config/schema.go` |
| Error Classification | `providers/*.go`, `agent/loop.go` |
| Block Chunking | `agent/loop.go`, `bus/outbound.go` |
| Thinking Tags | `agent/context.go`, `agent/loop.go` |
| Hierarchical Policy | `agent/tools/registry.go`, `config/schema.go` |
| Extensions System | `agent/loop.go` (new file) |

---

## Migration Path

### Phase 1: Foundation (Quick Wins)
1. Error Classification
2. Thinking Tag Handling
3. Final Block Handling
4. Block Chunking
5. Owner-Only Tools

### Phase 2: Reliability
1. Multi-Profile Auth Rotation
2. Automatic Retry
3. Message Tool Deduplication
4. SessionManager Caching

### Phase 3: Advanced Session Management
1. DM vs Group History Limits
2. Context Pruning
3. Tree-Structured Sessions
4. Auto-Compaction

### Phase 4: Enhanced Capabilities
1. Hierarchical Policy
2. System Prompt Enhancements (Sandbox, Reply Tags, etc.)
3. Google Gemini Provider
4. Extensions System

### Phase 5: Tools & Extras
1. Canvas Tool
2. Enhanced Image Generation
3. Provider-Specific Quirks

---

## References

- OpenClaw Pi Documentation: `/Users/chaoyuepan/ai/openclaw/docs/pi.md`
- OpenClaw Pi Dev Guide: `/Users/chaoyuepan/ai/openclaw/docs/pi-dev.md`
- Pi SDK: `@mariozechner/pi-coding-agent`
