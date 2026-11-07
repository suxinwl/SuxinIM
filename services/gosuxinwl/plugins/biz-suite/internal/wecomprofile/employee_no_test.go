package wecomprofile

import (
	"strings"
	"testing"
)

func TestEmployeeNoFromRaw(t *testing.T) {
	for _, test := range []struct {
		name, raw, want string
	}{
		{"real shape", `{"extattr":{"attrs":[{"name":"工号","type":0,"text":{"value":"7319"}}]}}`, "7319"},
		{"leading zero and whitespace", `{"extattr":{"attrs":[{"name":" 工号 ","type":0,"text":{"value":" 007319 "}}]}}`, "007319"},
		{"alphanumeric", `{"extattr":{"attrs":[{"name":"工号","type":0,"text":{"value":"HR-003"}}]}}`, "HR-003"},
		{"same duplicate", `{"extattr":{"attrs":[{"name":"工号","type":0,"text":{"value":"7319"}},{"name":"工号","type":0,"text":{"value":"7319"}}]}}`, "7319"},
		{"conflicting duplicate", `{"extattr":{"attrs":[{"name":"工号","type":0,"text":{"value":"7319"}},{"name":"工号","type":0,"text":{"value":"3240"}}]}}`, ""},
		{"duplicate wrong type", `{"extattr":{"attrs":[{"name":"工号","type":0,"text":{"value":"7319"}},{"name":"工号","type":1,"web":{"title":"7319"}}]}}`, ""},
		{"not userid or guessed field", `{"userid":"7319","employee_no":"7319","extattr":{"attrs":[{"name":"员工编号","type":0,"text":{"value":"7319"}}]}}`, ""},
		{"not a web field", `{"extattr":{"attrs":[{"name":"工号","type":1,"text":{"value":"7319"}}]}}`, ""},
		{"missing type", `{"extattr":{"attrs":[{"name":"工号","text":{"value":"7319"}}]}}`, ""},
		{"null type", `{"extattr":{"attrs":[{"name":"工号","type":null,"text":{"value":"7319"}}]}}`, ""},
		{"string type", `{"extattr":{"attrs":[{"name":"工号","type":"0","text":{"value":"7319"}}]}}`, ""},
		{"number value", `{"extattr":{"attrs":[{"name":"工号","type":0,"text":{"value":7319}}]}}`, ""},
		{"blank value", `{"extattr":{"attrs":[{"name":"工号","type":0,"text":{"value":"  "}}]}}`, ""},
		{"null value", `{"extattr":{"attrs":[{"name":"工号","type":0,"text":{"value":null}}]}}`, ""},
		{"control character", `{"extattr":{"attrs":[{"name":"工号","type":0,"text":{"value":"15\n99"}}]}}`, ""},
		{"too long", `{"extattr":{"attrs":[{"name":"工号","type":0,"text":{"value":"` + strings.Repeat("1", 65) + `"}}]}}`, ""},
		{"missing", `{}`, ""},
		{"null attributes", `{"extattr":null}`, ""},
		{"wrong attributes shape", `{"extattr":{"attrs":{}}}`, ""},
		{"invalid json", `{`, ""},
		{"wrong root shape", `[]`, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := EmployeeNoFromRaw(test.raw); got != test.want {
				t.Fatalf("employee number = %q, want %q", got, test.want)
			}
		})
	}
}
