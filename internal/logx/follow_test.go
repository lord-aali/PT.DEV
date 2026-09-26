package logx

import "testing"

func TestParseCommonLog(t *testing.T) {
	line := `127.0.0.1 - - [25/Sep/2026:03:00:00 +0000] "GET /foo?x=1 HTTP/1.1" 200 1234`
	method, uri, status, ok := parseCommonLog(line)
	if !ok {
		t.Fatal("expected parse ok")
	}
	if method != "GET" || uri != "/foo?x=1" || status != "200" {
		t.Fatalf("got %s %s %s", method, uri, status)
	}
	if _, _, _, ok := parseCommonLog("not a log line"); ok {
		t.Fatal("expected parse fail")
	}
}
