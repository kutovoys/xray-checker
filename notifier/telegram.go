// Package notifier delivers status-change notifications to external services.
package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"xray-checker/checker"
	"xray-checker/logger"
)

const (
	telegramAPIURL          = "https://api.telegram.org"
	telegramMessageMaxBytes = 4000
	telegramQueueSize       = 16
)

// TelegramNotifier sends one grouped status report to every configured Telegram
// chat after a check cycle. Delivery happens asynchronously through a bounded
// FIFO queue; a full queue drops its oldest pending batch to keep checks moving.
type TelegramNotifier struct {
	token             string
	chats             []telegramChat
	client            *http.Client
	apiURL            string
	proxyURLsProvider func() []string
	showSubscription  bool
	messageQueue      chan []string
	queueMu           sync.Mutex
}

type telegramChat struct {
	id       string
	threadID *int64
}

// NewTelegramNotifier configures delivery through the currently healthy SOCKS5
// endpoints returned by proxyURLsProvider. When none are available, delivery
// falls back to a direct connection.
func NewTelegramNotifier(token string, chatIDs []string, proxyURLsProvider func() []string, showSubscription bool) *TelegramNotifier {
	chats := make([]telegramChat, 0, len(chatIDs))
	for _, chatID := range chatIDs {
		if chatID = strings.TrimSpace(chatID); chatID != "" {
			chats = append(chats, parseTelegramChat(chatID))
		}
	}

	notifier := &TelegramNotifier{
		token:             token,
		chats:             chats,
		client:            &http.Client{Timeout: 10 * time.Second},
		apiURL:            telegramAPIURL,
		proxyURLsProvider: proxyURLsProvider,
		showSubscription:  showSubscription,
		messageQueue:      make(chan []string, telegramQueueSize),
	}
	go notifier.deliverPending()
	return notifier
}

// parseTelegramChat accepts a normal chat ID and, for forum topics, the
// CHAT_ID_THREAD_ID form (for example, -1001234567890_42). The underscore
// suffix is only interpreted for numeric chat IDs, so @user_names stay valid.
func parseTelegramChat(value string) telegramChat {
	separator := strings.LastIndex(value, "_")
	if separator > 0 {
		chatID, threadValue := value[:separator], value[separator+1:]
		if _, err := strconv.ParseInt(chatID, 10, 64); err == nil {
			if threadID, err := strconv.ParseInt(threadValue, 10, 64); err == nil && threadID > 0 {
				return telegramChat{id: chatID, threadID: &threadID}
			}
		}
	}
	return telegramChat{id: value}
}

// NotifyStatusChanges sends all availability transitions from one check cycle in
// grouped messages. One report per chat avoids a burst of requests when many
// proxies change state at once.
func (n *TelegramNotifier) NotifyStatusChanges(changes []checker.StatusChange) {
	if n == nil || n.token == "" || len(n.chats) == 0 || len(changes) == 0 {
		return
	}

	messages := telegramStatusMessages(changes, n.showSubscription)
	n.enqueue(messages)
}

func (n *TelegramNotifier) enqueue(messages []string) {
	n.queueMu.Lock()
	defer n.queueMu.Unlock()

	select {
	case n.messageQueue <- messages:
		return
	default:
	}
	select {
	case <-n.messageQueue:
		logger.Warn("Telegram notification queue is full; dropping oldest pending batch")
	default:
	}
	select {
	case n.messageQueue <- messages:
	default:
		logger.Warn("Telegram notification queue is full; dropping newest batch")
	}
}

func (n *TelegramNotifier) deliverPending() {
	for messages := range n.messageQueue {
		for _, chat := range n.chats {
			for _, message := range messages {
				if err := n.send(chat, message); err != nil {
					logger.Error("Failed to send Telegram notification to chat %s: %v", chat.id, err)
					break
				}
			}
		}
	}
}

func telegramStatusMessages(changes []checker.StatusChange, showSubscription bool) []string {
	changes = append([]checker.StatusChange(nil), changes...)
	sort.Slice(changes, func(i, j int) bool {
		left, right := changes[i].Proxy, changes[j].Proxy
		if left.SubName != right.SubName {
			return left.SubName < right.SubName
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.Server < right.Server
	})

	lines := []string{}
	for _, online := range []bool{false, true} {
		header := "🔴 Unavailable:"
		if online {
			header = "🟢 Restored:"
		}
		sectionStarted := false
		for _, change := range changes {
			if change.Online != online {
				continue
			}
			if !sectionStarted {
				if len(lines) > 0 {
					lines = append(lines, "")
				}
				lines = append(lines, header)
				sectionStarted = true
			}
			proxy := change.Proxy
			line := "• " + proxy.Name
			if showSubscription && proxy.SubName != "" {
				line += " | " + proxy.SubName
			}
			lines = append(lines, line)
		}
	}

	return splitTelegramMessage(lines)
}

func splitTelegramMessage(lines []string) []string {
	messages := make([]string, 0, 1)
	current := ""
	for _, line := range lines {
		line = truncateUTF8(line, telegramMessageMaxBytes)
		candidate := line
		if current != "" {
			candidate = current + "\n" + line
		}
		if len(candidate) > telegramMessageMaxBytes && current != "" {
			messages = append(messages, current)
			current = line
			continue
		}
		current = candidate
	}
	if current != "" {
		messages = append(messages, current)
	}
	return messages
}

func truncateUTF8(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	for maxBytes > 0 && !utf8.RuneStart(value[maxBytes]) {
		maxBytes--
	}
	return value[:maxBytes]
}

func (n *TelegramNotifier) send(chat telegramChat, message string) error {
	proxyURLs := []string(nil)
	if n.proxyURLsProvider != nil {
		proxyURLs = n.proxyURLsProvider()
	}
	if len(proxyURLs) == 0 {
		return n.sendWithProxy(chat, message, "")
	}

	var lastErr error
	for _, proxyURL := range proxyURLs {
		if err := n.sendWithProxy(chat, message, proxyURL); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	if err := n.sendWithProxy(chat, message, ""); err == nil {
		return nil
	} else {
		return fmt.Errorf("all %d online Telegram proxies failed (%v), and direct delivery failed: %w", len(proxyURLs), lastErr, err)
	}
}

func (n *TelegramNotifier) sendWithProxy(chat telegramChat, message, proxyURL string) error {
	body, err := json.Marshal(struct {
		ChatID          string `json:"chat_id"`
		Text            string `json:"text"`
		MessageThreadID *int64 `json:"message_thread_id,omitempty"`
	}{ChatID: chat.id, Text: message, MessageThreadID: chat.threadID})
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}

	ctx := context.Background()
	cancel := func() {}
	if n.client.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, n.client.Timeout)
	}
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.apiURL+"/bot"+n.token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client, err := n.notificationClient(proxyURL)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %s", strings.ReplaceAll(err.Error(), n.token, "[REDACTED]"))
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Telegram API returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (n *TelegramNotifier) notificationClient(proxyURL string) (*http.Client, error) {
	client := *n.client
	if proxyURL == "" {
		return &client, nil
	}
	parsedProxyURL, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("parse Telegram proxy URL: %w", err)
	}
	if parsedProxyURL.Scheme != "socks5" && parsedProxyURL.Scheme != "socks5h" {
		return nil, fmt.Errorf("Telegram proxy must use socks5 or socks5h scheme")
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(parsedProxyURL)
	client.Transport = transport
	return &client, nil
}
