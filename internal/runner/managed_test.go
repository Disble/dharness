package runner

// Slice B2 (mutate-staged-v1.9): managed long-lived processes with whole-tree
// ownership. RED first — these tests fail until managed.go and the platform
// managed files exist.
//
// The liveness instrument is a heartbeat file, not a PID query, so the same
// test proves tree death on both platforms: the grandchild appends while
// alive, the test proves the file grows before Close and freezes after it.
// A test that never observed a live grandchild cannot pass vacuously.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestManagedHelperProcess is re-executed as the child and grandchild. It is
// never run as a real test: without the marker env it returns immediately.
func TestManagedHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_RUNNER_HELPER") != "1" {
		return
	}
	role := ""
	beat := ""
	for _, a := range os.Args {
		if v, ok := strings.CutPrefix(a, "role="); ok {
			role = v
		}
		if v, ok := strings.CutPrefix(a, "beat="); ok {
			beat = v
		}
	}
	switch role {
	case "tree":
		// Parent of the heartbeat writer: spawn it, announce readiness once
		// it has beaten at least once, then sleep until killed.
		grandchild := exec.Command(os.Args[0], "-test.run=TestManagedHelperProcess", "--", "role=heartbeat", "beat="+beat)
		grandchild.Env = append(os.Environ(), "GO_WANT_RUNNER_HELPER=1")
		if err := grandchild.Start(); err != nil {
			fmt.Println("spawn failed")
			os.Exit(2)
		}
		for i := 0; i < 100; i++ {
			if sizeOf(beat) > 0 {
				fmt.Println("ready")
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		time.Sleep(60 * time.Second)
	case "heartbeat":
		f, err := os.OpenFile(beat, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			os.Exit(2)
		}
		defer func() { _ = f.Close() }()
		for {
			if _, err := f.WriteString("beat\n"); err != nil {
				os.Exit(0)
			}
			_ = f.Sync()
			time.Sleep(100 * time.Millisecond)
		}
	case "exit0":
		fmt.Println("quick line")
	default:
		fmt.Println("unknown role")
		os.Exit(2)
	}
	os.Exit(0)
}

func sizeOf(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return -1
	}
	return st.Size()
}

func startTreeHelper(t *testing.T, dir string) (*ManagedProcess, string) {
	t.Helper()
	// The marker travels through the test process environment because
	// Command carries no env override: StartManaged builds the child
	// environment from the host one, so the helper child (and through it
	// the grandchild) only acts when the test process carries the marker.
	t.Setenv("GO_WANT_RUNNER_HELPER", "1")
	beat := filepath.Join(dir, "heartbeat.log")
	mp, err := StartManaged(Command{Name: os.Args[0], Args: []string{"-test.run=TestManagedHelperProcess", "--", "role=tree", "beat=" + beat}, Dir: dir})
	if err != nil {
		t.Fatalf("StartManaged() = %v, want nil", err)
	}
	t.Cleanup(func() { _ = mp.Close() })
	deadline := time.Now().Add(5 * time.Second)
	for {
		if sizeOf(beat) > 0 {
			return mp, beat
		}
		if time.Now().After(deadline) {
			t.Fatal("grandchild never beat: the tree was never observably alive")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestManagedCloseRemovesTheWholeTree(t *testing.T) {
	dir := t.TempDir()
	mp, beat := startTreeHelper(t, dir)

	if err := mp.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}
	frozen := sizeOf(beat)
	time.Sleep(600 * time.Millisecond)
	if got := sizeOf(beat); got != frozen {
		t.Errorf("heartbeat grew from %d to %d after Close: a descendant survived", frozen, got)
	}
	if err := mp.Wait(); err == nil {
		t.Error("Wait() = nil after Close, want the kill report")
	}
}

func TestManagedCloseIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	mp, _ := startTreeHelper(t, dir)

	first := mp.Close()
	second := mp.Close()
	if (first == nil) != (second == nil) {
		t.Errorf("Close() = %v then %v, want the same outcome twice", first, second)
	}
}

func TestManagedQuickExitWaitsCleanly(t *testing.T) {
	t.Setenv("GO_WANT_RUNNER_HELPER", "1")
	mp, err := StartManaged(Command{Name: os.Args[0], Args: []string{"-test.run=TestManagedHelperProcess", "--", "role=exit0"}})
	if err != nil {
		t.Fatalf("StartManaged() = %v, want nil", err)
	}
	t.Cleanup(func() { _ = mp.Close() })
	if err := mp.Wait(); err != nil {
		t.Errorf("Wait() = %v, want nil for the clean exit", err)
	}
	if err := mp.Close(); err != nil {
		t.Errorf("Close() after a clean exit = %v, want nil: nothing is left to own", err)
	}
}
