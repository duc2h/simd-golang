# Each target shows one way to inspect or prove what the code does.
TESTBIN := simdnorm.test
FUNCS   := ^simdnorm\.(sumSquaresNEON|scaleNEON|sumSquaresScalar|scaleScalar)(\.abi0)?$$

.PHONY: test proof bench bench-amd64 asm compiler-asm profile amd64 clean

## test: vet (also checks the assembly's stack offsets) + all tests
test:
	go vet .
	go test -count=1 .

## proof: print the three kinds of evidence (disassembly, lane fingerprint, profile)
proof:
	go test -count=1 -v -run TestProof .

## bench: SIMD vs scalar for different vector sizes
bench:
	go test -run '^$$' -bench . .

## bench-amd64: same benchmarks for the AVX2 version (via Rosetta 2 on a Mac: timings are not real Intel speed)
bench-amd64:
	GOARCH=amd64 GOEXPERIMENT=simd go test -run '^$$' -bench . .

## asm: final machine code of the SIMD and scalar functions (arm64)
asm:
	go test -c -o $(TESTBIN) .
	go tool objdump -s '$(FUNCS)' $(TESTBIN)

## compiler-asm: what the Go compiler emits for the pure-Go loop (no auto-vectorization)
compiler-asm:
	go build -a -gcflags='-S' . 2>&1 | sed -n '/^simdnorm\.sumSquaresScalar STEXT/,/^simdnorm\..* STEXT/p'

## profile: CPU profile of Normalize; shows samples per assembly line/instruction
profile:
	go test -run '^$$' -bench 'Normalize/n=1536/simd' -cpuprofile cpu.pprof -o $(TESTBIN) .
	go tool pprof -top -nodecount=6 $(TESTBIN) cpu.pprof
	go tool pprof -disasm 'sumSquaresNEON' $(TESTBIN) cpu.pprof

## amd64: build the AVX2 version (simd/archsimd) and run it under Rosetta 2
amd64:
	GOARCH=amd64 GOEXPERIMENT=simd go test -count=1 -v -run 'TestProof' .

clean:
	rm -f $(TESTBIN) cpu.pprof
