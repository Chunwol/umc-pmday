package queue

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"umc-pmday/internal/db"
	"umc-pmday/internal/models"
)

type jobKind int

const (
	jobApply jobKind = iota
	jobCancel
	jobAdminRemove
	jobAdminSettings
	jobAdminReset
)

type job struct {
	kind          jobKind
	seq           uint64
	clickedAt     time.Time
	applyReq      models.ApplyRequest
	cancelReq     models.CancelRequest
	adminRemoveID int64
	settingsReq   struct {
		totalQuota int
		mode       string
		openAt     time.Time
		paused     bool
	}
	applyResp  chan models.ApplyResponse
	actionResp chan ActionResult
}

type ActionResult struct {
	OK      bool                 `json:"ok"`
	Message string               `json:"message"`
	State   models.StateResponse `json:"state"`
}

// Hub manages SSE subscribers and coalesces state broadcasts under heavy burst traffic.
type Hub struct {
	mu      sync.RWMutex
	clients map[chan []byte]struct{}
	notify  chan struct{}
	store   *db.Store
}

func NewHub(store *db.Store) *Hub {
	h := &Hub{
		clients: make(map[chan []byte]struct{}),
		notify:  make(chan struct{}, 1),
		store:   store,
	}
	go h.loop()
	return h
}

func (h *Hub) Subscribe() chan []byte {
	ch := make(chan []byte, 8)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) Unsubscribe(ch chan []byte) {
	h.mu.Lock()
	if _, ok := h.clients[ch]; ok {
		delete(h.clients, ch)
		close(ch)
	}
	h.mu.Unlock()
}

func (h *Hub) Trigger() {
	select {
	case h.notify <- struct{}{}:
	default:
	}
}

func (h *Hub) loop() {
	for range h.notify {
		// Coalesce rapid bursts over a tiny 40ms window
		time.Sleep(40 * time.Millisecond)
		select {
		case <-h.notify:
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		state, err := h.store.GetState(ctx)
		cancel()
		if err != nil {
			continue
		}
		payload, err := json.Marshal(state)
		if err != nil {
			continue
		}

		h.mu.RLock()
		for ch := range h.clients {
			select {
			case ch <- payload:
			default:
				// Drop slow client's old frame rather than blocking broadcast
			}
		}
		h.mu.RUnlock()
	}
}

// Coordinator assigns strict microsecond arrival sequence and serializes mutations.
type Coordinator struct {
	store  *db.Store
	hub    *Hub
	seq    atomic.Uint64
	ch     chan job
	stopCh chan struct{}
}

func NewCoordinator(store *db.Store, hub *Hub) (*Coordinator, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	maxSeq, err := store.MaxClickSeq(ctx)
	if err != nil {
		return nil, err
	}
	c := &Coordinator{
		store:  store,
		hub:    hub,
		ch:     make(chan job, 2048),
		stopCh: make(chan struct{}),
	}
	c.seq.Store(maxSeq)
	go c.run()
	return c, nil
}

func (c *Coordinator) run() {
	for {
		select {
		case j := <-c.ch:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			switch j.kind {
			case jobApply:
				resp := c.store.ApplyAtomic(ctx, j.seq, j.clickedAt, j.applyReq)
				j.applyResp <- resp
				c.hub.Trigger()
			case jobCancel:
				ok, msg := c.store.CancelByUser(ctx, j.seq, j.clickedAt, j.cancelReq)
				state, _ := c.store.GetState(ctx)
				j.actionResp <- ActionResult{OK: ok, Message: msg, State: state}
				c.hub.Trigger()
			case jobAdminRemove:
				ok, msg := c.store.AdminRemoveApplication(ctx, j.seq, j.clickedAt, j.adminRemoveID)
				state, _ := c.store.GetState(ctx)
				j.actionResp <- ActionResult{OK: ok, Message: msg, State: state}
				c.hub.Trigger()
			case jobAdminSettings:
				err := c.store.UpdateSettings(ctx, j.settingsReq.totalQuota, j.settingsReq.mode, j.settingsReq.openAt, j.settingsReq.paused)
				state, _ := c.store.GetState(ctx)
				if err != nil {
					j.actionResp <- ActionResult{OK: false, Message: "설정 저장 실패: " + err.Error(), State: state}
				} else {
					j.actionResp <- ActionResult{OK: true, Message: "설정이 반영되었습니다.", State: state}
					c.hub.Trigger()
				}
			case jobAdminReset:
				err := c.store.AdminResetAll(ctx)
				if err == nil {
					c.seq.Store(0)
				}
				state, _ := c.store.GetState(ctx)
				if err != nil {
					j.actionResp <- ActionResult{OK: false, Message: "초기화 실패: " + err.Error(), State: state}
				} else {
					j.actionResp <- ActionResult{OK: true, Message: "모든 신청 및 로그가 초기화되었습니다.", State: state}
					c.hub.Trigger()
				}
			}
			cancel()
		case <-c.stopCh:
			return
		}
	}
}

func (c *Coordinator) EnqueueApply(ctx context.Context, req models.ApplyRequest) models.ApplyResponse {
	seq := c.seq.Add(1)
	clickedAt := time.Now()
	respCh := make(chan models.ApplyResponse, 1)

	select {
	case c.ch <- job{
		kind:      jobApply,
		seq:       seq,
		clickedAt: clickedAt,
		applyReq:  req,
		applyResp: respCh,
	}:
	default:
		return models.ApplyResponse{
			OK:          false,
			Status:      "BUSY",
			Message:     "현재 요청이 폭주하고 있습니다. 잠시 후 다시 시도해 주세요.",
			ArrivalSeq:  seq,
			ClickedText: clickedAt.Format("15:04:05.000000"),
		}
	}

	select {
	case resp := <-respCh:
		return resp
	case <-ctx.Done():
		return models.ApplyResponse{
			OK:          false,
			Status:      "TIMEOUT",
			Message:     "처리 대기 시간이 초과되었습니다. 실시간 현황에서 배정 여부를 확인해 주세요.",
			ArrivalSeq:  seq,
			ClickedText: clickedAt.Format("15:04:05.000000"),
		}
	}
}

func (c *Coordinator) EnqueueCancel(ctx context.Context, req models.CancelRequest) ActionResult {
	seq := c.seq.Add(1)
	clickedAt := time.Now()
	respCh := make(chan ActionResult, 1)
	c.ch <- job{
		kind:       jobCancel,
		seq:        seq,
		clickedAt:  clickedAt,
		cancelReq:  req,
		actionResp: respCh,
	}
	select {
	case res := <-respCh:
		return res
	case <-ctx.Done():
		return ActionResult{OK: false, Message: "요청 시간이 초과되었습니다."}
	}
}

func (c *Coordinator) EnqueueAdminRemove(ctx context.Context, appID int64) ActionResult {
	seq := c.seq.Add(1)
	clickedAt := time.Now()
	respCh := make(chan ActionResult, 1)
	c.ch <- job{
		kind:          jobAdminRemove,
		seq:           seq,
		clickedAt:     clickedAt,
		adminRemoveID: appID,
		actionResp:    respCh,
	}
	select {
	case res := <-respCh:
		return res
	case <-ctx.Done():
		return ActionResult{OK: false, Message: "요청 시간이 초과되었습니다."}
	}
}

func (c *Coordinator) EnqueueAdminSettings(ctx context.Context, totalQuota int, mode string, openAt time.Time, paused bool) ActionResult {
	respCh := make(chan ActionResult, 1)
	j := job{
		kind:       jobAdminSettings,
		actionResp: respCh,
	}
	j.settingsReq.totalQuota = totalQuota
	j.settingsReq.mode = mode
	j.settingsReq.openAt = openAt
	j.settingsReq.paused = paused
	c.ch <- j
	select {
	case res := <-respCh:
		return res
	case <-ctx.Done():
		return ActionResult{OK: false, Message: "요청 시간이 초과되었습니다."}
	}
}

func (c *Coordinator) EnqueueAdminReset(ctx context.Context) ActionResult {
	respCh := make(chan ActionResult, 1)
	c.ch <- job{
		kind:       jobAdminReset,
		actionResp: respCh,
	}
	select {
	case res := <-respCh:
		return res
	case <-ctx.Done():
		return ActionResult{OK: false, Message: "요청 시간이 초과되었습니다."}
	}
}
