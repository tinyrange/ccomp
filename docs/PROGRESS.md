# Compiler Progress Report — Phase 3 (In Progress)

## Overview
This report summarizes the current state of the SSA-based C compiler per the multi-phase plan. We have a working end-to-end pipeline with SSA construction, basic optimizations, phi elimination, and an x86_64 SysV backend. Phase 3 is underway with control-flow support (if/else and while loops) and groundwork for extended language features.

## Implemented
- Frontend
  - Lexer: `int`, `return`, `if`, `else`, `while`, operators `+ - * /`, comparisons `== != < <= > >=`, punctuation `(){};,`.
  - Parser: functions with `int` params, blocks, decls/assignments, `return`, `if`/`else`, `while`, expression precedence for `* /`, `+ -`, relational, and equality.
- IR (SSA)
  - Values/ops: `const, add, sub, mul, div, eq, ne, lt, le, gt, ge, param, copy, phi, jmp, jnz`.
  - CFG on basic blocks: `Preds`/`Succs` with helper `addEdge`.
- SSA construction
  - Direct SSA during AST traversal (Braun-style read/write per block).
  - Unsealed-block handling with placeholder `phi` and sealing to fill operands; backedges supported for loops.
- SSA destruction
  - Phi elimination with critical-edge splitting and parallel copies on incoming edges.
- Optimizations (Phase 2)
  - Constant folding/propagation.
  - Dead code elimination (no-side-effect values).
  - Simple linear-scan register allocation (caller-saved, avoids `%rax`).
  - Peephole: immediates for `add/sub/imul` where applicable.
- Backend (x86_64, SysV AMD64)
  - Prologue/epilogue; 8-byte slot stack frame; param moves from arg regs to SSA homes.
  - Arithmetic, division (safe via `%rax/%rdx`), comparisons via `cmp`+`setcc`+`movzx`, copies, `jmp`/`jne`.
- CLI/Build
  - `ccomp` with `-o` anywhere in argv.
  - Sandboxed builds using local Go caches; `Makefile` targets `build`, `run`, `e2e`, `clean`, `test`.
  - Runtime `_start` for `-nostdlib` linking.
- Tests
  - ~30 tests in `tests/` with expectations: `// EXPECT: EXIT <n>` or `// EXPECT: COMPILE-FAIL`.
  - Runner `tools/run_tests.sh` compiles, links, runs, and checks results. `make test` wraps it.

## What Works End-to-End
- Expressions: integer arithmetic with precedence, parentheses.
- Declarations/assignments: local `int` variables.
- Comparisons: produce 0/1 with correct codegen.
- Control flow: `if/else` and `while` with loop-carried phi; example `while (i<10) i=i+1;` returns 10.
- Sanity check: `make e2e` returns exit code 14 for the sample.

## Known Limitations (Phase 3 todo)
- Not yet implemented: `for`, `do/while`, `break`/`continue`, `switch`.
- No pointers/arrays/structs/globals/strings yet (type system and memory ops pending).
- No function calls/recursion in IR yet; ABI groundwork exists but call lowering is not wired.
- Logical `&&`/`||` short-circuit and unary/bitwise/shift ops not implemented.
- SSA validator and more robust error diagnostics are future work.

## Next Steps
1. Control flow: add `for`, `do/while`, `break`, `continue`; then `switch`.
2. Types/memory: basic type system, `load`/`store`, address-of/deref, aggregates layout; globals/strings in `.data`/`.rodata`.
3. Calls: IR `call` + argument/result marshaling per SysV; recursion tests.
4. Expressions: logical ops with short-circuit; unary and bitwise/shift.
5. Optimizations: SCCP, DCE refinement, simple GVN, small peepholes.
6. Testing: flip COMPILE-FAIL tests to EXIT as features land; add phi/cfg-specific tests.

## How To Run
- End-to-end sanity:
  - `make e2e` — builds compiler, generates assembly for `examples/phase1/ret_expr.c`, links, runs (expects exit=14).
- Full test suite (executes each binary with a 1s timeout):
  - `make test`
- Sandboxed build/use of compiler:
  - `GOCACHE=$(pwd)/.cache/go-build GOMODCACHE=$(pwd)/.cache/gomod go build -o ccomp ./cmd/ccomp`
  - `./ccomp -o out.s tests/t13_compare.c && gcc -nostdlib out.s runtime/start_linux_amd64.s -o a.out && ./a.out; echo $?`

## Repository Hygiene
- Temporary artifacts are cleaned by `make clean` and ignored via `.gitignore` (`.cache/`, `ccomp`, `out.s`, `a.out`, `.test.*`, `.test-tmp/`, `.t*`, `.w*`).

---
This report reflects the repository state after Phase 2 completion and Phase 3 initial control-flow support (if/else, while with loop-carried phi) has been implemented and validated.
