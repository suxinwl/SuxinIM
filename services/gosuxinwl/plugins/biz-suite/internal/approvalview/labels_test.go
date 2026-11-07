package approvalview

import (
	"encoding/json"
	"testing"
)

func TestPreferredLabel(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values []LocalizedText
		want   string
	}{
		{"Chinese after English", []LocalizedText{{"Annual leave", "en"}, {" 年假 ", "zh_CN"}}, "年假"},
		{"simplified before traditional", []LocalizedText{{"調休假", "zh_TW"}, {"调休假", "zh_Hans"}}, "调休假"},
		{"Chinese language variant", []LocalizedText{{"Vacation", "en"}, {"休假", "zh-HK"}}, "休假"},
		{"preserve supplied other language", []LocalizedText{{"Special leave", "en"}}, "Special leave"},
		{"skip blank Chinese", []LocalizedText{{" ", "zh_CN"}, {"Sick leave", "en"}}, "Sick leave"},
		{"missing language", []LocalizedText{{"事假", ""}}, "事假"},
		{"missing labels", nil, ""},
		{"all blank", []LocalizedText{{"\t ", "zh_CN"}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := PreferredLabel(tc.values); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLeaveTypeLabelReadsOnlyTheRecordsSelectedOption(t *testing.T) {
	// Key 11 is deliberately paired with a different fixture label: this must
	// read the supplied text, never rely on a tenant-specific key dictionary.
	control := `{"control":"Vacation","value":{"vacation":{"selector":{"options":[{"key":"11","value":[{"text":"Annual leave","lang":"en"},{"text":"年假","lang":"zh_CN"}]}]}}}}`
	detail := `{"apply_data":{"contents":[` + control + `]}}`
	for _, raw := range []string{detail, `{"errcode":0,"info":` + detail + `}`} {
		for _, stored := range []string{"11", "年假", "", UnknownLeaveType} {
			if got := LeaveTypeLabel(raw, stored); got != "年假" {
				t.Fatalf("stored %q: got %q", stored, got)
			}
		}
		if got := LeaveTypeLabel(raw, "unrelated-key"); got != "" {
			t.Fatalf("unrelated saved option was guessed: %q", got)
		}
	}
	for _, raw := range []string{
		"", "not json", `{}`, `{"info":[]}`, `{"apply_data":{"contents":[]}}`,
		`{"apply_data":{"contents":[` + control + `,` + control + `]}}`,
		`{"apply_data":{"contents":[{"control":"Selector","value":{"vacation":{"selector":{"options":[{"key":"11","value":[{"text":"Wrong type","lang":"en"}]}]}}}}]}}`,
		`{"apply_data":{"contents":[{"control":"Vacation","value":{"vacation":{"selector":{"options":[{"key":"11"}]}}}}]}}`,
		`{"apply_data":{"contents":[{"control":"Vacation","value":{"vacation":{"selector":{"options":[{"key":"11","value":[{"text":"年假","lang":"zh_CN"}]},{"key":"12","value":[{"text":"事假","lang":"zh_CN"}]}]}}}}]}}`,
	} {
		if got := LeaveTypeLabel(raw, "11"); got != "" {
			t.Fatalf("missing or ambiguous label was guessed: %q", got)
		}
	}
}

func TestLeaveTypeLabelPreservesActualLabelText(t *testing.T) {
	for _, label := range []string{"事假", "年假", "病假", "调休假", "丧假", "Untranslated leave"} {
		value, _ := json.Marshal(label)
		raw := `{"apply_data":{"contents":[{"control":"Vacation","value":{"vacation":{"selector":{"options":[{"key":"tenant-specific-option","value":[{"text":` + string(value) + `,"lang":"zh_CN"}]}]}}}}]}}`
		if got := LeaveTypeLabel(raw, "tenant-specific-option"); got != label {
			t.Fatalf("got %q, want %q", got, label)
		}
	}
}
