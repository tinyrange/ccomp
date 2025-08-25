package x86_64

import (
    "sort"
    "github.com/tinyrange/cc/internal/ir"
)

// SSA-aware linear-scan register allocation for multi-block functions.
// Avoids using %rax so division and return can use it freely.

// Reserve %rcx for emitter scratch (loads/stores, shifts), so exclude it here.
// Call-preserved registers: %rbx, %r12-r15 (we don't use these for simplicity)
// Call-clobbered registers: %rdx, %r8-r11, %rsi, %rdi (we use these)
var allocableRegs = []string{"%rdx", "%r8", "%r9", "%r10", "%r11", "%rsi", "%rdi"}

// Call-clobbered registers that need to be saved/restored around calls
var callClobberedRegs = map[string]bool{
    "%rdx": true, "%r8": true, "%r9": true, "%r10": true, "%r11": true, 
    "%rsi": true, "%rdi": true,
}

type allocation struct {
    regOf map[ir.ValueID]string
}

type liveInterval struct {
    id    ir.ValueID
    start int
    end   int
    spill bool  // true if this interval should be spilled
}

func allocateRegisters(f *ir.Function) allocation {
    // Build a global instruction numbering across all blocks
    instrToNum := make(map[*ir.Instr]int)
    var allInstrs []*ir.Instr
    num := 0
    
    for _, b := range f.Blocks {
        for i := range b.Instrs {
            instrToNum[&b.Instrs[i]] = num
            allInstrs = append(allInstrs, &b.Instrs[i])
            num++
        }
    }
    
    if len(allInstrs) == 0 {
        return allocation{regOf: map[ir.ValueID]string{}}
    }

    // Find all calls for later clobber handling
    var callInstrNums []int
    for _, ins := range allInstrs {
        if ins.Val.Op == ir.OpCall {
            callInstrNums = append(callInstrNums, instrToNum[ins])
        }
    }

    // Compute live intervals using def-use analysis
    defAt := make(map[ir.ValueID]int)
    lastUseAt := make(map[ir.ValueID]int)
    
    for i, ins := range allInstrs {
        // Record definition
        if ins.Res >= 0 {
            if _, exists := defAt[ins.Res]; !exists {
                defAt[ins.Res] = i
                lastUseAt[ins.Res] = i // initialize with def point
            }
        }
        // Record uses
        for _, arg := range ins.Val.Args {
            lastUseAt[arg] = i
        }
    }

    // Build live intervals
    var intervals []liveInterval
    for id, def := range defAt {
        end, hasUse := lastUseAt[id]
        if !hasUse || end <= def {
            continue // Dead value, no uses
        }
        
        interval := liveInterval{
            id:    id,
            start: def,
            end:   end,
        }
        
        // Check if this interval spans any calls
        spansCall := false
        for _, callNum := range callInstrNums {
            if callNum > def && callNum < end {
                spansCall = true
                break
            }
        }
        
        // If interval spans a call, it needs to be spilled (for simplicity)
        // A more sophisticated approach would save/restore around calls
        if spansCall {
            interval.spill = true
        }
        
        intervals = append(intervals, interval)
    }

    // Sort intervals by start position
    sort.Slice(intervals, func(i, j int) bool {
        return intervals[i].start < intervals[j].start
    })

    // Linear scan allocation
    type activeInterval struct {
        interval liveInterval
        reg      string
    }
    
    var active []activeInterval
    alloc := allocation{regOf: make(map[ir.ValueID]string)}
    
    expireOldIntervals := func(position int) {
        // Remove intervals that have ended
        newActive := active[:0]
        for _, a := range active {
            if a.interval.end >= position {
                newActive = append(newActive, a)
            }
        }
        active = newActive
    }
    
    findFreeRegister := func() (string, bool) {
        usedRegs := make(map[string]bool)
        for _, a := range active {
            usedRegs[a.reg] = true
        }
        
        for _, reg := range allocableRegs {
            if !usedRegs[reg] {
                return reg, true
            }
        }
        return "", false
    }
    
    spillCandidate := func() *activeInterval {
        // Simple spill heuristic: spill the interval that ends last
        if len(active) == 0 {
            return nil
        }
        
        maxEnd := -1
        var candidate *activeInterval
        for i := range active {
            if active[i].interval.end > maxEnd {
                maxEnd = active[i].interval.end
                candidate = &active[i]
            }
        }
        return candidate
    }

    // Process each interval
    for _, current := range intervals {
        expireOldIntervals(current.start)
        
        // Force spill if interval spans calls
        if current.spill {
            // Leave unassigned (will spill to stack)
            continue
        }
        
        if reg, available := findFreeRegister(); available {
            // Assign free register
            alloc.regOf[current.id] = reg
            active = append(active, activeInterval{
                interval: current,
                reg:     reg,
            })
        } else if len(active) > 0 {
            // Try to spill an existing interval
            if candidate := spillCandidate(); candidate != nil && candidate.interval.end > current.end {
                // Spill the candidate and assign its register to current
                delete(alloc.regOf, candidate.interval.id)
                alloc.regOf[current.id] = candidate.reg
                
                // Remove candidate from active
                newActive := active[:0]
                for _, a := range active {
                    if a.interval.id != candidate.interval.id {
                        newActive = append(newActive, a)
                    }
                }
                active = newActive
                
                // Add current to active
                active = append(active, activeInterval{
                    interval: current,
                    reg:     candidate.reg,
                })
            }
            // If we can't find a good spill candidate, leave current unassigned (spilled)
        }
    }
    
    return alloc
}

func max(a, b int) int {
    if a > b { return a }
    return b
}
