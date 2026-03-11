package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dujiao-next/internal/config"
)

type telegramSendMessageResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
}

// TelegramSendOptions Telegram 发送参数。
type TelegramSendOptions struct {
	ChatID                string
	Message               string
	ParseMode             string
	DisableWebPagePreview bool
	AttachmentURL         string
	AttachmentDisplayName string
}

// TelegramNotifyService Telegram 通知发送服务
type TelegramNotifyService struct {
	settingService *SettingService
	defaultCfg     config.TelegramAuthConfig
	httpClient     *http.Client
}

// NewTelegramNotifyService 创建 Telegram 通知发送服务
func NewTelegramNotifyService(settingService *SettingService, defaultCfg config.TelegramAuthConfig) *TelegramNotifyService {
	return &TelegramNotifyService{
		settingService: settingService,
		defaultCfg:     defaultCfg,
		httpClient: &http.Client{
			Timeout: 6 * time.Second,
		},
	}
}

// SendMessage 发送 Telegram 消息
func (s *TelegramNotifyService) SendMessage(ctx context.Context, chatID, message string) error {
	token, err := s.resolveBotToken()
	if err != nil {
		return err
	}
	if token == "" {
		return ErrNotificationConfigInvalid
	}
	return s.SendWithBotToken(ctx, token, TelegramSendOptions{
		ChatID:                chatID,
		Message:               message,
		DisableWebPagePreview: true,
	})
}

// SendWithBotToken 使用显式 bot token 发送 Telegram 消息。
func (s *TelegramNotifyService) SendWithBotToken(ctx context.Context, botToken string, options TelegramSendOptions) error {
	chatID := strings.TrimSpace(options.ChatID)
	message := strings.TrimSpace(options.Message)
	botToken = strings.TrimSpace(botToken)
	if chatID == "" || message == "" || botToken == "" {
		return ErrNotificationSendFailed
	}

	if strings.TrimSpace(options.AttachmentURL) != "" {
		payload := map[string]interface{}{
			"chat_id":  chatID,
			"document": strings.TrimSpace(options.AttachmentURL),
			"caption":  message,
		}
		if parseMode := strings.TrimSpace(options.ParseMode); parseMode != "" {
			payload["parse_mode"] = parseMode
		}
		return s.sendJSONRequest(ctx, botToken, "sendDocument", payload)
	}

	payload := map[string]interface{}{
		"chat_id":                  chatID,
		"text":                     message,
		"disable_web_page_preview": options.DisableWebPagePreview,
	}
	if parseMode := strings.TrimSpace(options.ParseMode); parseMode != "" {
		payload["parse_mode"] = parseMode
	}
	return s.sendJSONRequest(ctx, botToken, "sendMessage", payload)
}

func (s *TelegramNotifyService) sendJSONRequest(ctx context.Context, botToken, method string, payload map[string]interface{}) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	requestURL := fmt.Sprintf("https://api.telegram.org/bot%s/%s", botToken, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotificationSendFailed, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotificationSendFailed, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: telegram status=%d body=%s", ErrNotificationSendFailed, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed telegramSendMessageResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("%w: parse telegram response failed", ErrNotificationSendFailed)
	}
	if !parsed.OK {
		return fmt.Errorf("%w: %s", ErrNotificationSendFailed, strings.TrimSpace(parsed.Description))
	}
	return nil
}

func (s *TelegramNotifyService) resolveBotToken() (string, error) {
	if s == nil {
		return "", nil
	}
	if s.settingService == nil {
		return strings.TrimSpace(s.defaultCfg.BotToken), nil
	}
	setting, err := s.settingService.GetTelegramAuthSetting(s.defaultCfg)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(setting.BotToken), nil
}
