package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/channels"
	"github.com/smallnest/goclaw/internal/logger"
	"github.com/smallnest/goclaw/session"
	"go.uber.org/zap"
)

// Handler WebSocket 消息处理器
type Handler struct {
	registry   *MethodRegistry
	bus        *bus.MessageBus
	sessionMgr *session.Manager
	channelMgr *channels.Manager
}

// NewHandler 创建处理器
func NewHandler(messageBus *bus.MessageBus, sessionMgr *session.Manager, channelMgr *channels.Manager) *Handler {
	h := &Handler{
		registry:   NewMethodRegistry(),
		bus:        messageBus,
		sessionMgr: sessionMgr,
		channelMgr: channelMgr,
	}

	// 注册系统方法
	h.registerSystemMethods()

	// 注册 Agent 方法
	h.registerAgentMethods()

	// 注册 Channel 方法
	h.registerChannelMethods()

	// 注册 Browser 方法
	h.registerBrowserMethods()

	return h
}

// HandleRequest 处理请求
func (h *Handler) HandleRequest(sessionID string, req *JSONRPCRequest) *JSONRPCResponse {
	result, err := h.registry.Call(req.Method, sessionID, req.Params)
	if err != nil {
		logger.Error("Method execution failed",
			zap.String("method", req.Method),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return NewErrorResponse(req.ID, ErrorInternalError, err.Error())
	}

	return NewSuccessResponse(req.ID, result)
}

// registerSystemMethods 注册系统方法
func (h *Handler) registerSystemMethods() {
	// config.get - 获取配置
	h.registry.Register("config.get", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		key, ok := params["key"].(string)
		if !ok {
			return nil, fmt.Errorf("key parameter is required")
		}
		// 这里应该从配置中读取
		// 简化实现：返回模拟数据
		return map[string]interface{}{
			"key":   key,
			"value": "config_value",
		}, nil
	})

	// config.set - 设置配置
	h.registry.Register("config.set", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		key, _ := params["key"].(string)
		value := params["value"]
		// 这里应该更新配置
		return map[string]interface{}{
			"key":   key,
			"value": value,
		}, nil
	})

	// health - 健康检查
	h.registry.Register("health", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{
			"status":    "ok",
			"timestamp": time.Now().Unix(),
			"version":   ProtocolVersion,
		}, nil
	})

	// logs - 获取日志
	h.registry.Register("logs.get", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		lines := 100
		if l, ok := params["lines"].(float64); ok {
			lines = int(l)
		}
		// 这里应该从日志中读取
		return map[string]interface{}{
			"lines": lines,
			"logs":  []string{}, // 实际应该返回日志
		}, nil
	})
}

// registerAgentMethods 注册 Agent 方法
func (h *Handler) registerAgentMethods() {
	// agent - 发送消息给 Agent
	h.registry.Register("agent", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		content, ok := params["content"].(string)
		if !ok {
			return nil, fmt.Errorf("content parameter is required")
		}

		// 构造入站消息
		msg := &bus.InboundMessage{
			Channel:   "websocket",
			SenderID:  sessionID,
			ChatID:    sessionID,
			Content:   content,
			Timestamp: time.Now(),
		}

		// 发布到消息总线
		if err := h.bus.PublishInbound(context.Background(), msg); err != nil {
			return nil, fmt.Errorf("failed to publish message: %w", err)
		}

		return map[string]interface{}{
			"status": "queued",
			"msg_id": msg.ID,
		}, nil
	})

	// agent.wait - 发送消息并等待响应
	h.registry.Register("agent.wait", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		content, ok := params["content"].(string)
		if !ok {
			return nil, fmt.Errorf("content parameter is required")
		}

		timeout := 30 * time.Second
		if t, ok := params["timeout"].(float64); ok {
			timeout = time.Duration(t) * time.Second
		}

		// 构造入站消息
		msg := &bus.InboundMessage{
			Channel:   "websocket",
			SenderID:  sessionID,
			ChatID:    sessionID,
			Content:   content,
			Timestamp: time.Now(),
		}

		// 发布到消息总线
		if err := h.bus.PublishInbound(context.Background(), msg); err != nil {
			return nil, fmt.Errorf("failed to publish message: %w", err)
		}

		// 等待响应（简化实现）
		// 实际应该通过监听出站消息来获取响应
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for response")
		default:
			// 返回初始响应
			return map[string]interface{}{
				"status":  "waiting",
				"msg_id":  msg.ID,
				"timeout": timeout.String(),
			}, nil
		}
	})

	// sessions.list - 列出所有会话
	h.registry.Register("sessions.list", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		sessions, err := h.sessionMgr.List()
		if err != nil {
			return nil, fmt.Errorf("failed to list sessions: %w", err)
		}

		result := make([]map[string]interface{}, 0, len(sessions))
		for _, key := range sessions {
			sess, err := h.sessionMgr.GetOrCreate(key)
			if err != nil {
				continue
			}
			result = append(result, map[string]interface{}{
				"key":           sess.Key,
				"message_count": len(sess.Messages),
				"created_at":    sess.CreatedAt,
				"updated_at":    sess.UpdatedAt,
			})
		}

		return result, nil
	})

	// sessions.get - 获取会话详情
	h.registry.Register("sessions.get", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		key, ok := params["key"].(string)
		if !ok {
			return nil, fmt.Errorf("key parameter is required")
		}

		sess, err := h.sessionMgr.GetOrCreate(key)
		if err != nil {
			return nil, fmt.Errorf("failed to get session: %w", err)
		}

		return map[string]interface{}{
			"key":        sess.Key,
			"messages":   sess.Messages,
			"created_at": sess.CreatedAt,
			"updated_at": sess.UpdatedAt,
			"metadata":   sess.Metadata,
		}, nil
	})

	// sessions.clear - 清空会话
	h.registry.Register("sessions.clear", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		key, ok := params["key"].(string)
		if !ok {
			return nil, fmt.Errorf("key parameter is required")
		}

		sess, err := h.sessionMgr.GetOrCreate(key)
		if err != nil {
			return nil, fmt.Errorf("failed to get session: %w", err)
		}

		sess.Clear()

		return map[string]interface{}{
			"status": "cleared",
			"key":    key,
		}, nil
	})
}

// registerChannelMethods 注册 Channel 方法
func (h *Handler) registerChannelMethods() {
	// channels.status - 获取通道状态
	h.registry.Register("channels.status", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		name, ok := params["channel"].(string)
		if !ok {
			return nil, fmt.Errorf("channel parameter is required")
		}

		status, err := h.channelMgr.Status(name)
		if err != nil {
			return nil, fmt.Errorf("failed to get channel status: %w", err)
		}

		return status, nil
	})

	// channels.list - 列出所有通道
	h.registry.Register("channels.list", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		channels := h.channelMgr.List()
		return map[string]interface{}{
			"channels": channels,
		}, nil
	})

	// send - 发送消息到通道
	h.registry.Register("send", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		channel, ok := params["channel"].(string)
		if !ok {
			return nil, fmt.Errorf("channel parameter is required")
		}

		chatID, ok := params["chat_id"].(string)
		if !ok {
			return nil, fmt.Errorf("chat_id parameter is required")
		}

		content, ok := params["content"].(string)
		if !ok {
			return nil, fmt.Errorf("content parameter is required")
		}

		msg := &bus.OutboundMessage{
			Channel:   channel,
			ChatID:    chatID,
			Content:   content,
			Timestamp: time.Now(),
		}

		if err := h.bus.PublishOutbound(context.Background(), msg); err != nil {
			return nil, fmt.Errorf("failed to send message: %w", err)
		}

		return map[string]interface{}{
			"status":  "sent",
			"msg_id":  msg.ID,
			"channel": channel,
			"chat_id": chatID,
		}, nil
	})

	// chat - 发送聊天消息（简化版）
	h.registry.Register("chat.send", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		// 与 send 相同，但可以添加更多聊天相关功能
		return h.registry.Call("send", sessionID, params)
	})
}

// registerBrowserMethods 注册 Browser 方法
func (h *Handler) registerBrowserMethods() {
	// browser.request - 浏览器请求
	h.registry.Register("browser.request", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		action, ok := params["action"].(string)
		if !ok {
			return nil, fmt.Errorf("action parameter is required")
		}

		// 这里应该调用浏览器工具
		// 简化实现：返回模拟响应
		return map[string]interface{}{
			"status": "executed",
			"action": action,
			"result": "browser action executed",
		}, nil
	})
}

// BroadcastNotification 广播通知
func (h *Handler) BroadcastNotification(method string, data interface{}) ([]byte, error) {
	notif := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params: map[string]interface{}{
			"data": data,
		},
	}

	return json.Marshal(notif)
}
