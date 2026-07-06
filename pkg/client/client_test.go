package client

import (
	"testing"
)

func TestNewClient_EmptyURL(t *testing.T) {
	_, err := NewClient(nil, "", "", "")
	if err == nil {
		t.Fatal("expected error for empty URL, got nil")
	}
}
