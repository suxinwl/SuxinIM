// Package wecomprofile reads explicitly named profile attributes without
// depending on either the HRM or the synchronization packages.
package wecomprofile

import (
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"
)

// EmployeeNoFromRaw returns only an unambiguous text attribute named 工号.
// User IDs and other profile fields are never treated as employee numbers.
// Missing, malformed or conflicting values return empty so callers can keep
// an existing local employee number instead of erasing it.
func EmployeeNoFromRaw(raw string) string {
	var profile struct {
		ExtAttr struct {
			Attrs []json.RawMessage `json:"attrs"`
		} `json:"extattr"`
	}
	if json.Unmarshal([]byte(raw), &profile) != nil {
		return ""
	}
	value := ""
	for _, rawAttr := range profile.ExtAttr.Attrs {
		var label struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(rawAttr, &label) != nil {
			return ""
		}
		if strings.TrimSpace(label.Name) != "工号" {
			continue
		}
		var attr struct {
			Type *int `json:"type"`
			Text struct {
				Value *string `json:"value"`
			} `json:"text"`
		}
		if json.Unmarshal(rawAttr, &attr) != nil || attr.Type == nil || *attr.Type != 0 || attr.Text.Value == nil {
			return ""
		}
		candidate := strings.TrimSpace(*attr.Text.Value)
		if candidate == "" || utf8.RuneCountInString(candidate) > 64 || strings.ContainsFunc(candidate, unicode.IsControl) {
			return ""
		}
		if value != "" && value != candidate {
			return ""
		}
		value = candidate
	}
	return value
}
