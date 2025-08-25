# Building a practical SSA-based C compiler specification

This comprehensive specification provides step-by-step guidance for implementing a basic C compiler with Static Single Assignment (SSA) form, architecture abstraction, and frontend flexibility. Based on analysis of successful minimal compilers like QBE, educational implementations, and proven algorithms from compiler research, this document offers a practical roadmap for building an extensible, working compiler.

## Core architecture overview

The compiler follows a three-stage pipeline with clean abstraction boundaries: **Frontend** (lexing, parsing, semantic analysis) → **SSA-based IR** (construction, optimization, register allocation) → **Backend** (instruction selection, assembly generation). This M+N approach enables multiple source languages and target architectures while maintaining a single optimization infrastructure.

The intermediate representation uses SSA form throughout compilation, following QBE's philosophy of "70% of LLVM's performance in 10% of the code." This approach prioritizes simplicity and maintainability while achieving production-quality code generation.

## SSA construction: Braun's simplified algorithm

For basic compilers, **Braun et al.'s simplified SSA construction algorithm** provides the optimal balance of simplicity and effectiveness. Unlike Cytron's algorithm, it requires no dominance frontier calculation and constructs SSA directly during AST traversal.

**Implementation strategy:** The algorithm maintains current variable definitions per basic block and creates phi nodes on-demand. When reading a variable in a block without a definition, the algorithm recursively searches predecessors and automatically inserts phi nodes at join points. This approach handles unsealed blocks gracefully, making it ideal for single-pass compilation.

```pseudocode
class BasicBlock:
    sealed: bool = False
    currentDef: Map[Variable, Value] = {}

def writeVariable(var: Variable, block: BasicBlock, value: Value):
    block.currentDef[var] = value

def readVariable(var: Variable, block: BasicBlock) -> Value:
    if var in block.currentDef:
        return block.currentDef[var]
    if len(block.predecessors) == 1:
        return readVariable(var, block.predecessors[0])
    # Create phi node for multiple predecessors
    phi = createPhi(var, block)
    writeVariable(var, block, phi)
    if block.sealed:
        addPhiOperands(phi)
    return phi
```

For compilers requiring maximum optimization opportunities, implement **semi-pruned SSA** using Cytron's algorithm with global variable filtering. This reduces phi functions by approximately 55% compared to minimal SSA while maintaining most optimization opportunities.

## Architecture abstraction layer design

The compiler implements a **QBE-inspired minimal abstraction** that separates target-independent optimization from machine-specific code generation through well-defined interfaces.

**Target description structure:**

```c
typedef struct {
    const char *name;
    int word_size;      // 32 or 64
    int num_gp_regs;    // General purpose registers
    int num_fp_regs;    // Floating point registers

    // Core interfaces
    void (*select_instructions)(Function *f);
    void (*allocate_registers)(Function *f);
    void (*emit_assembly)(Function *f, FILE *out);

    // ABI specification
    CallingConvention *calling_convention;
} TargetDesc;
```

Unlike LLVM's complex TableGen system, this approach hardcodes target knowledge in C, dramatically simplifying implementation while maintaining good code quality. Each target backend provides approximately 2000-3000 lines of code handling instruction selection, register allocation, and assembly emission.

**Calling convention abstraction:**

```c
typedef struct {
    int param_regs[6];       // First 6 parameters
    int return_reg;          // Return value register
    int callee_saved[8];     // Preserved registers
    bool stack_grows_down;
    int stack_alignment;
} CallingConvention;
```

## Frontend abstraction for multi-language support

The frontend abstraction enables multiple source languages through a **common AST interface** that maps language-specific constructs to a unified representation.

**Language-independent AST nodes:**

```c
typedef enum {
    AST_FUNCTION, AST_BLOCK, AST_IF, AST_WHILE,
    AST_ASSIGN, AST_BINARY_OP, AST_UNARY_OP,
    AST_CALL, AST_RETURN, AST_VAR_DECL
} ASTNodeType;

typedef struct ASTNode {
    ASTNodeType type;
    Type *dataType;          // Common type representation
    SourceLocation loc;      // For error reporting
    union {
        // Node-specific data
    } data;
} ASTNode;
```

Each language frontend implements a parser that generates this common AST, then a **semantic analysis pass** performs language-specific type checking before IR generation. This separation allows sharing the entire optimization pipeline across languages.

## Practical SSA-based IR design

The IR uses a **minimal instruction set** of 32 core operations sufficient for complete language implementation:

**Type system (following QBE's simplicity):**

- `w` (word): 32-bit integers
- `l` (long): 64-bit integers
- `s` (single): 32-bit floats
- `d` (double): 64-bit floats
- Pointers represented as integers of appropriate width
- Aggregates handled through memory operations only

**Essential instruction set:**

```
# Arithmetic (14 ops)
add, sub, mul, div, udiv, rem, urem, neg
and, or, xor, shl, shr, sar

# Memory (8 ops)
load, store, alloc
loadb, loadh, loadw, loadl
storeb, storeh, storew, storel

# Control flow (5 ops)
jmp, jnz, call, ret, phi

# Comparison (10 ops)
ceq, cne, cslt, csle, csgt, csge
cult, cule, cugt, cuge

# Type conversion (4 ops)
sext, zext, trunc, bitcast
```

**SSA value representation:**

```c
typedef struct SSAValue {
    unsigned id;              // Unique identifier
    IRType type;             // w, l, s, or d
    Instruction *def;        // Defining instruction
    List *uses;              // Use sites
} SSAValue;
```

**Memory operations in SSA:** Memory state passes as an explicit value through load/store operations, ensuring proper ordering and enabling memory optimization. This approach, used by Go's SSA backend, prevents subtle bugs from reordering memory operations.

## Code generation from SSA to x86_64

### Instruction selection via simple pattern matching

The compiler uses **direct pattern matching** without complex tree rewriting. Each SSA instruction maps to one or more x86_64 instructions through straightforward rules:

```c
void select_instruction(SSAInstruction *inst) {
    switch (inst->opcode) {
        case IR_ADD:
            if (is_immediate(inst->arg2))
                emit_add_imm(inst->dest, inst->arg1, inst->arg2);
            else
                emit_add_reg(inst->dest, inst->arg1, inst->arg2);
            break;
        case IR_LOAD:
            emit_mov_from_mem(inst->dest, inst->address);
            break;
        // ... pattern for each IR operation
    }
}
```

For better code quality, implement **simple peephole optimizations**: combining address calculations into addressing modes, using LEA for arithmetic, eliminating redundant moves, and selecting shorter instruction encodings.

### Linear scan register allocation

**Wimmer & Franz's linear scan algorithm** optimized for SSA provides excellent results with O(n log n) complexity:

```c
typedef struct LiveInterval {
    SSAValue *value;
    unsigned start, end;
    int assigned_reg;        // -1 if spilled
} LiveInterval;

void linear_scan_allocation(Function *func) {
    List *intervals = compute_live_intervals(func);
    sort_by_start_point(intervals);

    List *active = list_new();
    for (LiveInterval *current : intervals) {
        expire_old_intervals(active, current->start);

        if (list_length(active) == NUM_REGISTERS) {
            LiveInterval *spill = find_spill_candidate(active);
            spill->assigned_reg = SPILLED;
            insert_spill_code(spill);
        }

        current->assigned_reg = allocate_free_register(active);
        list_add(active, current);
    }
}
```

### Phi elimination and SSA destruction

Implement **parallel copy sequentialization** for phi nodes using critical edge splitting:

1. Identify critical edges (predecessor with multiple successors AND successor with multiple predecessors)
2. Insert new basic blocks on critical edges
3. Place copy instructions to resolve phi nodes
4. Handle circular dependencies with temporary variables

## Step-by-step implementation roadmap

### Phase 1: Minimal working compiler

1. **Basic lexer/parser** for subset of C (functions, integers, arithmetic)
2. **Direct SSA construction** using Braun's algorithm
3. **Simple code generation** without register allocation (stack machine)
4. **System V AMD64 ABI** implementation for function calls
5. **Test framework** with 20-30 basic programs

### Phase 2: SSA optimizations

1. **Constant propagation** and folding
2. **Dead code elimination** using SSA use-def chains
3. **Linear scan register allocation**
4. **Phi elimination** with critical edge splitting
5. **Peephole optimizations** for common patterns

### Phase 3: Full C support

1. **Complete type system** (structs, arrays, pointers)
2. **Control flow** (if/else, loops, switch)
3. **Global variables** and string literals
4. **Standard library** integration
5. **Comprehensive test suite** (100+ programs)

### Phase 4: Architecture abstraction

1. **Abstract backend interface**
2. **Second target** (ARM64 or RISC-V)
3. **Target-specific optimizations**
4. **Cross-compilation testing**

### Phase 5: Frontend abstraction

1. **Common AST interface**
2. **Second language frontend** (simplified Go or Rust subset)
3. **Language-specific semantic analysis**
4. **Mixed-language testing**

## Testing strategies and validation

### Differential testing with Csmith

Use **Csmith** to generate random C programs and compare output against GCC/Clang:

```bash
# Generate test program
csmith --no-unions --no-volatiles > test.c

# Compile with reference compiler
gcc -O0 test.c -o reference
./reference > expected.txt

# Compile with your compiler
./mycc test.c -o generated
./generated > actual.txt

# Compare outputs
diff expected.txt actual.txt
```

### SSA validation checks

Implement verification passes after each transformation:

```c
bool verify_ssa_properties(Function *func) {
    // Each variable defined exactly once
    Set *defined = set_new();
    for (BasicBlock *block : func->blocks) {
        for (Instruction *inst : block->instructions) {
            if (inst->dest && set_contains(defined, inst->dest))
                return false;  // Multiple definitions
            set_add(defined, inst->dest);
        }
    }

    // All uses dominated by definition
    for (BasicBlock *block : func->blocks) {
        for (Instruction *inst : block->instructions) {
            for (SSAValue *use : inst->operands) {
                if (!dominates(use->def->block, block))
                    return false;  // Use before def
            }
        }
    }
    return true;
}
```

### Regression test suite design

Build a hierarchical test suite covering:

1. **Syntax tests** - Parser accepts/rejects correctly
2. **Semantic tests** - Type checking and error reporting
3. **Codegen tests** - Correct assembly generation
4. **Optimization tests** - Transformations preserve semantics
5. **ABI tests** - External function calls work correctly

## Reference implementations to study

**QBE** provides the best reference for a production-quality minimal backend. Study its uniform SSA representation, simple type system, and efficient register allocation. The entire compiler is under 10,000 lines of readable C code.

**Educational implementations** like MicroC demonstrate complete compiler structure in manageable codebases. The OCaml-based MicroC compiler (624 lines) shows clean separation between frontend, semantic analysis, and code generation.

**Csmith and CompCert** offer insights into testing and verification. Csmith's differential testing approach has found hundreds of bugs in production compilers, while CompCert demonstrates formal verification techniques applicable to critical passes.

## Common pitfalls and solutions

**SSA construction issues:** The most common error is incorrect phi node placement. Use dominance frontier calculation carefully or adopt Braun's algorithm which handles this automatically.

**Register allocation bugs:** Spilling can introduce subtle errors. Always verify that spilled values are correctly restored before use and that the stack frame layout respects alignment requirements.

**ABI compliance problems:** Calling convention violations cause mysterious crashes. Carefully implement the System V AMD64 ABI, especially stack alignment (16-byte before CALL) and register preservation rules.

**Memory ordering issues:** Without explicit memory dependencies, loads and stores may be incorrectly reordered. Pass memory state as a value through SSA to maintain proper ordering.

## Optimization opportunities

Once the basic compiler works, consider these enhancements:

- **Sparse conditional constant propagation** leveraging SSA form
- **Global value numbering** for common subexpression elimination
- **Loop-invariant code motion** using dominance information
- **Escape analysis** for stack allocation of heap objects
- **Inlining** with cost models based on IR instruction counts

## Conclusion

This specification provides a complete roadmap for building a working SSA-based C compiler that can be extended to support multiple languages and architectures. By following the incremental implementation plan and leveraging proven algorithms and design patterns from successful compilers, you can create a maintainable, efficient compiler in approximately 5,000-10,000 lines of code.

The key insight is that compiler construction doesn't require overwhelming complexity. By choosing simple, proven algorithms (Braun's SSA construction, linear scan allocation, direct instruction selection) and maintaining clean abstractions, you can build a compiler that generates reasonable code while remaining comprehensible and extensible. Start with the minimal working compiler, validate each phase thoroughly, and gradually add sophistication as needed.
