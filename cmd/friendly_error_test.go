package cmd

import (
	"errors"
	"strings"
	"testing"
)

func TestFriendlyErrorRewritesHTMLJSONError(t *testing.T) {
	err := errors.New("invalid character '<' looking for beginning of value")
	got := friendlyError(err)
	if got == nil {
		t.Fatal("expected non-nil error")
	}
	msg := got.Error()
	if !strings.Contains(msg, "HTML") || !strings.Contains(strings.ToLower(msg), "authentication") {
		t.Fatalf("expected actionable HTML/auth message, got %q", msg)
	}
	// It should classify as an auth error, not internal.
	if code := classifyError(got); code != ExitAuthError {
		t.Fatalf("expected ExitAuthError(%d), got %d", ExitAuthError, code)
	}
}

func TestFriendlyErrorPassThrough(t *testing.T) {
	if friendlyError(nil) != nil {
		t.Fatal("nil should stay nil")
	}
	orig := errors.New("some other failure")
	if friendlyError(orig).Error() != "some other failure" {
		t.Fatal("unrelated errors should pass through unchanged")
	}
}
