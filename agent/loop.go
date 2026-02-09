package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/smallnest/dogclaw/goclaw/agent/tools"
	"github.com/smallnest/dogclaw/goclaw/bus"
	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"github.com/smallnest/dogclaw/goclaw/providers"
	"github.com/smallnest/dogclaw/goclaw/session"
	"go.uber.org/zap"
)

// Loop Agent 循环
type Loop struct {
	bus          *bus.MessageBus
	provider     providers.Provider
	sessionMgr   *session.Manager
	memory       *MemoryStore
	context      *ContextBuilder
	tools        *tools.Registry
	skillsLoader *SkillsLoader
	subagents    *SubagentManager
	workspace    string
	maxIteration int
	running      bool

	// 重试和错误处理
	errorClassifier *ErrorClassifier
	retryPolicy     RetryPolicy

	// 反思机制
	reflector *Reflector
}

// Config Loop 配置
type Config struct {
	Bus           *bus.MessageBus
	Provider      providers.Provider
	SessionMgr    *session.Manager
	Memory        *MemoryStore
	Context       *ContextBuilder
	Tools         *tools.Registry
	SkillsLoader  *SkillsLoader
	Subagents     *SubagentManager
	Workspace     string
	MaxIteration  int
	RetryConfig   *RetryConfig
	ReflectionCfg *ReflectionConfig
}

// NewLoop 创建 Agent 循环
func NewLoop(cfg *Config) (*Loop, error) {
	if cfg.MaxIteration <= 0 {
		cfg.MaxIteration = 15
	}

	// 创建错误分类器和重试策略
	errorClassifier := NewErrorClassifier()
	retryPolicy := NewDefaultRetryPolicy(cfg.RetryConfig)

	// 创建反思器
	reflector := NewReflector(cfg.ReflectionCfg, cfg.Provider, cfg.Workspace)

	return &Loop{
		bus:             cfg.Bus,
		provider:        cfg.Provider,
		sessionMgr:      cfg.SessionMgr,
		memory:          cfg.Memory,
		context:         cfg.Context,
		tools:           cfg.Tools,
		skillsLoader:    cfg.SkillsLoader,
		subagents:       cfg.Subagents,
		workspace:       cfg.Workspace,
		maxIteration:    cfg.MaxIteration,
		running:         false,
		errorClassifier: errorClassifier,
		retryPolicy:     retryPolicy,
		reflector:       reflector,
	}, nil
}

// Start 启动 Agent 循环
func (l *Loop) Start(ctx context.Context) error {
	logger.Info("Starting agent loop")
	l.running = true

	// 启动出站消息分发
	go l.dispatchOutbound(ctx)

	// 主循环
	for l.running {
		select {
		case <-ctx.Done():
			logger.Info("Agent loop stopped by context")
			return ctx.Err()
		default:
			// 消费入站消息
			msg, err := l.bus.ConsumeInbound(ctx)
			if err != nil {
				if err == context.DeadlineExceeded || err == context.Canceled {
					continue
				}
				logger.Error("Failed to consume inbound message", zap.Error(err))
				continue
			}

			// 处理消息
			go l.processMessage(ctx, msg)
		}
	}

	return nil
}

// Stop 停止 Agent 循环
func (l *Loop) Stop() error {
	logger.Info("Stopping agent loop")
	l.running = false
	return nil
}

// processMessage 处理消息
func (l *Loop) processMessage(ctx context.Context, msg *bus.InboundMessage) {
	logger.Info("Processing message",
		zap.String("channel", msg.Channel),
		zap.String("chat_id", msg.ChatID),
	)

	// 检查是否为系统消息
	if msg.IsSystemMessage() {
		l.processSystemMessage(ctx, msg)
		return
	}

	// 获取或创建会话
	sess, err := l.sessionMgr.GetOrCreate(msg.SessionKey())
	if err != nil {
		logger.Error("Failed to get session", zap.Error(err))
		return
	}

	// 添加用户消息到会话
	var media []session.Media
	for _, m := range msg.Media {
		media = append(media, session.Media{
			Type:     m.Type,
			URL:      m.URL,
			Base64:   m.Base64,
			MimeType: m.MimeType,
		})
	}

	sess.AddMessage(session.Message{
		Role:      "user",
		Content:   msg.Content,
		Media:     media,
		Timestamp: msg.Timestamp,
	})

	// 运行 Agent 迭代（带重试）
	response, err := l.runIterationWithRetry(ctx, sess, msg.Content)
	if err != nil {
		logger.Error("Agent iteration failed", zap.Error(err))

		// 检查是否需要上下文压缩
		if IsContextOverflowError(err.Error()) {
			logger.Info("Attempting context compression...")
			l.compressSession(sess)
			// 重试一次
			response, err = l.runIterationWithRetry(ctx, sess, msg.Content)
		}

		if err != nil {
			// 格式化错误消息
			userError := FormatErrorForUser(err.Error())
			_ = l.bus.PublishOutbound(ctx, &bus.OutboundMessage{
				Channel:   msg.Channel,
				ChatID:    msg.ChatID,
				Content:   fmt.Sprintf("抱歉，处理您的请求时出错：%s", userError),
				Timestamp: time.Now(),
			})
			return
		}
	}

	// 发送响应
	_ = l.bus.PublishOutbound(ctx, &bus.OutboundMessage{
		Channel:   msg.Channel,
		ChatID:    msg.ChatID,
		Content:   response,
		Timestamp: time.Now(),
	})

	// 添加助手响应到会话
	sess.AddMessage(session.Message{
		Role:      "assistant",
		Content:   response,
		Timestamp: time.Now(),
	})

	// 保存会话
	if err := l.sessionMgr.Save(sess); err != nil {
		logger.Error("Failed to save session", zap.Error(err))
	}
}

// processSystemMessage 处理系统消息
func (l *Loop) processSystemMessage(ctx context.Context, msg *bus.InboundMessage) {
	logger.Info("Processing system message",
		zap.String("task_id", msg.Metadata["task_id"].(string)),
	)

	// 从元数据中获取原始频道和聊天ID
	originChannel, _ := msg.Metadata["origin_channel"].(string)
	originChatID, _ := msg.Metadata["origin_chat_id"].(string)

	if originChannel == "" || originChatID == "" {
		logger.Warn("System message missing origin info")
		return
	}

	// 获取会话
	sess, err := l.sessionMgr.GetOrCreate(originChannel + ":" + originChatID)
	if err != nil {
		logger.Error("Failed to get session for system message", zap.Error(err))
		return
	}

	// 生成总结
	summary := l.generateSummary(ctx, msg)

	// 发送总结
	_ = l.bus.PublishOutbound(ctx, &bus.OutboundMessage{
		Channel:   originChannel,
		ChatID:    originChatID,
		Content:   summary,
		Timestamp: time.Now(),
	})

	// 添加到会话
	sess.AddMessage(session.Message{
		Role:      "assistant",
		Content:   summary,
		Timestamp: time.Now(),
	})

	// 保存会话
	if err := l.sessionMgr.Save(sess); err != nil {
		logger.Error("Failed to save session after system message", zap.Error(err))
	}
}

// runIterationWithRetry 使用重试机制运行 Agent 迭代
func (l *Loop) runIterationWithRetry(ctx context.Context, sess *session.Session, userRequest string) (string, error) {
	var result string
	var lastErr error

	attempt := 0
	maxAttempts := 3

	for attempt < maxAttempts {
		attempt++
		logger.Info("Agent iteration attempt", zap.Int("attempt", attempt))

		result, lastErr = l.runIteration(ctx, sess, userRequest)
		if lastErr == nil {
			return result, nil
		}

		// 检查是否应该重试
		shouldRetry, reason := l.retryPolicy.ShouldRetry(attempt, lastErr)
		if !shouldRetry {
			logger.Warn("No retry possible",
				zap.Int("attempt", attempt),
				zap.String("reason", string(reason)),
				zap.Error(lastErr))
			break
		}

		// 获取重试延迟
		delay := l.retryPolicy.GetDelay(attempt, reason)
		logger.Warn("Retrying after error",
			zap.Int("attempt", attempt),
			zap.String("reason", string(reason)),
			zap.Duration("delay", delay),
			zap.Error(lastErr))

		// 等待延迟
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(delay):
			continue
		}
	}

	return "", fmt.Errorf("failed after %d attempts: %w", attempt, lastErr)
}

// runIteration 运行 Agent 迭代（带反思机制）
func (l *Loop) runIteration(ctx context.Context, sess *session.Session, userRequest string) (string, error) {
	iteration := 0
	var lastResponse string
	var continuePrompt string

	// 获取已加载的技能名称（从会话元数据中）
	loadedSkills := l.getLoadedSkills(sess)

	for iteration < l.maxIteration {
		iteration++

		logger.Info("Agent iteration", zap.Int("iteration", iteration))

		// 获取可用技能
		var skills []*Skill
		if l.skillsLoader != nil {
			skills = l.skillsLoader.List()
		}

		// 构建上下文
		history := sess.GetHistory(50)
		messages := l.context.BuildMessages(history, continuePrompt, skills, loadedSkills)

		providerMessages := make([]providers.Message, len(messages))
		for i, msg := range messages {
			var tcs []providers.ToolCall
			for _, tc := range msg.ToolCalls {
				tcs = append(tcs, providers.ToolCall{
					ID:     tc.ID,
					Name:   tc.Name,
					Params: tc.Params,
				})
			}
			providerMessages[i] = providers.Message{
				Role:       msg.Role,
				Content:    msg.Content,
				Images:     msg.Images,
				ToolCallID: msg.ToolCallID,
				ToolCalls:  tcs,
			}
		}

		// 准备工具定义
		var toolDefs []providers.ToolDefinition
		if l.tools != nil {
			toolList := l.tools.List()
			logger.Info("Preparing tool definitions", zap.Int("tool_count", len(toolList)))
			for _, t := range toolList {
				toolDefs = append(toolDefs, providers.ToolDefinition{
					Name:        t.Name(),
					Description: t.Description(),
					Parameters:  t.Parameters(),
				})
				logger.Debug("Tool definition", zap.String("name", t.Name()), zap.String("description", t.Description()))
			}
		}

		// 调用 LLM
		response, err := l.provider.Chat(ctx, providerMessages, toolDefs)
		if err != nil {
			return "", fmt.Errorf("LLM call failed: %w", err)
		}

		logger.Info("LLM response received",
			zap.Int("tool_calls_count", len(response.ToolCalls)),
			zap.Int("content_length", len(response.Content)))

		// 检查是否有工具调用
		if len(response.ToolCalls) > 0 {
			// 重要：必须先把带有工具调用的助手消息存入历史记录
			var assistantToolCalls []session.ToolCall
			for _, tc := range response.ToolCalls {
				assistantToolCalls = append(assistantToolCalls, session.ToolCall{
					ID:     tc.ID,
					Name:   tc.Name,
					Params: tc.Params,
				})
				logger.Debug("Saving tool call to session",
					zap.String("tool_call_id", tc.ID),
					zap.String("tool_name", tc.Name),
					zap.Any("params", tc.Params))
			}
			assistantMsg := session.Message{
				Role:      "assistant",
				Content:   response.Content,
				Timestamp: time.Now(),
				ToolCalls: assistantToolCalls,
			}
			sess.AddMessage(assistantMsg)
			logger.Debug("Added assistant message with ToolCalls",
				zap.Int("tool_calls_count", len(assistantToolCalls)),
				zap.Int("session_messages_count", len(sess.Messages)))

			// 执行工具调用
			hasNewSkill := false
			for _, tc := range response.ToolCalls {
				result, err := l.executeToolWithRetry(ctx, tc.Name, tc.Params)
				if err != nil {
					// 工具执行错误不应该终止整个迭代
					// 将错误信息作为工具结果返回给 LLM
					result = fmt.Sprintf("Error: %v", err)
				}

				logger.Debug("Tool execution result",
					zap.String("tool_call_id", tc.ID),
					zap.String("tool_name", tc.Name),
					zap.Int("result_length", len(result)),
					zap.String("result_preview", func() string {
						if len(result) > 100 {
							return result[:100] + "..."
						}
						return result
					}()))

				// 检查是否是 use_skill 工具
				if tc.Name == "use_skill" {
					hasNewSkill = true
					// 提取技能名称
					if skillName, ok := tc.Params["skill_name"].(string); ok {
						loadedSkills = append(loadedSkills, skillName)
						l.setLoadedSkills(sess, loadedSkills)
					}
				}

				// 添加工具结果到会话
				toolMsg := session.Message{
					Role:       "tool",
					Content:    result,
					Timestamp:  time.Now(),
					ToolCallID: tc.ID,
					Metadata: map[string]interface{}{
						"tool_name": tc.Name,
					},
				}
				sess.AddMessage(toolMsg)
				logger.Debug("Added tool result message",
					zap.String("tool_call_id", tc.ID),
					zap.Int("session_messages_count", len(sess.Messages)))
			}

			// 如果加载了新技能，继续迭代让 LLM 获取完整内容
			if hasNewSkill {
				continue
			}

			// 继续下一次迭代
			continue
		}

		// 没有工具调用，检查任务是否完成
		if l.reflector != nil && l.reflector.config.Enabled {
			// 获取当前对话历史进行反思
			reflectionHistory := sess.GetHistory(30)

			reflection, reflectErr := l.reflector.Reflect(ctx, userRequest, reflectionHistory)
			if reflectErr != nil {
				logger.Warn("Reflection check failed, continuing without reflection", zap.Error(reflectErr))
			} else {
				// 根据反思结果决定是否继续
				if l.reflector.ShouldContinueIteration(reflection, iteration, l.maxIteration) {
					// 任务未完成，生成继续提示并继续迭代
					continuePrompt = l.reflector.GenerateContinuePrompt(reflection)
					logger.Info("Task not yet complete, continuing",
						zap.String("status", string(reflection.Status)),
						zap.Float64("confidence", reflection.Confidence),
						zap.Int("remaining_steps", len(reflection.RemainingSteps)))
					continue
				} else {
					// 任务完成或达到其他停止条件
					logger.Info("Task completion check",
						zap.String("status", string(reflection.Status)),
						zap.Float64("confidence", reflection.Confidence),
						zap.String("reasoning", reflection.Reasoning))
				}
			}
		}

		// 没有工具调用且任务完成，返回响应
		lastResponse = response.Content
		break
	}

	if iteration >= l.maxIteration {
		logger.Warn("Agent reached max iterations", zap.Int("max", l.maxIteration))
	}

	return lastResponse, nil
}

// executeToolWithRetry 使用重试机制执行工具
func (l *Loop) executeToolWithRetry(ctx context.Context, toolName string, params map[string]interface{}) (string, error) {
	var result string
	var err error

	attempt := 0
	maxAttempts := 2 // 工具执行最多重试 2 次

	for attempt < maxAttempts {
		attempt++

		result, err = l.tools.Execute(ctx, toolName, params)
		if err == nil {
			return result, nil
		}

		// 检查错误类型
		errMsg := strings.ToLower(err.Error())

		// 网络相关错误可以重试
		if strings.Contains(errMsg, "timeout") ||
			strings.Contains(errMsg, "network") ||
			strings.Contains(errMsg, "connection") ||
			strings.Contains(errMsg, "temporary") {

			logger.Warn("Tool execution failed, retrying",
				zap.String("tool", toolName),
				zap.Int("attempt", attempt),
				zap.Error(err))

			// 短暂延迟后重试
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
				continue
			}
		}

		// 其他错误不重试
		break
	}

	return "", fmt.Errorf("tool execution failed: %w", err)
}

// compressSession 压缩会话历史
func (l *Loop) compressSession(sess *session.Session) {
	originalCount := len(sess.Messages)

	// 保留最近的 10 轮对话
	if originalCount > 20 {
		// 保留系统消息
		var systemMessages []session.Message
		var recentMessages []session.Message
		turnCount := 0

		for i := len(sess.Messages) - 1; i >= 0; i-- {
			msg := sess.Messages[i]

			if msg.Role == "system" {
				systemMessages = append([]session.Message{msg}, systemMessages...)
				continue
			}

			if msg.Role == "user" {
				turnCount++
				if turnCount > 10 {
					break
				}
			}

			recentMessages = append([]session.Message{msg}, recentMessages...)
		}

		sess.Messages = append(systemMessages, recentMessages...)

		logger.Info("Session compressed",
			zap.Int("original_count", originalCount),
			zap.Int("compressed_count", len(sess.Messages)))
	}
}

// getLoadedSkills 从会话中获取已加载的技能名称
func (l *Loop) getLoadedSkills(sess *session.Session) []string {
	if sess.Metadata == nil {
		return []string{}
	}
	if v, ok := sess.Metadata["loaded_skills"].([]string); ok {
		return v
	}
	return []string{}
}

// setLoadedSkills 设置会话中已加载的技能名称
func (l *Loop) setLoadedSkills(sess *session.Session, skills []string) {
	if sess.Metadata == nil {
		sess.Metadata = make(map[string]interface{})
	}
	sess.Metadata["loaded_skills"] = skills
}

// generateSummary 生成子代理结果的总结
func (l *Loop) generateSummary(ctx context.Context, msg *bus.InboundMessage) string {
	// 简单实现：直接返回内容
	// 实际应该调用 LLM 生成更友好的总结
	return fmt.Sprintf("任务完成：%s", msg.Content)
}

// dispatchOutbound 分发出站消息
func (l *Loop) dispatchOutbound(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			msg, err := l.bus.ConsumeOutbound(ctx)
			if err != nil {
				if err == context.DeadlineExceeded || err == context.Canceled {
					continue
				}
				logger.Error("Failed to consume outbound message", zap.Error(err))
				continue
			}

			logger.Info("Dispatching outbound message",
				zap.String("channel", msg.Channel),
				zap.String("chat_id", msg.ChatID),
			)

			// 这里应该根据 channel 调用对应的通道发送器
			// 暂时只记录日志
		}
	}
}
