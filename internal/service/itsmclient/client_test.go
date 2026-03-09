package itsmclient

import "testing"

func TestMD5Token(t *testing.T) {
	token := md5Token(1729070026663, "58dc0cbc10134be7cdefb3511f7503c2")
	if token != "26dc1e0da0c58e4497ed7169529de75e" {
		t.Fatalf("unexpected token: %s", token)
	}
}
