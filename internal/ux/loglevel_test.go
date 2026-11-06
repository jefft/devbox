package ux

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestLogLevelFiltering(t *testing.T) {
	previous := logLevel.Load()
	t.Cleanup(func() { logLevel.Store(previous) })
	for _, level := range []string{"INFO", "warn", "ERROR"} {
		t.Run(level, func(t *testing.T) {
			if err := SetLogLevel(level); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			Finfo(&output, "plain-info\n")
			Finfof(&output, "formatted-%s\n", "info")
			Fwarning(&output, "plain-warning\n")
			Fwarningf(&output, "formatted-%s\n", "warning")
			FHidableWarning(context.Background(), &output, "visible-warning\n")
			FHidableWarning(HideMessage(context.Background(), "hidden-warning\n"), &output, "hidden-warning\n")
			Ferror(&output, "plain-error\n")
			Ferrorf(&output, "formatted-%s\n", "error")
			Fsuccess(&output, "result\n")
			for message, want := range map[string]bool{
				"plain-info": level == "INFO", "formatted-info": level == "INFO",
				"plain-warning": level != "ERROR", "formatted-warning": level != "ERROR",
				"visible-warning": level != "ERROR", "hidden-warning": false,
				"plain-error": true, "formatted-error": true, "result": true,
			} {
				if got := strings.Contains(output.String(), message); got != want {
					t.Errorf("%s visible = %t, want %t: %q", message, got, want, output.String())
				}
			}
		})
	}
	before := logLevel.Load()
	if err := SetLogLevel("invalid"); err == nil || logLevel.Load() != before {
		t.Fatal("invalid level must fail without changing the threshold")
	}
}
