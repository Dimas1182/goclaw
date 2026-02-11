package channels

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// Manager 通道管理器
type Manager struct {
	channels map[string]BaseChannel
	bus      *bus.MessageBus
	mu       sync.RWMutex
}

// NewManager 创建通道管理器
func NewManager(bus *bus.MessageBus) *Manager {
	return &Manager{
		channels: make(map[string]BaseChannel),
		bus:      bus,
	}
}

// Register 注册通道
func (m *Manager) Register(channel BaseChannel) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	name := channel.Name()
	if _, ok := m.channels[name]; ok {
		return fmt.Errorf("channel %s already registered", name)
	}

	m.channels[name] = channel
	logger.Info("Channel registered", zap.String("channel", name))
	return nil
}

// Start 启动所有通道
func (m *Manager) Start(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for name, channel := range m.channels {
		logger.Info("Starting channel", zap.String("channel", name))
		if err := channel.Start(ctx); err != nil {
			logger.Error("Failed to start channel",
				zap.String("channel", name),
				zap.Error(err),
			)
			continue
		}
	}

	return nil
}

// Stop 停止所有通道
func (m *Manager) Stop() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var errors []error
	for name, channel := range m.channels {
		if err := channel.Stop(); err != nil {
			logger.Error("Failed to stop channel",
				zap.String("channel", name),
				zap.Error(err),
			)
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to stop some channels: %d errors", len(errors))
	}

	return nil
}

// Get 获取通道
func (m *Manager) Get(name string) (BaseChannel, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	channel, ok := m.channels[name]
	return channel, ok
}

// List 列出所有通道名称
func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.channels))
	for name := range m.channels {
		names = append(names, name)
	}
	return names
}

// Status 获取通道状态
func (m *Manager) Status(name string) (map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	channel, ok := m.channels[name]
	if !ok {
		return nil, fmt.Errorf("channel not found: %s", name)
	}

	// 简化的状态信息
	return map[string]interface{}{
		"name":    channel.Name(),
		"enabled": true,
	}, nil
}

// DispatchOutbound 分发出站消息
func (m *Manager) DispatchOutbound(ctx context.Context) error {
	logger.Info(">>> Starting outbound message dispatcher <<<")
	defer logger.Info(">>> Outbound dispatcher exited <<<")

	// 订阅出站消息
	subscription := m.bus.SubscribeOutbound()
	defer subscription.Unsubscribe()

	logger.Info("Subscribed to outbound messages",
		zap.String("subscription_id", subscription.ID))

	busChan := subscription.Channel

	// 定期心跳日志
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Outbound dispatcher stopped by context")
			return ctx.Err()
		case <-heartbeat.C:
			logger.Info("Outbound dispatcher heartbeat - waiting for messages...",
				zap.Int("outbound_queue_size", m.bus.OutboundCount()))
		case msg, ok := <-busChan:
			logger.Info("Outbound dispatcher: got message from channel",
				zap.Bool("ok", ok),
				zap.Bool("msg_nil", msg == nil))
			if !ok {
				logger.Warn("Outbound channel closed, exiting dispatcher")
				return nil
			}
			if msg == nil {
				logger.Warn("Received nil message, continuing")
				continue
			}

			logger.Info("Outbound message received",
				zap.String("channel", msg.Channel),
				zap.String("chat_id", msg.ChatID),
				zap.Int("content_length", len(msg.Content)))

			// 查找对应的通道
			channel, ok := m.Get(msg.Channel)
			if !ok {
				logger.Warn("Channel not found for outbound message",
					zap.String("channel", msg.Channel),
				)
				continue
			}

			// 发送消息
			if err := channel.Send(msg); err != nil {
				logger.Error("Failed to send message via channel",
					zap.String("channel", msg.Channel),
					zap.Error(err),
				)
			} else {
				logger.Info("Message sent successfully via channel",
					zap.String("channel", msg.Channel),
					zap.String("chat_id", msg.ChatID))
			}
		}
	}
}

// SetupFromConfig 从配置设置通道
func (m *Manager) SetupFromConfig(cfg *config.Config) error {
	// Telegram 通道
	if cfg.Channels.Telegram.Enabled && cfg.Channels.Telegram.Token != "" {
		tgCfg := TelegramConfig{
			BaseChannelConfig: BaseChannelConfig{
				Enabled:    cfg.Channels.Telegram.Enabled,
				AllowedIDs: cfg.Channels.Telegram.AllowedIDs,
			},
			Token: cfg.Channels.Telegram.Token,
		}

		channel, err := NewTelegramChannel(tgCfg, m.bus)
		if err != nil {
			logger.Error("Failed to create Telegram channel", zap.Error(err))
		} else {
			if err := m.Register(channel); err != nil {
				logger.Error("Failed to register Telegram channel", zap.Error(err))
			}
		}
	}

	// WhatsApp 通道
	if cfg.Channels.WhatsApp.Enabled && cfg.Channels.WhatsApp.BridgeURL != "" {
		waCfg := WhatsAppConfig{
			BaseChannelConfig: BaseChannelConfig{
				Enabled:    cfg.Channels.WhatsApp.Enabled,
				AllowedIDs: cfg.Channels.WhatsApp.AllowedIDs,
			},
			BridgeURL: cfg.Channels.WhatsApp.BridgeURL,
		}

		channel, err := NewWhatsAppChannel(waCfg, m.bus)
		if err != nil {
			logger.Error("Failed to create WhatsApp channel", zap.Error(err))
		} else {
			if err := m.Register(channel); err != nil {
				logger.Error("Failed to register WhatsApp channel", zap.Error(err))
			}
		}
	}

	// 飞书通道
	if cfg.Channels.Feishu.Enabled && cfg.Channels.Feishu.AppID != "" {
		channel, err := NewFeishuChannel(cfg.Channels.Feishu, m.bus)
		if err != nil {
			logger.Error("Failed to create Feishu channel", zap.Error(err))
		} else {
			if err := m.Register(channel); err != nil {
				logger.Error("Failed to register Feishu channel", zap.Error(err))
			}
		}
	}

	// QQ 通道 (使用官方 API)
	if cfg.Channels.QQ.Enabled {
		if cfg.Channels.QQ.AppID != "" {
			channel, err := NewQQChannel(cfg.Channels.QQ, m.bus)
			if err != nil {
				logger.Error("Failed to create QQ channel", zap.Error(err))
			} else {
				if err := m.Register(channel); err != nil {
					logger.Error("Failed to register QQ channel", zap.Error(err))
				}
			}
		} else {
			logger.Warn("QQ channel enabled but app_id not configured")
		}
	}

	// 企业微信通道
	if cfg.Channels.WeWork.Enabled && cfg.Channels.WeWork.CorpID != "" {
		channel, err := NewWeWorkChannel(cfg.Channels.WeWork, m.bus)
		if err != nil {
			logger.Error("Failed to create WeWork channel", zap.Error(err))
		} else {
			if err := m.Register(channel); err != nil {
				logger.Error("Failed to register WeWork channel", zap.Error(err))
			}
		}
	}

	return nil
}
