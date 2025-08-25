package x86_64

import "github.com/tinyrange/cc/internal/ir"

// Very simple linear-scan register allocation for single-block functions.
// Avoids using %rax so division and return can use it freely.

var allocableRegs = []string{"%rcx", "%rdx", "%r8", "%r9", "%r10", "%r11", "%rsi", "%rdi"}

type allocation struct {
    regOf map[ir.ValueID]string
}

func allocateRegisters(f *ir.Function) allocation {
    // Conservative: if function has multiple basic blocks, skip register allocation
    // to avoid incorrect live range analysis across control flow for now.
    if len(f.Blocks) > 1 {
        return allocation{regOf: map[ir.ValueID]string{}}
    }
    // Flatten instructions
    var instrs []ir.Instr
    for _, b := range f.Blocks { instrs = append(instrs, b.Instrs...) }
    n := len(instrs)

    type interval struct{ id ir.ValueID; start, end int }
    useCount := map[ir.ValueID]int{}
    defAt := map[ir.ValueID]int{}
    endAt := map[ir.ValueID]int{}

    for i, ins := range instrs {
        if ins.Res >= 0 {
            if _, ok := defAt[ins.Res]; !ok { defAt[ins.Res] = i; endAt[ins.Res] = i }
        }
        for _, a := range ins.Val.Args {
            useCount[a]++
            if i > endAt[a] { endAt[a] = i }
        }
    }

    var intervals []interval
    for id, s := range defAt {
        e := endAt[id]
        // Ignore values never used; no need to allocate register
        if useCount[id] == 0 { continue }
        intervals = append(intervals, interval{id: id, start: s, end: e})
    }

    // Sort by start (simple insertion sort for small N)
    for i := 1; i < len(intervals); i++ {
        j := i
        for j > 0 && intervals[j-1].start > intervals[j].start {
            intervals[j-1], intervals[j] = intervals[j], intervals[j-1]
            j--
        }
    }

    type activeI struct{ iv interval; reg string }
    var active []activeI
    alloc := allocation{regOf: map[ir.ValueID]string{}}

    expire := func(pos int) {
        out := active[:0]
        for _, a := range active {
            if a.iv.end >= pos { out = append(out, a) }
        }
        active = out
    }

    takeFreeReg := func() (string, bool) {
        used := map[string]bool{}
        for _, a := range active { used[a.reg] = true }
        for _, r := range allocableRegs { if !used[r] { return r, true } }
        return "", false
    }

    for _, iv := range intervals {
        expire(iv.start)
        if r, ok := takeFreeReg(); ok {
            alloc.regOf[iv.id] = r
            active = append(active, activeI{iv: iv, reg: r})
        } else {
            // spill: do nothing (remain on stack)
        }
    }
    _ = n
    return alloc
}
