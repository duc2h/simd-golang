# simdnorm: make a vector's length equal to 1, using SIMD

This guide is for beginners. Every special word is explained the first time
it appears, and there is a [Glossary](#glossary-word-list) at the end.

**Contents**

1. [What does this project do?](#1-what-does-this-project-do)
2. [What is SIMD?](#2-what-is-simd)
3. [Should I use SIMD for normalizing?](#3-should-i-use-simd-for-normalizing)
4. [How the code works](#4-how-the-code-works)
5. [How to prove the code really uses SIMD (the ideas)](#5-how-to-prove-the-code-really-uses-simd-the-ideas)
6. [Things that may surprise you](#6-things-that-may-surprise-you)
7. [Every command, with real output, explained](#7-every-command-with-real-output-explained)
8. [Testing Intel / AMD (AVX2, AVX-512) when you have a Mac](#8-testing-intel--amd-avx2-avx-512-when-you-have-a-mac)
9. [Glossary](#glossary-word-list)

---

## 1. What does this project do?

A **vector** here is just a list of numbers, for example `[3, 4]`.

A vector has a **length**, also called **magnitude**. You compute it like this:

```
length of [3, 4] = √(3² + 4²) = √(9 + 16) = √25 = 5
```

To **normalize** a vector means: divide every number by the length, so the
new length becomes exactly **1**.

```
[3, 4]  →  [3/5, 4/5]  =  [0.6, 0.8]       new length = √(0.36 + 0.64) = 1 ✓
```

In Go:

```go
v := []float32{3, 4}
length := simdnorm.Normalize(v)   // length = 5, and v is now [0.6, 0.8]
```

Normalizing always has **2 steps**:

| Step | What it does | Example with [3, 4] |
|---|---|---|
| Step 1: sum of squares | multiply each number by itself, add them all up | 3·3 + 4·4 = 25 |
| Step 2: scale | multiply each number by `1 / √sum` | 3 · (1/5), 4 · (1/5) |

---

## 2. What is SIMD?

**SIMD** = **S**ingle **I**nstruction, **M**ultiple **D**ata.
It means: **one CPU command works on several numbers at the same time.**

### An analogy: a cashier

- **Normal code (called "scalar")**: the cashier scans **1 item** at a time.
- **SIMD code**: the cashier scans **4 items** with one beep.

```
Normal (scalar):   a0×a0   a1×a1   a2×a2   a3×a3      → 4 commands
SIMD:              [a0 a1 a2 a3] × [a0 a1 a2 a3]      → 1 command
```

### Two words you will see often

- **Register**: a tiny storage box *inside* the CPU. The CPU can only
  calculate with numbers that are in registers.
- **Lane**: one slot inside a SIMD register. A SIMD register is like an egg
  tray. Each slot (lane) holds one number.

```
Normal register:   [ 3.0 ]                    ← holds 1 number
SIMD register:     [ 3.0 | 4.0 | 1.5 | 2.0 ]  ← holds 4 numbers (4 lanes)
```

Different CPUs have different SIMD "trays":

| Your CPU | SIMD name | Numbers per command |
|---|---|---|
| Apple M1/M2/M3/M4, phones, AWS Graviton (**arm64**) | NEON | 4 |
| Most Intel / AMD (**amd64**) | AVX2 | 8 |
| Some Intel / AMD server CPUs (**amd64**) | AVX-512 | 16 |

---

## 3. Should I use SIMD for normalizing?

**Short answer: yes, if your vector has more than about 64 numbers. For tiny
vectors (like 3D points) it does not help.**

Measured on an Apple M4 with `make bench` (see [section 7.2](#72-make-bench-how-fast-is-it)).
"ns" = nanosecond = one billionth of a second. Smaller is faster.

| How many numbers in the vector | Normal Go | SIMD version | Result |
|---:|---:|---:|---|
| 3 | 10.2 ns | 11.8 ns | no real difference (see below) |
| 16 | 20.4 ns | 14.2 ns | 1.4× faster |
| 128 | 139 ns | 23 ns | 6.1× faster |
| 1536 | 1554 ns | 171 ns | 9.1× faster |
| 65 536 | 66 µs | 7.4 µs | 8.9× faster |
| 4 million | 4.50 ms | 0.48 ms | 9.4× faster |

**Why no gain for 3 numbers?**
The SIMD code works in groups of **16 numbers**. A 3-number vector does not
fill even one group, so the code simply uses the normal loop. The small
difference in the table is random noise: run it again and it can flip.

**Tip:** if you have *many* small vectors (for example a million 3D points),
put all of them into one big list and process them together.

---

## 4. How the code works

### Why is there an assembly file?

- **Assembly** is a programming language that is very close to the CPU. Each
  line is one CPU command.
- The Go **compiler** turns your Go code into CPU commands. But the Go
  compiler **never creates SIMD commands by itself**. It always uses the
  1-number-at-a-time kind. (You will see the proof in [section 7.4](#74-make-compiler-asm-what-the-go-compiler-made).)
- So to get SIMD on a Mac, we must write those commands ourselves in
  assembly. That is the file `normalize_arm64.s`.
- (On Intel/AMD, Go 1.26 has a new experimental package `simd/archsimd` that
  lets you write SIMD in normal Go. It does not support Mac/ARM yet.)

### Which file does what

| File | What is inside |
|---|---|
| `normalize.go` | `Normalize()` (the function you call) and the normal 1-at-a-time version `NormalizeScalar()` |
| `normalize_arm64.s` | the SIMD code for Mac / ARM, written in assembly |
| `normalize_arm64.go` | connects Go to the assembly file |
| `normalize_amd64_simd.go` | the SIMD code for Intel / AMD (AVX2), using `simd/archsimd` |
| `normalize_other.go` | fallback for other CPUs: normal Go, no SIMD |
| `normalize_test.go` | checks that the answers are correct |
| `simdproof_test.go` | checks that SIMD is **really** being used (see section 5) |
| `bench_test.go` | measures speed |

### Only ONE SIMD version is inside each program

Go chooses the files **when it builds the program**, not while it runs. It
looks at the file name ending (`_arm64`) and the `//go:build` line at the top of
each file, and **completely ignores** the files that don't match:

| You build for | Files used | Files ignored |
|---|---|---|
| Mac / ARM (normal `go build` on your Mac) | `normalize.go`, `normalize_arm64.go`, `normalize_arm64.s` | `normalize_amd64_simd.go`, `normalize_other.go` |
| Intel/AMD + `GOEXPERIMENT=simd` | `normalize.go`, `normalize_amd64_simd.go` | `normalize_arm64.go`, `normalize_arm64.s`, `normalize_other.go` |
| anything else | `normalize.go`, `normalize_other.go` | all the SIMD files |

You can ask Go to show this list:

```sh
go list -f '{{.GoFiles}} {{.SFiles}}' .                          # → [normalize.go normalize_arm64.go] [normalize_arm64.s]
GOARCH=amd64 GOEXPERIMENT=simd go list -f '{{.GoFiles}}' .       # → [normalize.go normalize_amd64_simd.go]
```

**This is why `normalize_amd64_simd.go` looks "unused" on your Mac.** Your
editor builds for your Mac, so it skips that file and may show it greyed out.
It *is* used, but only in Intel/AMD builds.

All 3 versions provide functions with the **same names**: `sumSquares()` and
`scale()`. `Normalize()` simply calls `sumSquares()` and `scale()`. It does not
know or care which version is inside:

```
Normalize()  →  sumSquares()  →  on Mac:   sumSquaresNEON()   (normalize_arm64.s)
                              →  on Intel: sumSquaresAVX2()   (normalize_amd64_simd.go)
                              →  others:   sumSquaresScalar() (normalize.go)
```

### What the SIMD loop does (Mac version, simplified)

```
repeat until the vector is finished:
    load the next 16 numbers into 4 registers (4 numbers each)        ← VLD1
    for each register:  total += number × number   (4 lanes at once)  ← VFMLA
at the end: add all the lanes together into one final total
```

The assembly handles numbers in groups of 16. If the vector length is not a
multiple of 16 (for example 100 = 6×16 + 4), the leftover 4 numbers are handled
by normal Go code.

---

## 5. How to prove the code really uses SIMD (the ideas)

You should not just *believe* the code is SIMD. You can **check** it.
This section explains the 3 ideas. [Section 7](#7-every-command-with-real-output-explained)
shows the real output of each one.

> ⚠️ "It runs faster" is **not** a proof. Code can become faster for other
> reasons. Use the checks below.

| Way | Question it answers | Tool |
|---|---|---|
| 1. Look at the CPU commands | Is SIMD code **inside** the program? | disassembler |
| 2. Trick number | Did the SIMD code **really run**? | a special test input |
| 3. Profiler | Is the time **spent** in the SIMD code? | profiler |

### Way 1: look at the CPU commands (disassembler)

Your Go program is turned into a list of CPU commands. A **disassembler** is a
tool that shows you that list in readable text. If you see SIMD commands, the
SIMD code exists.

How to tell SIMD from normal commands:

| CPU | SIMD looks like | Normal (1 number) looks like |
|---|---|---|
| Mac (arm64) | `V4.S4`: `V` register, `.S4` = 4 numbers inside | `F1`: `F` register = 1 number |
| Intel/AMD | name ends in `ps` (e.g. `vmulps`) + `ymm` register | name ends in `ss` (e.g. `mulss`) |

(`ps` = **p**acked **s**ingle = many numbers packed together. `ss` = **s**calar **s**ingle = 1 number.)

**Limitation:** this only proves the SIMD code is **inside** the program, not
that it **runs**. A program can check the CPU and decide to use normal code instead.

### Way 2: a trick number that only SIMD can produce

SIMD keeps several separate totals, one per lane. Normal code keeps only one
total. We choose input numbers that give a *different answer* depending on how
many totals there are.

**Background:** a `float32` number is like a calculator with limited digits.
When a number gets very big (16,777,216 or more), adding `+1` gets **lost**,
because the calculator has no room for that last digit.

**The input:** `[4096, 1, 1, 1, … , 1]` (1024 numbers in total)

- `4096 × 4096 = 16,777,216`, which is exactly the "too big to add +1" size.
- Then we add `1 × 1 = 1`, 1023 times.

**Normal code (1 worker, 1 total):**
```
16,777,216 + 1 → lost → 16,777,216
           + 1 → lost → 16,777,216
           ...  every +1 is lost
answer: 16,777,216
```

**SIMD on a Mac (16 workers, each with their own total):**
```
worker 1:       gets the big number → all its +1 are lost → 16,777,216
workers 2..16:  only get the 1s → they count normally → 64 each
at the end:     16,777,216 + 15 × 64 = 16,778,176
```

Normal code **can never** produce 16,778,176. So if we get that number, the
SIMD code **really ran**. The number even tells us how many lanes worked.

### Way 3: the profiler (where is the time spent?)

A **profiler** pauses the program about 100 times per second and writes down
"which function is running right now?". If most pauses land inside the SIMD
functions, then the SIMD functions are what really runs.

### Bonus: hardware counters (advanced, optional)

Modern CPUs can **count** how many SIMD commands they ran. This is the
strongest proof, but the tools are different on every system:
- Linux + Intel: the `perf stat` command
- Mac: the Instruments app (comes with Xcode), template "CPU Counters"

### Why use more than one way?

This really happened while building the project. The Intel version was tested
on a Mac through **Rosetta** (Apple's tool that runs Intel programs on Apple
chips):

- Way 1 said ✅: the SIMD commands were inside the program.
- Way 2 said ❌: the answer was 16,777,216, which means normal code ran.

The reason: Rosetta does not support one feature (FMA) the code asked for, so
the program quietly switched to normal code. **Code being inside the program is
not the same as code that runs.** (This is fixed now.)

---

## 6. Things that may surprise you

- **SIMD and normal code can give slightly different answers** in the last
  digit (like `24.999998` vs `25.000002`). They add the numbers in a
  different order, and `float32` rounding depends on the order. This is normal.
  The tests allow a tiny difference.
- **A vector of all zeros cannot be normalized** (you cannot divide by 0).
  `Normalize` leaves it unchanged and returns `0`.
- **Extremely large numbers** (bigger than about 10¹⁹) become "infinity" when
  squared in `float32`.

---

## 7. Every command, with real output, explained

All outputs below are **real**. They were captured on an Apple M4 (macOS,
Go 1.26.3). Your numbers will be a bit different each run; that is normal.
Long outputs are shortened with `...`.

| Command | What it answers | Section |
|---|---|---|
| `make test` | Are the answers correct? | [7.1](#71-make-test-are-the-answers-correct) |
| `make bench` | How fast is it? | [7.2](#72-make-bench-how-fast-is-it) |
| `make asm` | Which CPU commands are inside the program? | [7.3](#73-make-asm-which-cpu-commands-are-inside-the-program) |
| `make compiler-asm` | Does the Go compiler create SIMD by itself? | [7.4](#74-make-compiler-asm-what-the-go-compiler-made) |
| `make proof` | All 3 proofs, as automatic tests | [7.5](#75-make-proof-all-3-proofs-as-automatic-tests) |
| `make profile` | Where is the time spent? | [7.6](#76-make-profile-where-is-the-time-spent) |
| `make amd64` | Does the Intel (AVX2) version work? | [7.7](#77-make-amd64-test-the-intel-version-on-a-mac) |
| `make bench-amd64` | How fast is the Intel (AVX2) version? | [7.8](#78-make-bench-amd64-speed-test-for-the-intel-version) |
| `make clean` | Delete the files the commands created | [7.9](#79-make-clean) |

---

### 7.1 `make test`: are the answers correct?

**Runs:** `go vet .` then `go test -count=1 .`

**Real output:**

```
go vet .
go test -count=1 .
ok  	simdnorm	2.855s
```

**How to read it:**

- `go vet .` printed nothing. That means it found **no problems**. `go vet` is
  a checker. One of its checks compares the assembly file with the Go
  declarations, and catches mistakes like "the assembly reads the input from
  the wrong place".
- `ok  simdnorm  2.855s` means **every test passed**, in 2.855 seconds.
  (`-count=1` means "really run the tests, don't reuse an old result".)

**What the tests check** (in `normalize_test.go`):

| Test | What it checks |
|---|---|
| `TestNormalizeKnownValue` | `[3, 4]` becomes `[0.6, 0.8]` and the length is 5 |
| `TestNormalizeZeroVector` | `[0, 0, …]` stays unchanged and returns 0 |
| `TestNormalizeUnitLength` | for 20 different sizes (1 to 4099 numbers), the result really has length 1 |
| `TestSumSquaresMatchesScalar` | SIMD sum ≈ normal sum (tiny difference allowed) |
| `TestScaleBitIdenticalToScalar` | SIMD scaling gives **exactly** the same bits as normal scaling |
| `TestDoesNotWriteOutsideSlice` | the assembly does not overwrite memory next to the vector |
| `TestNormalizeMatchesScalar` | the full `Normalize` ≈ `NormalizeScalar` |

The sizes include tricky ones like 15, 16, 17, 31, 33 (just below, at, and just
above a group of 16). This makes sure the "leftover numbers" code is correct.

**✅ Proves:** the results are correct.
**❌ Does not prove:** that SIMD was used. Normal code would also pass these tests.

---

### 7.2 `make bench`: how fast is it?

**Runs:** `go test -run '^$' -bench . .`
(`-run '^$'` = skip the normal tests; `-bench .` = run all speed tests.)

**Real output (shortened):**

```
cpu: Apple M4
BenchmarkNormalize/n=3/simd-neon-10         	98894976	        11.76 ns/op	 1020.40 MB/s
BenchmarkNormalize/n=3/scalar-10            	100000000	        10.22 ns/op	 1174.51 MB/s
BenchmarkNormalize/n=16/simd-neon-10        	88171816	        14.16 ns/op	 4518.29 MB/s
BenchmarkNormalize/n=16/scalar-10           	59701370	        20.39 ns/op	 3138.39 MB/s
BenchmarkNormalize/n=128/simd-neon-10       	57029251	        22.62 ns/op	22634.08 MB/s
BenchmarkNormalize/n=128/scalar-10          	 8142240	       138.8 ns/op	 3688.28 MB/s
...
BenchmarkNormalize/n=1536/simd-neon-10      	 6965197	       170.8 ns/op	35975.58 MB/s
BenchmarkNormalize/n=1536/scalar-10         	  782961	      1554 ns/op	 3953.93 MB/s
...
BenchmarkSumSquares/n=1536/simd-neon-10     	13122151	        93.20 ns/op
BenchmarkSumSquares/n=1536/scalar-10        	 1000000	      1127 ns/op
BenchmarkSumSquares/n=1536/scalar-4acc-10   	 3781520	       321.3 ns/op
```

**Which SIMD is "simd"?** The name tells you: `simd-neon` = the Mac (ARM)
assembly version. When you build for Intel/AMD, the same benchmark is called
`simd-avx2` (see [7.8](#78-make-bench-amd64-speed-test-for-the-intel-version)).
One program only contains one SIMD version (see section 4), so one benchmark
run can never show NEON and AVX2 side by side.

**How to read one line:**

```
BenchmarkNormalize/n=1536/simd-neon-10   6965197     170.8 ns/op     35975.58 MB/s
        │            │       │      │       │             │                │
        │            │       │      │       │             │                └ data processed per second
        │            │       │      │       │             └ time for ONE Normalize call  ← the important one
        │            │       │      │       └ how many times Go ran it to get a stable average
        │            │       │      └ Go was allowed to use 10 CPU cores (the M4 has 10)
        │            │       └ which version: simd-neon, simd-avx2, or scalar (normal)
        │            └ the vector has 1536 numbers
        └ the name of the speed test
```

**What the results say:**

1. **Big vectors:** for 1536 numbers, normal = 1554 ns, SIMD = 171 ns, so
   SIMD is **9× faster**.
2. **Tiny vectors:** for 3 numbers, both run the *same* normal loop (fewer than
   16 numbers, so there is no SIMD group). The difference is noise.
3. **The `BenchmarkSumSquares` lines** show *where* the speedup comes from
   (only step 1, the sum of squares):

   | Version | Time | What it does |
   |---|---:|---|
   | `scalar` | 1127 ns | normal Go, **1 total**. Each `+` must wait for the previous `+` to finish |
   | `scalar-4acc` | 321 ns | normal Go, **4 separate totals**. The CPU can work on 4 additions at once → 3.5× faster, without SIMD! |
   | `simd-neon` | 93 ns | 4 registers × 4 lanes = **16 totals** → another 3.5× faster |

   **Lesson:** part of the gain comes simply from "not waiting". You can get
   that part in normal Go too. SIMD gives the rest.

**✅ Proves:** the SIMD version is faster for medium and large vectors.
**❌ Does not prove:** that the speed comes from SIMD. The `scalar-4acc` line
shows that normal code can also become faster. That is why we need proofs 7.3–7.6.

---

### 7.3 `make asm`: which CPU commands are inside the program?

**Runs:**
1. `go test -c -o simdnorm.test .` builds the test program into a file
   (`-c` = build only, don't run).
2. `go tool objdump -s '<function names>' simdnorm.test` is the
   **disassembler**: it prints the CPU commands of the 4 functions we care about.

**How to read one line:**

```
normalize_arm64.s:30    0x10011aa84    4e24cc80    VFMLA V4.S4, V4.S4, V0.S4
        │                    │             │               │
        │                    │             │               └ the command, in readable text
        │                    │             └ the command as raw bytes: this is what the CPU really reads
        │                    └ the address in memory where this command lives
        └ which source file and line it came from
```

**Real output, part 1: the normal Go loop** (`sumSquaresScalar`, from `normalize.go`):

```
TEXT simdnorm.sumSquaresScalar(SB) normalize.go
  normalize.go:49   FMOVS (R0)(R2<<2), F1     ← load ONE number into register F1
  normalize.go:50   FMADDS F1, F0, F1, F0     ← total(F0) = total + F1 × F1   (1 number)
  normalize.go:49   ADD $1, R2, R2            ← i = i + 1   (go to the next number)
  normalize.go:49   CMP R2, R1                ← is i < length?
  normalize.go:49   BGT -4(PC)                ← if yes, jump back 4 lines (repeat)
```

Every register here is `F` (1 number). One round of the loop = **1 number**.
For 1536 numbers, this loop runs **1536 rounds**.

**Real output, part 2: the SIMD loop** (`sumSquaresNEON`, from `normalize_arm64.s`):

```
TEXT simdnorm.sumSquaresNEON.abi0(SB) normalize_arm64.s
  normalize_arm64.s:15  LSR $4, R1, R1                               ← rounds = length ÷ 16
  normalize_arm64.s:20  VEOR V0.B16, V0.B16, V0.B16                  ← set total V0 = [0 0 0 0]  (same for V1, V2, V3)
  ...
  normalize_arm64.s:28  VLD1.P 64(R0), [V4.S4, V5.S4, V6.S4, V7.S4]  ← load 16 numbers in ONE command
  normalize_arm64.s:30  VFMLA V4.S4, V4.S4, V0.S4                    ← V0 += V4 × V4   (4 numbers)
  normalize_arm64.s:31  VFMLA V5.S4, V5.S4, V1.S4                    ← V1 += V5 × V5   (4 numbers)
  normalize_arm64.s:32  VFMLA V6.S4, V6.S4, V2.S4                    ← V2 += V6 × V6   (4 numbers)
  normalize_arm64.s:33  VFMLA V7.S4, V7.S4, V3.S4                    ← V3 += V7 × V7   (4 numbers)
  normalize_arm64.s:34  SUBS $1, R1, R1                              ← rounds = rounds - 1
  normalize_arm64.s:35  BNE -6(PC)                                   ← if rounds left, jump back (repeat)
  normalize_arm64.s:42  VFMLA V1.S4, V8.S4, V0.S4                    ← after the loop: add the 4 totals together…
  ...
  normalize_arm64.s:50  FADDS F1, F0, F0                             ← …then add the 4 lanes into 1 final number
```

The registers are `V…​.S4` (4 numbers each). One round of the loop = **16 numbers**.
For 1536 numbers, this loop runs only **96 rounds** (1536 ÷ 16).

| | Normal loop | SIMD loop |
|---|---|---|
| Numbers per round | 1 | 16 |
| Rounds for 1536 numbers | 1536 | 96 |
| Register type | `F` (1 number) | `V….S4` (4 numbers) |

**Small details you may notice:**

- `.abi0` at the end of the name marks a function written in assembly.
- `VFMLA … V8.S4 …` after the loop: V8 is filled with `[1 1 1 1]`. Go's
  assembler has no "SIMD add" command, so we do `total += other × 1`, which is
  the same as an add.
- `scaleNEON` (also printed) works the same way: load 16 numbers, multiply
  by the scale with `VFMLA`, and store 16 numbers back with `VST1.P`.
- `?` at the end of a function: empty filler bytes, not a command. Ignore it.

**✅ Proves:** SIMD commands are inside the program, and the normal loop has none.
**❌ Does not prove:** that the SIMD commands actually run.

---

### 7.4 `make compiler-asm`: what the Go compiler made

**Runs:** `go build -a -gcflags='-S' .`
(`-gcflags='-S'` asks the Go compiler to print the commands it creates.
`-a` = rebuild everything, so the output is printed even if it was built before.)

**Real output (shortened):**

```
simdnorm.sumSquaresScalar STEXT size=48 args=0x18 locals=0x0 funcid=0x0 align=0x0 leaf
	(normalize.go:47)	TEXT	simdnorm.sumSquaresScalar(SB), LEAF|NOFRAME|ABIInternal, $0-24
	(normalize.go:47)	FUNCDATA	$0, gclocals·wvjpxkknJ4nY1JtrArJJaw==(SB)
	(normalize.go:47)	PCDATA	$3, $1
	(normalize.go:49)	FMOVS	ZR, F0                ← total = 0
	(normalize.go:49)	FMOVS	(R0)(R2<<2), F1       ← load 1 number
	(normalize.go:50)	FMADDS	F1, F0, F1, F0        ← total += number × number
	(normalize.go:49)	ADD	$1, R2, R2
	(normalize.go:49)	CMP	R2, R1
	(normalize.go:49)	BGT	16
	(normalize.go:52)	RET	(R30)
	0x0000 e0 07 00 f9 e2 03 1f aa e0 03 27 1e 04 00 00 14  ..........'.....
```

**How to read it:**

- This is the loop from `normalize.go`:
  ```go
  for _, x := range v {
      s += x * x
  }
  ```
- `FUNCDATA` / `PCDATA` lines are notes for Go's garbage collector. They are
  not CPU commands. Ignore them.
- The lines with hex bytes at the end (`e0 07 00 f9 …`) are the same commands
  as raw bytes.
- The math command is `FMADDS` on `F` registers: **1 number at a time**.

**Difference from 7.3:** `make asm` shows the *final program*. This command
shows what the *compiler* produced, one step earlier. Both agree.

**✅ Proves:** the Go compiler does **not** turn a normal loop into SIMD by
itself. If you want SIMD in Go, you must write it yourself (assembly, or
`simd/archsimd` on Intel/AMD).

---

### 7.5 `make proof`: all 3 proofs, as automatic tests

**Runs:** `go test -count=1 -v -run TestProof .`
(`-v` = verbose: show all the messages. `-run TestProof` = only run tests whose name starts with `TestProof`.)

These are the 3 ideas from section 5, written as tests. Each test **fails** if
the proof fails, so you don't have to read the output by eye every time.

**Real output, test 1: `TestProofDisassembly`** (Way 1: disassembler)

```
=== RUN   TestProofDisassembly
    simdnorm.sumSquaresNEON: 7 packed SIMD instructions, e.g. "VFMLA V4.S4"
    simdnorm.scaleNEON: 4 packed SIMD instructions, e.g. "VFMLA V0.S4"
    simdnorm.sumSquaresScalar: 0 packed SIMD instructions, scalar math only, e.g. "FMADDS F1"
    simdnorm.scaleScalar: 0 packed SIMD instructions, scalar math only, e.g. "FMULS F0"
--- PASS: TestProofDisassembly (0.31s)
```

The test does the same thing as `make asm` (7.3), then **counts** the SIMD commands:
- The 2 SIMD functions have 7 and 4 SIMD commands → SIMD code exists ✅
- The 2 normal Go functions have 0 SIMD commands → the compiler made no SIMD ✅

**Real output, test 2: `TestProofLaneFingerprint`** (Way 2: trick number)

```
=== RUN   TestProofLaneFingerprint
    implementation:   arm64 NEON (hand-written assembly)
    exact answer:     16778239
    sequential loop:  16777216
    sumSquares:       16778176  (expected for 16 lanes: 16778176)
--- PASS: TestProofLaneFingerprint (0.00s)
```

| Line | Meaning |
|---|---|
| `implementation` | which version this computer uses: the Mac SIMD (NEON) version |
| `exact answer` | the true mathematical answer: 16,777,216 + 1023 |
| `sequential loop` | the normal Go loop: every `+1` got lost (see section 5, Way 2) |
| `sumSquares` | our SIMD function: **16,778,176**. Only 16 parallel totals can give this number |

**The SIMD code really ran, with 16 lanes.** ✅

**Real output, test 3: `TestProofProfiler`** (Way 3: profiler)

```
=== RUN   TestProofProfiler
              flat  flat%   sum%        cum   cum%
             530ms 62.35% 62.35%      530ms 62.35%  simdnorm.scaleNEON
             300ms 35.29% 97.65%      300ms 35.29%  simdnorm.sumSquaresNEON
              10ms  1.18% 98.82%       10ms  1.18%  runtime.pthread_cond_signal
              10ms  1.18%   100%       10ms  1.18%  simdnorm.sumSquaresScalar
    ...
    share of CPU samples inside SIMD functions: 97.6%
--- PASS: TestProofProfiler (1.46s)
PASS
ok  	simdnorm	2.001s
```

The test calls `Normalize` over and over for 1 second while the profiler
watches. **97.6% of the time was inside the 2 SIMD functions.** ✅
(The test passes if this is more than 50%. The columns are explained in 7.6.)

---

### 7.6 `make profile`: where is the time spent?

**Runs 3 commands:**

1. `go test -run '^$' -bench 'Normalize/n=1536/simd' -cpuprofile cpu.pprof -o simdnorm.test .`
   runs the speed test for 1536 numbers, and records a profile into the file `cpu.pprof`.
2. `go tool pprof -top -nodecount=6 simdnorm.test cpu.pprof`
   shows the top 6 functions that used the most time.
3. `go tool pprof -disasm 'sumSquaresNEON' simdnorm.test cpu.pprof`
   shows the time **per CPU command** inside `sumSquaresNEON`.

**Real output, part 1: the top functions**

```
Duration: 1.31s, Total samples = 1.06s (80.69%)
      flat  flat%   sum%        cum   cum%
     0.65s 61.32% 61.32%      0.65s 61.32%  simdnorm.scaleNEON
     0.37s 34.91% 96.23%      0.37s 34.91%  simdnorm.sumSquaresNEON
     0.02s  1.89% 98.11%      0.02s  1.89%  runtime.pthread_cond_signal
     0.01s  0.94% 99.06%      0.03s  2.83%  runtime.startm
```

| Column | Meaning |
|---|---|
| `flat` | time spent **inside this function itself** |
| `flat%` | the same, as a percentage of all time |
| `sum%` | running total of `flat%` from the top row down |
| `cum` | time inside this function **plus** the functions it calls |
| `cum%` | the same, as a percentage |

**What it says:**
- 61% of the time is in `scaleNEON` (step 2) and 35% in `sumSquaresNEON` (step 1).
  Together that is **96%**, so the SIMD code is what really runs.
- Why is scaling slower than summing? Summing only *reads* the numbers.
  Scaling must *read* them and also *write* them back to memory.
- `runtime.…` lines are Go's own internal work (scheduling). They are tiny. Ignore them.

**Real output, part 2: time per CPU command**

```
ROUTINE ======================== simdnorm.sumSquaresNEON.abi0
     370ms      370ms (flat, cum) 34.91% of Total
         .          .  10011aa80: VLD1.P 64(R0), [V4.S4, V5.S4, V6.S4, V7.S4]
     370ms      370ms  10011aa84: VFMLA V4.S4, V4.S4, V0.S4      ;normalize_arm64.s:30
         .          .  10011aa88: VFMLA V5.S4, V5.S4, V1.S4      ;normalize_arm64.s:31
         .          .  10011aa8c: VFMLA V6.S4, V6.S4, V2.S4      ;normalize_arm64.s:32
         .          .  10011aa90: VFMLA V7.S4, V7.S4, V3.S4      ;normalize_arm64.s:33
```

`.` means 0 samples. All 370 ms landed on the **SIMD command `VFMLA`** inside the loop.

**Why only on the first `VFMLA`?** That command needs the numbers that
`VLD1.P` (the line above) is still loading from memory, so the CPU waits
there. When the profiler pauses the program, it usually catches the CPU
*waiting at that command*. The rule: profilers point at "where the CPU was
waiting", which is often one line *after* the slow part. Look at the whole loop,
not just one line.

**✅ Proves:** the SIMD commands are actually running, and almost all the time
is spent in them.

---

### 7.7 `make amd64`: test the Intel version on a Mac

**Runs:** `GOARCH=amd64 GOEXPERIMENT=simd go test -count=1 -v -run 'TestProof' .`

| Part | Meaning |
|---|---|
| `GOARCH=amd64` | "build the program for Intel/AMD CPUs" instead of your Mac's CPU |
| `GOEXPERIMENT=simd` | turn on Go's experimental `simd/archsimd` package (needed for the AVX2 code) |
| `go test …` | run the proof tests |

Your Mac cannot run Intel commands directly. macOS automatically uses
**Rosetta**, which translates Intel commands into Mac commands while the
program runs.

**Real output, test 1: disassembler** (Intel commands look different)

```
    simdnorm.sumSquaresAVX2: 11 packed SIMD instructions, e.g. "vmulps %ymm4, %ymm4, %ymm4"
        <_simdnorm.sumSquaresAVX2>:
         vmulps   %ymm4, %ymm4, %ymm4      ← multiply 8 numbers × 8 numbers
         vmulps   %ymm5, %ymm5, %ymm5
         vmulps   %ymm6, %ymm6, %ymm6
         vmulps   %ymm7, %ymm7, %ymm7
         vaddps   %ymm3, %ymm4, %ymm3      ← add 8 numbers to the total
         ...
    simdnorm.scaleAVX2: 1 packed SIMD instructions, e.g. "vmulps %ymm0, %ymm1, %ymm1"
    simdnorm.sumSquaresScalar: 0 packed SIMD instructions, scalar math only, e.g. "mulss"
         mulss    %xmm1, %xmm1             ← multiply 1 number
         addss    %xmm1, %xmm0             ← add 1 number
--- PASS: TestProofDisassembly (1.15s)
```

- `vmulps` / `vaddps`: names end in **`ps`** (packed, many numbers) and use
  **`ymm`** registers (256 bits = 8 numbers) → **AVX2 SIMD** ✅
- `mulss` / `addss`: names end in **`ss`** (scalar, 1 number) → normal code
- Intel commands are written in a different style than Mac commands (lowercase,
  `%` before registers). The idea is the same.

**Real output, test 2: trick number**

```
    implementation:   amd64 AVX2 (simd/archsimd intrinsics)
    exact answer:     16778239
    sequential loop:  16777216
    sumSquares:       16778208  (expected for 32 lanes: 16778208)
--- PASS: TestProofLaneFingerprint (0.00s)
```

The answer is **16,778,208** this time, not 16,778,176 like on the Mac.
The Intel version has 4 registers × 8 lanes = **32 totals**:
`16,777,216 + 31 × 32 = 16,778,208`. The trick number shows the lane count
changed from 16 to 32. ✅

**Real output, test 3: profiler**

```
             470ms 58.75% 58.75%      470ms 58.75%  simdnorm.scaleAVX2
             330ms 41.25%   100%      330ms 41.25%  simdnorm.sumSquaresAVX2
    share of CPU samples inside SIMD functions: 100.0%
```

100% of the time is in the AVX2 functions. ✅

**✅ Proves:** the Intel AVX2 code is correct and is actually chosen and used.
**❌ Does not prove:** real Intel **speed**. Rosetta translates the commands, so
the timing is not what a real Intel CPU would give.

---

### 7.8 `make bench-amd64`: speed test for the Intel version

**Runs:** `GOARCH=amd64 GOEXPERIMENT=simd go test -run '^$' -bench . .`

This is the same speed test as `make bench` (7.2), but built for Intel/AMD,
so the "simd" row uses the AVX2 code (`sumSquaresAVX2`, `scaleAVX2`).
On a Mac it runs through Rosetta.

**Real output (shortened):**

```
goarch: amd64
cpu: VirtualApple @ 2.50GHz
BenchmarkNormalize/n=3/simd-avx2-10         	100000000	        11.75 ns/op
BenchmarkNormalize/n=3/scalar-10            	100000000	        11.85 ns/op
BenchmarkNormalize/n=128/simd-avx2-10       	 23655088	        50.85 ns/op
BenchmarkNormalize/n=128/scalar-10          	 10381038	       114.1 ns/op
BenchmarkNormalize/n=1536/simd-avx2-10      	  3787221	       321.6 ns/op
BenchmarkNormalize/n=1536/scalar-10         	   834561	      1316 ns/op
...
BenchmarkSumSquares/n=1536/simd-avx2-10     	  8241314	       143.9 ns/op
BenchmarkSumSquares/n=1536/scalar-10        	  1386990	       864.0 ns/op
BenchmarkSumSquares/n=1536/scalar-4acc-10   	  3452534	       357.1 ns/op
```

**How to read it:**

- `goarch: amd64` → this program was built for Intel/AMD.
- `cpu: VirtualApple @ 2.50GHz` → not a real Intel CPU. This is **Rosetta**
  pretending to be one.
- `simd-avx2` → the "simd" rows now use the **AVX2** code.

**What the results say:**

| 1536 numbers | Normal Go | SIMD | Speedup |
|---|---:|---:|---:|
| Mac build (NEON, from 7.2) | 1554 ns | 171 ns | 9.1× |
| Intel build under Rosetta (AVX2) | 1316 ns | 322 ns | 4.1× |

- Even under Rosetta, AVX2 is about **4× faster** than normal code.
- **Do not compare the AVX2 numbers with the NEON numbers.** Rosetta has to
  translate every Intel command into Mac commands while the program runs. Your
  Mac's SIMD registers hold 4 numbers, so each 8-number AVX2 command most likely
  becomes several Mac commands. On a real Intel/AMD CPU the numbers would be
  different, probably better.

**✅ Proves:** the AVX2 version is faster than normal code, measured in the same conditions.
**❌ Does not prove:** how fast it is on a real Intel/AMD CPU. For that, run
`GOEXPERIMENT=simd go test -run '^$' -bench .` on a real Intel/AMD machine (see section 8).

---

### 7.9 `make clean`

**Runs:** `rm -f simdnorm.test cpu.pprof`

**Real output:**
```
rm -f simdnorm.test cpu.pprof
```

Deletes the 2 files that `make asm` and `make profile` created: the built test
program (`simdnorm.test`) and the profile recording (`cpu.pprof`).

---

## 8. Testing Intel / AMD (AVX2, AVX-512) when you have a Mac

### `amd64` is not the same as AVX2

- **`amd64`** is the **CPU family**: almost every Intel and AMD computer.
  (It is also called x86-64.)
- **SSE, AVX2 and AVX-512** are **SIMD features** *inside* that family. Newer
  CPUs have bigger SIMD registers:

| Feature | Register size | float32 numbers per command | Which CPUs have it |
|---|---|---|---|
| SSE | 128 bits | 4 | every amd64 CPU |
| AVX2 | 256 bits | 8 | almost all Intel/AMD since ~2013–2015 |
| AVX-512 | 512 bits | 16 | some Intel server CPUs (Xeon), AMD Zen 4 and newer |

- **This project's amd64 code uses AVX2 only.** Before using it, the program
  asks the CPU "do you have AVX2?". If the answer is no, it uses the normal Go
  loop. There is **no AVX-512 code** in this project yet. Go's
  `simd/archsimd` package supports AVX-512 (the `Float32x16` type), so it can
  be added.

### What you can test on your Mac

Here is what Rosetta told the program on this Mac (Apple M4):

```
AVX: true   AVX2: true   FMA: false   AVX512: false
```

| What you want to test | On your Mac? | How |
|---|---|---|
| Mac SIMD (NEON) | ✅ yes, real speed | `make test`, `make proof`, `make bench` |
| Intel AVX2: correctness + proofs | ✅ yes, through Rosetta | `make amd64` |
| Intel AVX2: rough speed (SIMD vs normal) | ✅ yes, through Rosetta | `make bench-amd64` |
| Intel AVX2: real speed | ❌ no | Rosetta speed is not real Intel speed |
| Intel AVX-512 | ❌ no | Rosetta does not support AVX-512 |

Docker on a Mac (`docker run --platform linux/amd64 …`) does **not** help.
It also emulates the Intel CPU (with Rosetta or QEMU), so it has the same limits.

### How to test on a real Intel / AMD CPU

| Option | AVX2 | AVX-512 | Real speed | Cost |
|---|:-:|:-:|:-:|---|
| A friend's / office Intel or AMD Linux or Windows PC | ✅ | depends on the CPU | ✅ | free |
| **GitHub Actions** (free Linux machines that run your tests on every push) | ✅ | sometimes, depends on the machine you get | ✅ | free for public repos |
| **Cloud computer** for an hour, e.g. AWS `c7i` (Intel) or `c7a` (AMD Zen 4), Google Cloud `c3` | ✅ | ✅ | ✅ | a few cents per hour |
| **Intel SDE**, a free Intel tool that *pretends* to be a newer CPU | ✅ | ✅ | ❌ | free, but needs an Intel/AMD PC (does not run on a Mac M-chip) |

**On the Linux machine**, first check which features the CPU has:

```sh
grep -o -w 'avx2\|avx512f\|fma' /proc/cpuinfo | sort -u
```

```
avx2        ← has AVX2
avx512f     ← has AVX-512 (line missing = no AVX-512)
fma
```

Then run the tests. On a real Intel/AMD machine you do **not** need
`GOARCH=amd64` (it is already amd64), but you still need the experiment flag:

```sh
GOEXPERIMENT=simd go test -v -run TestProof .     # proofs
GOEXPERIMENT=simd go test -run '^$' -bench .      # real AVX2 speed
```

Without `GOEXPERIMENT=simd`, Go uses `normalize_other.go` (normal code, no SIMD).
The trick-number test then shows `16777216` with 1 lane.

---

## Glossary (word list)

| Word | Simple meaning |
|---|---|
| **Vector** | a list of numbers, e.g. `[3, 4]` |
| **Length / magnitude** | how "long" a vector is: `√(sum of each number²)` |
| **Normalize** | change the vector so its length is 1 (keep the direction) |
| **float32** | a decimal number stored in 32 bits (about 7 correct digits) |
| **CPU** | the chip that runs your program |
| **Instruction / command** | one basic action the CPU can do, like "add" or "multiply" |
| **SIMD** | one instruction that works on several numbers at once |
| **Scalar** | the normal way: one instruction works on one number |
| **Register** | a tiny, very fast storage box inside the CPU |
| **Lane** | one slot inside a SIMD register (like one cup in an egg tray) |
| **NEON** | the SIMD feature of ARM CPUs (Apple M-chips): 4 float32 per instruction |
| **SSE / AVX2 / AVX-512** | SIMD features of Intel/AMD CPUs: 4 / 8 / 16 float32 per instruction |
| **FMA** | "fused multiply-add": does `a × b + c` in one instruction |
| **arm64 / amd64** | CPU families. arm64 = Apple M-chips, phones. amd64 = most Intel/AMD PCs and servers |
| **GOARCH** | tells Go which CPU family to build the program for (`arm64`, `amd64`, …) |
| **GOEXPERIMENT** | turns on experimental Go features, e.g. `simd` |
| **Compiler** | the tool that turns Go code into CPU instructions |
| **Assembly** | a language where each line is one CPU instruction |
| **Disassembler** | a tool that turns a compiled program back into readable instructions (`go tool objdump`) |
| **Profiler** | a tool that measures which functions use the most time (`go tool pprof`) |
| **Sample** | one "pause and look" by the profiler |
| **Benchmark** | a test that measures speed (`go test -bench`) |
| **Emulator** | a program that pretends to be a different CPU (Rosetta, QEMU, Intel SDE) |
| **Rosetta** | Apple's emulator that runs Intel programs on Apple chips |
| **ns / µs / ms** | nanosecond (1/1,000,000,000 s) / microsecond (1/1,000,000 s) / millisecond (1/1,000 s) |
