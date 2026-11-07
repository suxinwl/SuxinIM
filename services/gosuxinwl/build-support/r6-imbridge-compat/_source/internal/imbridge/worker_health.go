package imbridge

import (
	"context"
	"encoding/json"
	"time"
)

// workerHealth is deliberately limited to fixed stages, timestamps and safe
// error classifications. It never stores employees, payloads or credentials.
type workerHealth struct {
	Stage             string `json:"stage"`
	UpdatedAt         int64  `json:"updatedAt"`
	LastScanStarted   int64  `json:"lastScanStarted"`
	LastScanCompleted int64  `json:"lastScanCompleted"`
	LastScanResult    string `json:"lastScanResult"`
	LastDrainFinished int64  `json:"lastDrainFinished"`
	LastDrainResult   string `json:"lastDrainResult"`
}

func (h *workerHealth) record(parent context.Context, s *SQLStore, stage string, result error) error {
	now := time.Now().Unix()
	if stage == "drain_completed" && h.Stage == stage && now-h.UpdatedAt < 10 && h.LastDrainResult == Diagnostic(result) {
		return nil
	}
	h.Stage, h.UpdatedAt = stage, now
	switch stage {
	case "scanning":
		h.LastScanStarted = now
	case "scan_completed":
		h.LastScanCompleted, h.LastScanResult = now, Diagnostic(result)
	case "drain_completed":
		h.LastDrainFinished, h.LastDrainResult = now, Diagnostic(result)
	}
	raw, err := json.Marshal(h)
	if err != nil {
		return err
	}
	// A heartbeat must not make a slow database delay actual reconciliation.
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	if _, err = s.DB.ExecContext(ctx, `INSERT INTO biz_openim_state(k,v) SELECT 'worker_health',? WHERE NOT EXISTS (SELECT 1 FROM biz_openim_state WHERE k='worker_health')`, string(raw)); err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE biz_openim_state SET v=? WHERE k='worker_health'`, string(raw))
	return err
}
