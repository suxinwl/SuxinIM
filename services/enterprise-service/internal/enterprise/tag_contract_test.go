package enterprise

import (
	"context"
	"testing"
)

func TestTagSendRequiresOriginalCustomElemEnvelopeBeforePersistingOrSending(t *testing.T) {
	s, im, _, _ := fixture(t)
	ctx := context.Background()
	for _, content := range []string{
		`{"customType":903,"data":{"textElem":{"content":"missing outer CustomElem"}}}`,
		`{"data":""}`, `{"data":"not JSON"}`, `{"data":"null"}`, `{"data":"[]"}`, `{"data":"{}"}`,
		`{"data":{"customType":903}}`,
	} {
		w := post(s, "alice", "/enterprise/v1/office/tag/send", map[string]any{"userIDs": []string{"bob"}, "content": content})
		if w.Code != 400 {
			t.Fatalf("invalid CustomElem was accepted: HTTP %d %s", w.Code, w.Body.String())
		}
	}
	docs, err := s.Store.List(ctx, "tag_send", "alice")
	if err != nil || len(docs) != 0 || len(im.sent) != 0 {
		t.Fatal("invalid payload persisted a send log or reached IM")
	}
	// Preserve the original envelope and inner business subtype exactly; the
	// same validation also permits picture/video/file fields inside its data.
	content := string(mustJSON(map[string]any{"data": string(mustJSON(map[string]any{"customType": 903, "data": map[string]any{"textElem": map[string]string{"content": "original tag notice"}}})), "description": "tag notice", "extension": ""}))
	data := mustData(t, post(s, "alice", "/enterprise/v1/office/tag/send", map[string]any{"userIDs": []string{"bob"}, "content": content}))
	if data["sendLog"] == nil || im.sent["bob"] != 1 {
		t.Fatal("valid original CustomElem was rejected")
	}
	docs, err = s.Store.List(ctx, "tag_send", "alice")
	if err != nil || len(docs) != 1 {
		t.Fatal("valid delivery was not recorded")
	}
	log, _, err := load[TagSendLog](ctx, s.Store, docs[0].ID)
	if err != nil || log.Content != content {
		t.Fatal("validation changed the original custom message format")
	}
}
