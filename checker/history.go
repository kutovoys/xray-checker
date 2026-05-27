package checker

import (
	"sync"
)

// CheckResult — одна запись истории проверки одного прокси.
type CheckResult struct {
	Online    bool  `json:"online"`
	LatencyMs int64 `json:"latencyMs"`
}

// ProxyHistory — кольцевой буфер на capacity записей.
type ProxyHistory struct {
	mu       sync.RWMutex
	capacity int
	buf      []CheckResult
	// total — сколько всего проверок было сделано (для checkCount в API).
	total int
}

func NewProxyHistory(capacity int) *ProxyHistory {
	if capacity < 1 {
		capacity = 1
	}
	return &ProxyHistory{
		capacity: capacity,
		buf:      make([]CheckResult, 0, capacity),
	}
}

// Add добавляет результат. Если буфер заполнен — выкидывает самый старый.
func (h *ProxyHistory) Add(r CheckResult) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.total++
	if len(h.buf) < h.capacity {
		h.buf = append(h.buf, r)
		return
	}
	// сдвиг влево, новый элемент в конец
	copy(h.buf, h.buf[1:])
	h.buf[len(h.buf)-1] = r
}

// SnapshotWithCounts возвращает копию истории, где последняя запись имеет checkCount = total,
// предпоследняя total-1 и т.д.
type HistoryItem struct {
	CheckCount int   `json:"checkCount"`
	Online     bool  `json:"online"`
	LatencyMs  int64 `json:"latencyMs"`
}

func (h *ProxyHistory) Snapshot() []HistoryItem {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]HistoryItem, len(h.buf))
	// самый старый элемент имеет номер (total - len(buf) + 1)
	startNum := h.total - len(h.buf) + 1
	for i, r := range h.buf {
		out[i] = HistoryItem{
			CheckCount: startNum + i,
			Online:     r.Online,
			LatencyMs:  r.LatencyMs,
		}
	}
	return out
}
