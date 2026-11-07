package wecom

import (
 "context"
 "net/http"
 "net/http/httptest"
 "testing"
)

func TestRehearsalBlocksManualAndAutomaticUpstreamRequests(t *testing.T){
 t.Setenv("GOSUXIN_REHEARSAL","true");calls:=0;s:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){calls++;w.Write([]byte(`{"errcode":0,"access_token":"test","expires_in":7200}`))}));defer s.Close();c:=NewFromEnv();c.BaseURL=s.URL;c.CorpID="test";c.AppSecret="test";if _,e:=c.AccessToken(context.Background());e==nil{t.Fatal("rehearsal called an upstream API")};if calls!=0{t.Fatal("network side effect in rehearsal")};if e:=RegisterScheduler(context.Background(),c);e!=nil{t.Fatal(e)}
}
