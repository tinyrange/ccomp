package types

// Kind represents a minimal set of C-like types we care about now.
type Kind int

const (
    Int8 Kind = iota
    Int16
    Int32
    Int64
    Uint8
    Uint16
    Uint32
    Uint64
    Float32
    Float64
    Ptr
    Byte // alias for Uint8
)

// Type is a minimal description of a value's type.
// For now we only track 64-bit integers and pointers to another Type.
type Type struct {
    K    Kind
    Elem *Type // non-nil only when K==Ptr
}

func Int() Type { return Type{K: Int64} }
func Int8T() Type { return Type{K: Int8} }
func Int16T() Type { return Type{K: Int16} }
func Int32T() Type { return Type{K: Int32} }
func Uint8T() Type { return Type{K: Uint8} }
func Uint16T() Type { return Type{K: Uint16} }
func Uint32T() Type { return Type{K: Uint32} }
func Uint64T() Type { return Type{K: Uint64} }

func PointerTo(elem Type) Type { return Type{K: Ptr, Elem: &elem} }

// Size returns the size in bytes for this type on our target.
func (t Type) Size() int {
    switch t.K {
    case Int8, Uint8, Byte:
        return 1
    case Int16, Uint16:
        return 2
    case Int32, Uint32, Float32:
        return 4
    case Int64, Uint64, Float64:
        return 8
    case Ptr:
        // 64-bit pointers
        return 8
    default:
        return 8
    }
}

// ElemSize returns the pointee size if pointer, else 0.
func (t Type) ElemSize() int {
    if t.K == Ptr && t.Elem != nil {
        return t.Elem.Size()
    }
    return 0
}

func (t Type) IsPointer() bool { return t.K == Ptr }

func ByteT() Type { return Type{K: Byte} }
func CharT() Type { return Type{K: Byte} } // char is unsigned byte by default
func Float64T() Type { return Type{K: Float64} }
func DoubleT() Type { return Type{K: Float64} } // double is 64-bit float

// IsSigned returns true for signed integer types
func (t Type) IsSigned() bool {
    switch t.K {
    case Int8, Int16, Int32, Int64:
        return true
    default:
        return false
    }
}

// IsUnsigned returns true for unsigned integer types
func (t Type) IsUnsigned() bool {
    switch t.K {
    case Uint8, Uint16, Uint32, Uint64, Byte:
        return true
    default:
        return false
    }
}

// IsInteger returns true for any integer type
func (t Type) IsInteger() bool {
    return t.IsSigned() || t.IsUnsigned()
}

// IsFloat returns true for floating point types
func (t Type) IsFloat() bool {
    return t.K == Float32 || t.K == Float64
}

// StructType represents a struct type (forward declaration to avoid import cycle)
type StructType struct {
    Name string
    Def  interface{} // will be *ir.StructDef but we avoid the import cycle
}

// FromBasicType converts AST BasicType to internal Type
// Note: This will need ast import - will be updated in ir.go where it's used
func FromBasicType(bt int, isPtr bool) Type {
    // bt values: 0=BTInt, 1=BTChar, 2=BTDouble (from ast.go BasicType constants)
    if isPtr {
        switch bt {
        case 1: // BTChar
            return PointerTo(CharT())
        case 2: // BTDouble
            return PointerTo(DoubleT())
        default: // BTInt
            return PointerTo(Int())
        }
    } else {
        switch bt {
        case 1: // BTChar
            return CharT()
        case 2: // BTDouble
            return DoubleT()
        default: // BTInt
            return Int()
        }
    }
}
