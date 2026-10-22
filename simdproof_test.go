package simdnorm

// These tests check that the SIMD code is actually used. They do not just
// assume it. There are three independent kinds of evidence:
//
//   1. Static:     disassemble this test binary and inspect the machine code.
//   2. Behavioral: SIMD keeps several partial sums in parallel lanes, which
//                  gives a numeric result a sequential loop cannot produce.
//   3. Dynamic:    CPU-profile Normalize and check that the samples land in
//                  the SIMD functions, so they are executed and not just present.
//
// Run them verbosely to see the evidence:  go test -v -run TestProof

import (
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"testing"
	"time"
)

type archExpectation struct {
	objdump     string         // "go" for `go tool objdump`, or "system" for LLVM/GNU objdump
	simdFuncs   []string       // symbols that must contain packed-SIMD math
	scalarFuncs []string       // pure-Go loops: must contain scalar math only
	packedMath  *regexp.Regexp // a floating-point instruction working on a whole vector
	scalarMath  *regexp.Regexp // a floating-point instruction working on one float
}

var archExpectations = map[string]archExpectation{
	// arm64 NEON: "V4.S4" = register V4 holding 4 float32 lanes; "F0" = one float.
	"arm64": {
		objdump:     "go",
		simdFuncs:   []string{"simdnorm.sumSquaresNEON", "simdnorm.scaleNEON"},
		scalarFuncs: []string{"simdnorm.sumSquaresScalar", "simdnorm.scaleScalar"},
		packedMath:  regexp.MustCompile(`\bVF\w+\s+V\d+\.S4`),
		scalarMath:  regexp.MustCompile(`\bF(MADD|MUL|ADD)S\s+F\d+`),
	},
	// amd64 AVX: "...ps" = packed single (many floats), "...ss" = scalar single;
	// %ymm registers are 256 bits wide (8 float32 lanes).
	//
	// Go's own x86 disassembler cannot decode AVX arithmetic (it prints FMA
	// instructions as "?" or garbage, and ymm registers as X), so use the
	// system objdump (LLVM on macOS, GNU binutils on Linux).
	"amd64": {
		objdump:     "system",
		simdFuncs:   []string{"simdnorm.sumSquaresAVX2", "simdnorm.scaleAVX2"},
		scalarFuncs: []string{"simdnorm.sumSquaresScalar", "simdnorm.scaleScalar"},
		packedMath:  regexp.MustCompile(`\bv(fmadd\w*|mul|add)ps\s.*%ymm\d+`),
		scalarMath:  regexp.MustCompile(`\b(v?(mul|add)ss|vfmadd\w*ss)\b`),
	},
}

func goTool(t *testing.T) string {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go command not in PATH; cannot run go tool")
	}
	return goBin
}

// disassemble returns the machine code of each requested symbol.
//
// The binary that `go test` runs is linked with -s -w (no symbol table), so a
// disassembler cannot find functions in it. Instead, build an unstripped copy
// of this same test binary with `go test -c`, using the same GOARCH,
// GOEXPERIMENT, etc. from the environment.
func disassemble(t *testing.T, tool string, symbols []string) map[string]string {
	t.Helper()
	goBin := goTool(t)
	exe := filepath.Join(t.TempDir(), "simdnorm.test")
	if out, err := exec.Command(goBin, "test", "-c", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("go test -c: %v\n%s", err, out)
	}

	var cmd *exec.Cmd
	var header *regexp.Regexp // matches the line that starts each function
	if tool == "go" {
		quoted := make([]string, len(symbols))
		for i, s := range symbols {
			quoted[i] = regexp.QuoteMeta(s)
		}
		// Assembly functions are named with an ".abi0" suffix (Go's
		// stack-based calling convention).
		pattern := "^(" + strings.Join(quoted, "|") + `)(\.abi0)?$`
		cmd = exec.Command(goBin, "tool", "objdump", "-s", pattern, exe)
		header = regexp.MustCompile(`(?m)^TEXT (\S+?)(?:\.abi0)?\(SB\)`)
	} else {
		objdump, err := exec.LookPath("objdump")
		if err != nil {
			t.Skip("objdump (LLVM or GNU binutils) not in PATH")
		}
		cmd = exec.Command(objdump, "-d", "--no-show-raw-insn", exe)
		// "0000000000401000 <simdnorm.f>:" (ELF), "<_simdnorm.f>:" (Mach-O)
		header = regexp.MustCompile(`(?m)^[0-9a-f]*\s*<_?([^>]+)>:$`)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", cmd, err, out)
	}

	want := map[string]bool{}
	for _, s := range symbols {
		want[s] = true
	}
	funcs := map[string]string{}
	starts := header.FindAllSubmatchIndex(out, -1)
	for i, m := range starts {
		name := string(out[m[2]:m[3]])
		if !want[name] {
			continue
		}
		end := len(out)
		if i+1 < len(starts) {
			end = starts[i+1][0]
		}
		funcs[name] = string(out[m[0]:end])
	}
	return funcs
}

func TestProofDisassembly(t *testing.T) {
	exp, ok := archExpectations[runtime.GOARCH]
	if !ok {
		t.Skipf("no SIMD implementation for GOARCH=%s", runtime.GOARCH)
	}
	if Implementation() == "scalar (pure Go)" { // normalize_other.go was compiled in
		t.Skip("built without GOEXPERIMENT=simd, so only the scalar code exists")
	}
	funcs := disassemble(t, exp.objdump, append(exp.simdFuncs, exp.scalarFuncs...))

	for _, name := range exp.simdFuncs {
		asm, ok := funcs[name]
		if !ok {
			t.Fatalf("symbol %s not found in binary", name)
		}
		packed := exp.packedMath.FindAllString(asm, -1)
		if len(packed) == 0 {
			t.Fatalf("%s contains no packed SIMD floating-point instructions:\n%s", name, asm)
		}
		t.Logf("%s: %d packed SIMD instructions, e.g. %q\n%s", name, len(packed), packed[0], asm)
	}

	// Contrast: the plain Go loops compile to scalar math only. This shows
	// that the Go compiler does not auto-vectorize.
	for _, name := range exp.scalarFuncs {
		asm, ok := funcs[name]
		if !ok {
			t.Fatalf("symbol %s not found in binary (was it inlined?)", name)
		}
		if m := exp.packedMath.FindString(asm); m != "" {
			t.Errorf("%s unexpectedly contains SIMD instruction %q", name, m)
		}
		scalar := exp.scalarMath.FindAllString(asm, -1)
		if len(scalar) == 0 {
			t.Fatalf("%s: expected scalar floating-point instructions:\n%s", name, asm)
		}
		t.Logf("%s: 0 packed SIMD instructions, scalar math only, e.g. %q\n%s", name, scalar[0], asm)
	}
}

// TestProofLaneFingerprint exploits float32 rounding to reveal how many
// independent partial sums the implementation keeps.
//
// Input: v = [4096, 1, 1, ..., 1] (n elements). Then 4096² = 2^24, and above
// 2^24 float32 can only represent even integers, so 2^24 + 1 rounds back to
// 2^24 (round-half-to-even). A sequential loop therefore gets stuck:
//
//	scalar:  2^24 + 1 + 1 + ... = 2^24            (every +1 is lost)
//
// With L parallel lanes, element i goes to lane i mod L. Only lane 0 holds
// the large 2^24; the other L-1 lanes each add n/L ones exactly. The sums
// are combined only at the end:
//
//	SIMD:    2^24 + (L-1)·(n/L)
//
// A sequential loop cannot produce this number, so getting it shows that
// lanes really ran side by side. (The exact answer is 2^24 + n - 1. Parallel
// lanes are also more accurate here.)
func TestProofLaneFingerprint(t *testing.T) {
	const n = 1024
	v := make([]float32, n)
	v[0] = 4096
	for i := 1; i < n; i++ {
		v[i] = 1
	}

	lanes := accumulatorLanes()
	want := float32(1<<24 + (lanes-1)*(n/lanes))
	seq := sumSquaresScalar(v)
	got := sumSquares(v)

	t.Logf("implementation:   %s", Implementation())
	t.Logf("exact answer:     %d", 1<<24+n-1)
	t.Logf("sequential loop:  %.0f", seq)
	t.Logf("sumSquares:       %.0f  (expected for %d lanes: %.0f)", got, lanes, want)

	if seq != 1<<24 {
		t.Fatalf("sanity check failed: sequential sum = %.0f, want %d", seq, 1<<24)
	}
	if got != want {
		t.Fatalf("sumSquares = %.0f, want %.0f (the fingerprint of %d parallel lanes)", got, want, lanes)
	}
	if lanes > 1 && got == seq {
		t.Fatal("result is identical to a sequential loop; SIMD path was not used")
	}
}

// TestProofProfiler samples the CPU while Normalize runs and checks where the
// time went. Static disassembly shows the SIMD code exists; the profile shows
// it is the code that actually executes.
func TestProofProfiler(t *testing.T) {
	exp, ok := archExpectations[runtime.GOARCH]
	if !ok || accumulatorLanes() == 1 {
		t.Skip("no SIMD implementation active on this platform")
	}
	if testing.Short() {
		t.Skip("profiling takes about a second; skipped in -short mode")
	}

	profPath := filepath.Join(t.TempDir(), "cpu.pprof")
	f, err := os.Create(profPath)
	if err != nil {
		t.Fatal(err)
	}
	r := rand.New(rand.NewPCG(9, 10))
	v := randomVector(r, 1536)
	if err := pprof.StartCPUProfile(f); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		for range 1000 {
			Normalize(v)
		}
	}
	pprof.StopCPUProfile()
	f.Close()

	exe, _ := os.Executable()
	goBin := goTool(t)
	top, err := exec.Command(goBin, "tool", "pprof", "-top", "-nodecount=8", exe, profPath).CombinedOutput()
	if err != nil {
		t.Fatalf("go tool pprof: %v\n%s", err, top)
	}

	// Rows look like:  "500ms 57.47% 57.47%  500ms 57.47%  simdnorm.scaleNEON"
	row := regexp.MustCompile(`(?m)^\s*\S+\s+([\d.]+)%\s+\S+\s+\S+\s+\S+\s+(\S+)`)
	var simdShare float64
	for _, m := range row.FindAllStringSubmatch(string(top), -1) {
		pct, _ := strconv.ParseFloat(m[1], 64)
		for _, fn := range exp.simdFuncs {
			if m[2] == fn {
				simdShare += pct
			}
		}
	}
	t.Logf("go tool pprof -top:\n%s", top)
	t.Logf("share of CPU samples inside SIMD functions: %.1f%%", simdShare)
	if simdShare < 50 {
		t.Errorf("only %.1f%% of samples were in SIMD functions, want most of them", simdShare)
	}

	// Per-line view: for the arm64 .s file every line is one instruction, so
	// this shows the samples landing on the vector instructions.
	focus := regexp.QuoteMeta(exp.simdFuncs[0])
	list, err := exec.Command(goBin, "tool", "pprof", "-list="+focus, exe, profPath).CombinedOutput()
	if err != nil {
		t.Fatalf("go tool pprof -list: %v\n%s", err, list)
	}
	t.Logf("go tool pprof -list=%s (samples per source line):\n%s", focus, list)
}
