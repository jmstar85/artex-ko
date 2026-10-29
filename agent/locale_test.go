package agent

import (
	"context"
	"testing"
)

func TestLocaleContextIsolation(t *testing.T) {
	base := context.Background()
	zh := WithLocale(base, "zh")
	ko := WithLocale(zh, "ko")
	for _, tc := range []struct {
		name string
		ctx context.Context
		want string
	}{
		{"base", base, ""},
		{"chinese", zh, "zh"},
		{"korean child", ko, "ko"},
		{"english", WithLocale(base, "en"), "en"},
		{"invalid clears inherited", WithLocale(zh, "zh-CN"), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := LocaleFromContext(tc.ctx); got != tc.want {
				t.Fatalf("locale = %q, want %q", got, tc.want)
			}
		})
	}
	for _, value := range []string{"", "ZH", " zh", "zh-CN", "ja"} {
		if got := NormalizeLocale(value); got != "" {
			t.Errorf("NormalizeLocale(%q) = %q, want empty", value, got)
		}
	}
}
