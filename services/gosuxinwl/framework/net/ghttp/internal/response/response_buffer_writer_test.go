package response

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFlushKeepsCommittedEmptyResponseEmpty(t *testing.T) {
	for _, code := range []int{http.StatusOK, http.StatusNoContent, http.StatusPartialContent, http.StatusNotModified, http.StatusForbidden} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			writer := NewBufferWriter(recorder)
			writer.Status = code
			writer.RawWriter().WriteHeader(code)
			writer.Flush()
			writer.Flush()
			if recorder.Code != code || recorder.Body.Len() != 0 {
				t.Fatalf("committed response changed: status=%d body=%q", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestFlushRetainsNewDefaultErrorBody(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := NewBufferWriter(recorder)
	writer.WriteHeader(http.StatusNotFound)
	writer.Flush()
	writer.Flush()
	if recorder.Code != http.StatusNotFound || recorder.Body.String() != "Not Found" {
		t.Fatalf("default 404 changed: status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}
