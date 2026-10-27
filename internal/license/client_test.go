package license

import (
	"errors"
	"strings"
	"testing"
)

func TestAPIErrorUserMessagesAreCodeBased(t *testing.T) {
	cases := []struct {
		code string
		want string
	}{
		{"client_outdated", "过旧"},
		{"identity_mismatch", "邮箱"},
		{"max_machines_reached", "上限"},
		{"machine_already_bound", "另一个激活码"},
		{"email_not_configured", "邮件服务"},
		{"invalid_verification", "验证码"},
	}
	for _, test := range cases {
		err := &APIError{StatusCode: 400, Code: test.code, Message: "localized text"}
		if got := err.UserMessage(); !strings.Contains(got, test.want) {
			t.Fatalf("UserMessage(%q) = %q", test.code, got)
		}
	}
}

func TestDescribeErrorPreservesOnlyAPIErrors(t *testing.T) {
	apiErr := &APIError{StatusCode: 403, Code: "identity_mismatch", Message: "raw"}
	if got := DescribeError(apiErr); got == nil || !strings.Contains(got.Error(), "邮箱") {
		t.Fatalf("DescribeError = %v", got)
	}
	plain := errors.New("network down")
	if got := DescribeError(plain); got != plain {
		t.Fatalf("non-API error was wrapped: %v", got)
	}
}
