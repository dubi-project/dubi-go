package dubi

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	. "github.com/dubi-project/dubi-go/base"
)

func captureStartupLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	previous := log.Writer()
	var output bytes.Buffer
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previous) })
	return &output
}

func TestStartupLogReportsEffectiveIdentityAndProfileSelection(t *testing.T) {
	output := captureStartupLog(t)
	rt := testRuntime(WithTenant("example"), WithRole("api"), WithProfile("web"), WithNode("worker-1"))
	t.Cleanup(Reset)
	rt.Register("web", Profile{Include: []string{"core:method:*"}})
	rt.Register("user.get", Method{Action: func(*Context) Map { return Map{} }})
	rt.Register("excluded", "not selected by profile")
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"dubi: ready", "modules=", "entries=1", "elapsed=", `tenant="example"`, `role="api"`, `profile="web"`, `node="worker-1"`} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("startup log missing %q: %s", expected, output)
		}
	}
	if err := rt.Start(context.Background()); err == nil {
		t.Fatal("second Start succeeded")
	}
	if count := strings.Count(output.String(), "dubi: ready"); count != 1 {
		t.Fatalf("readiness logged %d times: %s", count, output)
	}
}

func TestStartupFailureDoesNotReportReady(t *testing.T) {
	for _, phase := range []string{"open", "start"} {
		t.Run(phase, func(t *testing.T) {
			output := captureStartupLog(t)
			rt := testRuntime()
			t.Cleanup(Reset)
			failure := errors.New("test startup failure")
			if phase == "open" {
				testLifecycleModule.openError = failure
			} else {
				testLifecycleModule.startError = failure
			}
			if err := rt.Run(""); !errors.Is(err, failure) {
				t.Fatalf("startup failure = %v", err)
			}
			if strings.Contains(output.String(), "dubi: ready") {
				t.Fatalf("failed startup reported ready: %s", output)
			}
		})
	}
}
