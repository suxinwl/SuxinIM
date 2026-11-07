// Package approvalview extracts display labels from approval snapshots without
// database access or dependencies on the HRM and synchronization packages.
package approvalview

import (
	"encoding/json"
	"strings"
)

const UnknownLeaveType = "未提供假期名称"

type LocalizedText struct {
	Text string `json:"text"`
	Lang string `json:"lang"`
}

// PreferredLabel uses the supplied Chinese label first, then another supplied
// language. It never translates or infers a label from an option identifier.
func PreferredLabel(values []LocalizedText) string {
	best, bestRank := "", 3
	for _, value := range values {
		text := strings.TrimSpace(value.Text)
		if text == "" {
			continue
		}
		language := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value.Lang)), "_", "-")
		rank := 2
		if language == "zh-cn" || language == "zh-hans" {
			rank = 0
		} else if language == "zh" || strings.HasPrefix(language, "zh-") {
			rank = 1
		}
		if rank < bestRank {
			best, bestRank = text, rank
		}
	}
	return best
}

type vacationOption struct {
	Key   string          `json:"key"`
	Value []LocalizedText `json:"value"`
}

type approvalSnapshot struct {
	Info      json.RawMessage `json:"info"`
	ApplyData struct {
		Contents []struct {
			Control string `json:"control"`
			Value   struct {
				Vacation struct {
					Selector struct {
						Options []vacationOption `json:"options"`
					} `json:"selector"`
				} `json:"vacation"`
			} `json:"value"`
		} `json:"contents"`
	} `json:"apply_data"`
}

// LeaveTypeLabel resolves a stored option key (or an already normalized label)
// against this record's own Vacation selector. Missing or ambiguous snapshots
// return an empty string so callers can display an explicit unavailable label.
// Both a detail object and the official {"info": detail} response are supported.
func LeaveTypeLabel(rawJSON, storedType string) string {
	var snapshot approvalSnapshot
	if json.Unmarshal([]byte(rawJSON), &snapshot) != nil {
		return ""
	}
	if len(snapshot.Info) > 0 && strings.TrimSpace(string(snapshot.Info)) != "null" {
		var detail approvalSnapshot
		if json.Unmarshal(snapshot.Info, &detail) != nil {
			return ""
		}
		snapshot = detail
	}
	var selected *vacationOption
	vacations := 0
	for _, control := range snapshot.ApplyData.Contents {
		if control.Control != "Vacation" {
			continue
		}
		vacations++
		if len(control.Value.Vacation.Selector.Options) == 1 {
			option := control.Value.Vacation.Selector.Options[0]
			selected = &option
		}
	}
	if vacations != 1 || selected == nil {
		return ""
	}
	label := PreferredLabel(selected.Value)
	storedType = strings.TrimSpace(storedType)
	if label == "" || (storedType != "" && storedType != selected.Key && storedType != label && storedType != UnknownLeaveType) {
		return ""
	}
	return label
}
