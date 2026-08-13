package msb

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nkeys"

	"github.com/impire-io/soulstream-workloads/backend"
	"github.com/impire-io/soulstream-workloads/minter"
)

// stubMsb writes a fake msb executable. Every invocation appends its argv to
// the log (one token per line, invocations separated by "---"); the body runs
// only for `run` invocations (a real workload launch), so reap calls like
// `msb rm` return immediately. The body may write observations to $MARKS.
// This is the hermetic seam from research D6: unit tests drive the backend
// end to end with no real msb.
func stubMsb(t *testing.T, body string) (msbPath, logPath, marksPath string) {
	t.Helper()
	dir := t.TempDir()
	logPath = filepath.Join(dir, "log")
	marksPath = filepath.Join(dir, "marks")
	script := "#!/bin/sh\nLOG=" + logPath + "\nMARKS=" + marksPath + "\n" +
		"printf '%s\\n' \"$@\" >> \"$LOG\"\nprintf -- '---\\n' >> \"$LOG\"\n" +
		"[ \"$1\" = run ] || exit 0\n" + body + "\n"
	msbPath = filepath.Join(dir, "msb")
	if err := os.WriteFile(msbPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub msb: %v", err)
	}
	return msbPath, logPath, marksPath
}

// testCred mints a real scoped credential from a throwaway account — no
// server involved; Mint is pure signing.
func testCred(t *testing.T) minter.PersonaScopedCredential {
	t.Helper()
	acc, err := nkeys.CreateAccount()
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	pub, _ := acc.PublicKey()
	seed, _ := acc.Seed()
	m, err := minter.NewSigningKeyMinter(seed, pub, []string{"nats://127.0.0.1:4222"})
	if err != nil {
		t.Fatalf("minter: %v", err)
	}
	cred, err := m.Mint(minter.Scope{Persona: "researcher", Topic: "planning-ab12"}, time.Hour)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	return cred
}

func testSpec(t *testing.T, scratch string) backend.LaunchSpec {
	t.Helper()
	artifact := filepath.Join(t.TempDir(), "agent-echo")
	if err := os.WriteFile(artifact, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	return backend.LaunchSpec{
		Artifact:   artifact,
		Args:       []string{"--flag", "value"},
		Cred:       testCred(t),
		Realm:      "test-realm",
		Topic:      "planning-ab12",
		ScratchDir: scratch,
	}
}

// invocations splits the stub log into per-invocation argv slices.
func invocations(t *testing.T, logPath string) [][]string {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read stub log: %v", err)
	}
	var invs [][]string
	var cur []string
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if line == "---" {
			invs = append(invs, cur)
			cur = nil
			continue
		}
		cur = append(cur, line)
	}
	return invs
}

// TestStartArgvEnvAndReap drives one full launch through the stub and checks
// the whole seam contract: argv shape, the workload-env block (names from the
// shared contract, values adapted to the guest), a clean msb process env, the
// creds file present during the run, and Wait reaping both the sandbox record
// and the scratch dir.
func TestStartArgvEnvAndReap(t *testing.T) {
	scratch := filepath.Join(t.TempDir(), "item-42")
	// The stub asserts from inside the run: creds visible, no SOULSTREAM_* in
	// the process env msb itself receives.
	msbPath, logPath, marksPath := stubMsb(t,
		"test -f "+scratch+"/nats.creds && printf 'CREDS_PRESENT\\n' >> \"$MARKS\"\n"+
			"env | grep -q '^SOULSTREAM_' && printf 'ENV_LEAK\\n' >> \"$MARKS\"\nexit 0")

	b := &Backend{MsbPath: msbPath}
	spec := testSpec(t, scratch)

	h, err := b.Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Mount sources are handed to msb symlink-resolved (macOS tempdirs live
	// behind /var → /private/var); expectations must match the resolved form.
	wantScratch, err := filepath.EvalSymlinks(scratch)
	if err != nil {
		t.Fatalf("eval scratch: %v", err)
	}
	wantArtifact, err := filepath.EvalSymlinks(spec.Artifact)
	if err != nil {
		t.Fatalf("eval artifact: %v", err)
	}
	if st := h.Wait(); st.Code != 0 || st.Signalled() {
		t.Fatalf("Wait = %+v, want clean exit", st)
	}

	invs := invocations(t, logPath)
	if len(invs) != 2 {
		t.Fatalf("stub invoked %d times, want 2 (run + rm); log=%v", len(invs), invs)
	}

	run := invs[0]
	guestArtifact := "/artifact/" + filepath.Base(spec.Artifact)
	wantPrefix := []string{
		"run", "--no-tty", "--quiet",
		"--name", "soulstream-workloads-item-42",
		"-v", wantScratch + ":/scratch",
		"--copy-file", wantArtifact + ":" + guestArtifact,
		"-w", "/scratch",
		"--net", "host",
	}
	for i, want := range wantPrefix {
		if i >= len(run) || run[i] != want {
			t.Fatalf("run argv[%d] = %q, want %q; full=%v", i, run[i], want, run)
		}
	}
	wantEnv := []string{
		"SOULSTREAM_NATS_SERVERS=nats://host.microsandbox.internal:4222",
		"SOULSTREAM_NATS_CREDS=/scratch/nats.creds",
		"SOULSTREAM_REALM=test-realm",
		"SOULSTREAM_PERSONA=researcher",
		"SOULSTREAM_TOPIC=planning-ab12",
	}
	argvStr := strings.Join(run, "\n")
	for _, kv := range wantEnv {
		if !strings.Contains(argvStr, "-e\n"+kv) {
			t.Errorf("run argv missing env %q; full=%v", kv, run)
		}
	}
	wantTail := []string{DefaultImage, "--", guestArtifact, "--flag", "value"}
	tail := run[len(run)-len(wantTail):]
	for i, want := range wantTail {
		if tail[i] != want {
			t.Fatalf("run argv tail[%d] = %q, want %q; full=%v", i, tail[i], want, run)
		}
	}
	marks, _ := os.ReadFile(marksPath)
	if !strings.Contains(string(marks), "CREDS_PRESENT") {
		t.Error("creds file was not present in scratch during the run")
	}
	if strings.Contains(string(marks), "ENV_LEAK") {
		t.Error("msb process env leaked SOULSTREAM_* variables (must be clean)")
	}

	rm := invs[1]
	wantRm := []string{"rm", "--force", "soulstream-workloads-item-42"}
	if len(rm) < 3 || rm[0] != wantRm[0] || rm[1] != wantRm[1] || rm[2] != wantRm[2] {
		t.Fatalf("reap invocation = %v, want %v", rm, wantRm)
	}
	if _, err := os.Stat(scratch); !os.IsNotExist(err) {
		t.Fatalf("scratch dir survived Wait: stat err=%v", err)
	}
}

// TestExitCodePassthrough: the guest command's exit code, surfaced by the msb
// process, is the workload's exit status (seam exit-fidelity).
func TestExitCodePassthrough(t *testing.T) {
	msbPath, _, _ := stubMsb(t, "exit 7")
	b := &Backend{MsbPath: msbPath}

	h, err := b.Start(context.Background(), testSpec(t, filepath.Join(t.TempDir(), "item-7")))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	st := h.Wait()
	if st.Code != 7 || st.Signalled() {
		t.Fatalf("Wait = %+v, want Code 7", st)
	}
	if again := h.Wait(); again != st {
		t.Fatalf("second Wait = %+v, want same %+v", again, st)
	}
}

// TestStartFailureLeavesNothing: an unstartable msb yields an error and a
// cleaned scratch dir (behavioral contract item 1).
func TestStartFailureLeavesNothing(t *testing.T) {
	scratch := filepath.Join(t.TempDir(), "item-x")
	b := &Backend{MsbPath: filepath.Join(t.TempDir(), "no-such-msb")}

	if _, err := b.Start(context.Background(), testSpec(t, scratch)); err == nil {
		t.Fatal("Start succeeded with a nonexistent msb")
	}
	if _, err := os.Stat(scratch); !os.IsNotExist(err) {
		t.Fatalf("scratch dir survived failed Start: stat err=%v", err)
	}
}

// TestStopEscalation: a workload (msb process) that ignores SIGTERM is
// SIGKILLed after the grace, and Wait still reaps (US3 acceptance 2).
func TestStopEscalation(t *testing.T) {
	old := stopGrace
	stopGrace = 200 * time.Millisecond
	t.Cleanup(func() { stopGrace = old })

	scratch := filepath.Join(t.TempDir(), "item-stub")
	msbPath, logPath, marksPath := stubMsb(t,
		"trap '' TERM\nprintf 'READY\\n' >> \"$MARKS\"\nn=0\nwhile [ $n -lt 200 ]; do sleep 0.1; n=$((n+1)); done")
	b := &Backend{MsbPath: msbPath}

	h, err := b.Start(context.Background(), testSpec(t, scratch))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Wait until the stub has installed its TERM trap — a fixed sleep races
	// slow first spawns and would let SIGTERM win without escalation.
	ready := time.Now().Add(5 * time.Second)
	for {
		if data, _ := os.ReadFile(marksPath); strings.Contains(string(data), "READY") {
			break
		}
		if time.Now().After(ready) {
			t.Fatal("stub never became ready")
		}
		time.Sleep(10 * time.Millisecond)
	}

	start := time.Now()
	if err := h.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	st := h.Wait()
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("Stop+Wait took %v, escalation did not kick in", elapsed)
	}
	if !st.Signalled() {
		t.Fatalf("Wait = %+v, want a signalled status after SIGKILL", st)
	}
	if invs := invocations(t, logPath); len(invs) != 2 {
		t.Fatalf("stub invoked %d times, want 2 (run + rm)", len(invs))
	}
	if _, err := os.Stat(scratch); !os.IsNotExist(err) {
		t.Fatalf("scratch dir survived Wait after kill: stat err=%v", err)
	}
}

// TestSymlinkedPathsResolved: msb 0.6.7 cannot mount a source whose path
// traverses a symlink (measured — macOS tempdirs live behind /var →
// /private/var), so Start must hand msb fully resolved paths while the
// sandbox name keeps the declared work-item id.
func TestSymlinkedPathsResolved(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "real")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	link := filepath.Join(parent, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	scratch := filepath.Join(link, "item-9")

	msbPath, logPath, _ := stubMsb(t, "exit 0")
	b := &Backend{MsbPath: msbPath}
	spec := testSpec(t, scratch)

	h, err := b.Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	h.Wait()

	wantScratch, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("eval target: %v", err)
	}
	wantScratch = filepath.Join(wantScratch, "item-9")
	wantArtifact, err := filepath.EvalSymlinks(spec.Artifact)
	if err != nil {
		t.Fatalf("eval artifact: %v", err)
	}

	run := invocations(t, logPath)[0]
	argvStr := strings.Join(run, "\n")
	if !strings.Contains(argvStr, "-v\n"+wantScratch+":/scratch") {
		t.Errorf("scratch mount not resolved; argv=%v", run)
	}
	if !strings.Contains(argvStr, "--copy-file\n"+wantArtifact+":/artifact/") {
		t.Errorf("artifact path not resolved; argv=%v", run)
	}
	if !strings.Contains(argvStr, "--name\nsoulstream-workloads-item-9") {
		t.Errorf("sandbox name lost the work-item id; argv=%v", run)
	}
}

// TestRewriteServer moved to backend/natsurl (M2.1 extraction) — the env
// integration below still asserts msb passes rewritten servers through.

func TestSandboxName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/tmp/scratch/item-42", "soulstream-workloads-item-42"},
		{"/tmp/scratch/It.em:42", "soulstream-workloads-It-em-42"},
		{"/tmp/scratch/" + strings.Repeat("a", 150), "soulstream-workloads-" + strings.Repeat("a", 100)},
	}
	for _, c := range cases {
		if got := sandboxName(c.in); got != c.want {
			t.Errorf("sandboxName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestSatisfiesSeam is the compile-time statement that this package is a
// backend behind the seam.
var _ backend.Backend = (*Backend)(nil)

// TestZeroValueDefaults: the zero value must behave like New() so node-side
// config is always optional.
func TestZeroValueDefaults(t *testing.T) {
	var b Backend
	if b.image() != DefaultImage || b.msbPath() != DefaultMsbPath || b.hostAlias() != DefaultHostAlias {
		t.Fatalf("zero-value defaults = %q/%q/%q", b.image(), b.msbPath(), b.hostAlias())
	}
}
