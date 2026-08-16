package notifier

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"xray-checker/checker"
	"xray-checker/models"
)

func TestTelegramNotifierSendsOneGroupedReportToEveryChat(t *testing.T) {
	var mu sync.Mutex
	var requests []struct {
		ChatID          string `json:"chat_id"`
		Text            string `json:"text"`
		MessageThreadID *int64 `json:"message_thread_id"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottest-token/sendMessage" {
			t.Errorf("unexpected request path: %s", r.URL.Path)
		}
		var payload struct {
			ChatID          string `json:"chat_id"`
			Text            string `json:"text"`
			MessageThreadID *int64 `json:"message_thread_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		mu.Lock()
		requests = append(requests, payload)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewTelegramNotifier("test-token", []string{"100", " -200_42 "}, nil, true)
	notifier.apiURL = server.URL
	notifier.client = server.Client()
	notifier.NotifyStatusChanges([]checker.StatusChange{
		{Proxy: &models.ProxyConfig{Name: "down", Protocol: "vless", Server: "down.example", Port: 443, SubName: "main"}, Online: false},
		{Proxy: &models.ProxyConfig{Name: "up", Protocol: "trojan", Server: "up.example", Port: 443, SubName: "main"}, Online: true},
	})

	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		count := len(requests)
		mu.Unlock()
		if count == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected one grouped request to both chats, got %d", count)
		}
		time.Sleep(time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, request := range requests {
		if !strings.Contains(request.Text, "Unavailable:") || !strings.Contains(request.Text, "Restored:") {
			t.Errorf("expected grouped status message, got %q", request.Text)
		}
		if !strings.Contains(request.Text, "| main") {
			t.Errorf("expected subscription name in multi-subscription message, got %q", request.Text)
		}
		if request.ChatID == "-200" && (request.MessageThreadID == nil || *request.MessageThreadID != 42) {
			t.Errorf("expected topic 42 for chat -200, got %v", request.MessageThreadID)
		}
	}
}

func TestTelegramStatusMessagesHideSingleSubscriptionName(t *testing.T) {
	changes := []checker.StatusChange{{
		Proxy:  &models.ProxyConfig{Name: "node", Protocol: "vless", Server: "example.com", Port: 443, SubName: "only-sub"},
		Online: false,
	}}
	message := telegramStatusMessages(changes, false)[0]
	if strings.Contains(message, "only-sub") {
		t.Fatalf("single subscription name must be omitted, got %q", message)
	}
}

func TestTelegramNotifierFallsBackToDirectConnection(t *testing.T) {
	notifier := NewTelegramNotifier("test-token", []string{"100"}, func() []string { return nil }, false)
	client, err := notifier.notificationClient("")
	if err != nil {
		t.Fatalf("expected direct fallback, got error: %v", err)
	}
	if client.Transport != nil {
		t.Fatal("expected the default direct transport when no proxy is online")
	}
}

func TestTelegramNotifierFallsBackToDirectAfterProxyFailure(t *testing.T) {
	var received int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewTelegramNotifier("test-token", []string{"100"}, func() []string {
		return []string{"socks5://127.0.0.1:1"}
	}, false)
	notifier.apiURL = server.URL
	notifier.client = server.Client()
	if err := notifier.send(telegramChat{id: "100"}, "test"); err != nil {
		t.Fatalf("expected direct fallback, got error: %v", err)
	}
	if received != 1 {
		t.Fatalf("expected request through the second proxy, got %d", received)
	}
}

func TestTelegramNotifierDeliversBatchesInOrder(t *testing.T) {
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var mu sync.Mutex
	var messages []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		mu.Lock()
		messages = append(messages, payload.Text)
		first := len(messages) == 1
		mu.Unlock()
		if first {
			close(firstStarted)
			<-releaseFirst
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewTelegramNotifier("test-token", []string{"100"}, nil, false)
	notifier.apiURL = server.URL
	notifier.client = server.Client()
	notifier.NotifyStatusChanges([]checker.StatusChange{{Proxy: &models.ProxyConfig{Name: "down"}, Online: false}})
	<-firstStarted
	notifier.NotifyStatusChanges([]checker.StatusChange{{Proxy: &models.ProxyConfig{Name: "restored-1"}, Online: true}})
	notifier.NotifyStatusChanges([]checker.StatusChange{{Proxy: &models.ProxyConfig{Name: "restored-2"}, Online: true}})
	close(releaseFirst)

	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		count := len(messages)
		mu.Unlock()
		if count == 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected three ordered batches, got %d", count)
		}
		time.Sleep(time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(messages[0], "down") || !strings.Contains(messages[1], "restored-1") || !strings.Contains(messages[2], "restored-2") {
		t.Fatalf("unexpected delivered batches: %q", messages)
	}
}

func TestTelegramNotifierKeepsNewestBatchWhenQueueIsFull(t *testing.T) {
	notifier := &TelegramNotifier{messageQueue: make(chan []string, 1)}
	notifier.enqueue([]string{"older"})
	notifier.enqueue([]string{"newer"})
	if got := <-notifier.messageQueue; got[0] != "newer" {
		t.Fatalf("queued batch = %q, want newest batch", got)
	}
}

func TestTelegramStatusMessagesSplitLongReports(t *testing.T) {
	changes := make([]checker.StatusChange, 100)
	for i := range changes {
		changes[i] = checker.StatusChange{
			Proxy:  &models.ProxyConfig{Name: strings.Repeat("node", 20), Protocol: "vless", Server: "example.com", Port: 443, SubName: "main"},
			Online: false,
		}
	}
	messages := telegramStatusMessages(changes, false)
	if len(messages) < 2 {
		t.Fatalf("expected a long report to be split, got %d message", len(messages))
	}
	for _, message := range messages {
		if len(message) > telegramMessageMaxBytes {
			t.Fatalf("message exceeds %d bytes: %d", telegramMessageMaxBytes, len(message))
		}
	}

	longLineMessages := splitTelegramMessage([]string{"header", strings.Repeat("я", telegramMessageMaxBytes)})
	for _, message := range longLineMessages {
		if len(message) > telegramMessageMaxBytes {
			t.Fatalf("long line message exceeds %d bytes: %d", telegramMessageMaxBytes, len(message))
		}
	}
}
