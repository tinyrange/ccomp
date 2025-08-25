# Compiler Progress Report — Phase 3

## Overview

This report summarizes the current state of the SSA-based C compiler. We have a working end-to-end pipeline with SSA construction, optimizations, phi elimination, and an x86_64 SysV backend. Phase 3 substantially expanded control flow, function calls, logic/bitwise/shift operators, pointers, minimal arrays, and globals. All tests currently pass.

## Implemented

- Frontend
  - Lexer: keywords `int char struct enum typedef return if else while for do break continue switch case default`, punctuation `(){}[],:;.`, operators `= + - * / < <= > >= == != && || & | ^ ~ << >> !`.
  - Parser: functions with `int` params; blocks; decls/assignments; `return`; control-flow `if/else`, `while`, `for`, `do/while`, `break`, `continue`, `switch/case/default`; expressions with precedence including logical short-circuit, bitwise, and shifts; calls `f(a,b)`; unary `-`, `~`, `!`, address-of `&`, deref `*`; minimal arrays `int a[N]; a[i]; a[i]=...`; struct definitions `struct S { int x; int y; }`, field access `s.field`, field assignment `s.field = value`; enum definitions `enum E { A=1, B=2 }`; typedef declarations `typedef int i32`.
- IR (SSA)
  - Values/ops: arithmetic `add sub mul div`; compare `eq ne lt le gt ge`; logic/bitwise/shift `and or xor shl shr not logicalnot`; memory `load store`; control-flow `phi jmp jnz`; calls `call`; addressing `addr globaladdr slotaddr`; misc `const param copy`.
  - CFG on basic blocks: `Preds`/`Succs` with helper `addEdge`.
- SSA construction
  - Direct SSA during AST traversal (Braun-style read/write per block).
  - Unsealed-block handling with placeholder `phi` and sealing to fill operands; backedges supported for loops.
- SSA destruction
  - Phi elimination with critical-edge splitting (also rewrites predecessor terminators) and parallel copies on incoming edges.
- Optimizations (Phase 2)
  - Constant folding/propagation (arith + bitwise + shifts where both operands constant).
  - Dead code elimination (keeps params, calls, and stores; no-side-effect values removed).
  - SSA-aware linear-scan register allocation across CFG with proper call clobber handling; spills values that span calls.
  - Peephole: immediates for `add/sub/imul` where applicable.
- Backend (x86_64, SysV AMD64)
  - Prologue/epilogue; 8-byte-per-SSA slot stack frame; params from arg regs to SSA homes.
  - Arithmetic; division via `%rax/%rdx`; comparisons via `cmp`+`setcc`+`movzx`; bitwise `and/or/xor`; shifts `shl/sar` (count in imm or `%cl`); copies; `jmp/jne`.
  - Calls: marshal up to 6 integer args to `%rdi,%rsi,%rdx,%rcx,%r8,%r9`; maintain 16-byte alignment by `sub/add $8`; return in `%rax`.
  - Addressing: `lea slot(%rbp)` for locals; RIP-relative `lea sym(%rip)` for globals.
- CLI/Build
  - `ccomp` with `-o` anywhere in argv.
  - Sandboxed builds using local Go caches; `Makefile` targets `build`, `run`, `e2e`, `clean`, `test`.
  - Runtime `_start` for `-nostdlib` linking.
- Tests
  - 45 tests in `tests/` with expectations: `// EXPECT: EXIT <n>` or `// EXPECT: COMPILE-FAIL`.
  - Runner `tools/run_tests.sh` compiles, links, runs, and checks results using a 1s timeout wrapper to avoid hangs. `make test` wraps it.
  - Recent test additions: logical NOT operator (`!`) validation, struct/enum/typedef functionality, floating point literal casting.

## Recently Completed (Phase 3 Extensions)

- Enhanced type system: extended beyond int/pointer with signed/unsigned variants (Int8, Int16, Int32, Int64, Uint8, Uint16, Uint32, Uint64) and proper size calculations.
- Pointer arithmetic: `ptr +/- int` scales by pointee size; `ptr - ptr` returns element count difference (C-compliant semantics).
- Global arrays: parse/emit `int g[N];` as zero-initialized `.data` with `.zero N*elemsize`; support `g[i]` loads/stores with proper element scaling.
- String literals: lex/parse `"..."`, intern in module `.rodata` as NUL-terminated with unique labels; expressions of type `char*` yield address via RIP-relative `lea`.
- Struct definitions: complete parsing and IR layout calculation with field offset computation.
- Enum constants: full implementation with module-level storage and identifier resolution (e.g., `enum E { A=1, B=2 }; return B;` works).
- Typedef declarations: parsing implemented (type aliases not yet functional).
- Register allocation: upgraded from single-block-only to full SSA-aware linear scan supporting multi-block functions and proper call clobber handling.
- Expression system: added logical NOT operator (`!`) with proper code generation.
- Floating point literals: added lexer, parser, and AST support for floating point literals with compile-time evaluation (enables `(int)3.5` casting).
- Floating point arithmetic: implemented constant folding optimization for `OpFAdd`, `OpFSub`, `OpFMul`, `OpFDiv` operations.
- Float-to-int casting: added `OpF2I` conversion with compile-time constant folding support.

## What Works End-to-End

- Expressions: integer arithmetic; comparisons; logical short-circuit `&&/||` and unary `!`; bitwise `& | ^` and unary `~`; shifts `<< >>`; floating point literals and arithmetic with compile-time constant folding; float-to-int casting; parentheses respected.
- Declarations/assignments: local `int`/`char` variables; minimal arrays `int a[N]` with `a[i]` r/w backed by frame slots; pointers `&x`, `*p` with proper element-size scaling.
- Control flow: `if/else`, `while`, `for`, `do/while`, `break`, `continue`, and `switch/case/default` (fallthrough by omission) with correct CFG/phi.
- Calls/recursion: direct calls with SysV arg passing; recursion works (factorial test returns 120).
- Globals: `int g = <int>` and `char gc = <int>` in `.data`, accessed via RIP-relative addressing; global arrays `int ga[N]`.
- Structs: `struct S { int x; int y; };` definitions with field layout; `struct S s;` variable declarations; `s.field` access and `s.field = value` assignments.
- Enums: `enum E { A=1, B=2 };` definitions with constants that resolve correctly (returns proper values).
- Typedefs: `typedef int i32; i32 x = 42;` type alias definitions and usage in variable declarations.
- Sanity: `make e2e` returns exit code 14 for the sample; full suite: all 45 tests passing.

## Known Limitations (remaining work)

- Type system: enhanced types exist but most operations still default to 64-bit int behavior; no signed/unsigned distinction in operations.
- Memory model: no alias analysis; struct memory layout calculated and used for field access.
- Floating point: runtime floating point operations with variables not supported (only compile-time constant expressions).
- No union; no varargs.
- Diagnostics: parser/IR errors are minimal; no SSA validator.

## Next Steps

1. **Expressions**: ✓ logical `!` implemented; ✓ floating point literals and casting; ✓ float-to-int casts with compile-time constant folding; casts work for basic types; refine comparisons for signed/unsigned as types solidify.
2. **Register allocation**: ✓ implemented SSA-aware linear scan across CFG with call clobber handling.
3. **Optimizations**: SCCP, simple GVN, and peepholes for address arithmetic and copy cleanup.
4. **Tooling**: SSA validator and improved diagnostics.

## How To Run

- End-to-end sanity:
  - `make e2e` — builds compiler, generates assembly for `examples/phase1/ret_expr.c`, links, runs (expects exit=14).
- Full test suite (1s timeout per binary): `make test`
- Sandboxed build/use of compiler:
  - `GOCACHE=$(pwd)/.cache/go-build GOMODCACHE=$(pwd)/.cache/gomod go build -o ccomp ./cmd/ccomp`
  - `./ccomp -o out.s tests/t13_compare.c && gcc -nostdlib out.s runtime/start_linux_amd64.s -o a.out && ./a.out; echo $?`

## Repository Hygiene

- Temporary artifacts are cleaned by `make clean` and ignored via `.gitignore` (`.cache/`, `ccomp`, `out.s`, `a.out`, `.test.*`, `.test-tmp/`, `.t*`, `.w*`).

---

This report reflects the repository state after Phase 2 completion and Phase 3 initial control-flow support (if/else, while with loop-carried phi) has been implemented and validated.
