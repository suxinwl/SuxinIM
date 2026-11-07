package auth

import (
	"strconv"
	"testing"
	"time"

	"github.com/suxinwl/GoSuxin/utility/tools/cryptojs"
)

func TestDynamicTokenClockToleranceAndExpiry(t *testing.T) {
	now := time.Unix(1792928700, 0)
	for _, test := range []struct {
		name   string
		offset int64
		valid  bool
	}{
		{"current", 0, true}, {"remote browser three seconds ahead", 3, true},
		{"clock tolerance boundary", 5, true}, {"too far in future", 6, false},
		{"within expiry", -29, true}, {"expired boundary", -30, false},
		{"very old", -3600, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			wrapped, err := cryptojs.AesEncrypt("test-session#" + strconv.FormatInt(now.Unix()+test.offset, 10))
			if err != nil {
				t.Fatal(err)
			}
			token, ok := decodeDynamicTokenAt(wrapped, now)
			if ok != test.valid || (ok && token != "test-session") || (!ok && token != "") {
				t.Fatalf("unexpected acceptance: %v", ok)
			}
		})
	}
	for _, payload := range []string{"test-session#not-a-time", "test-session#9223372036854775807", "test-session#-9223372036854775808", "#1792928700", "test-session", "test-session#1792928700#extra"} {
		wrapped, err := cryptojs.AesEncrypt(payload)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := decodeDynamicTokenAt(wrapped, now); ok {
			t.Fatal("malformed timestamp accepted")
		}
	}
}
