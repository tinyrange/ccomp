package types

// Kind represents a minimal set of C-like types we care about now.
type Kind int

const (
    Int64 Kind = iota
    Ptr
    Byte
)

// Type is a minimal description of a value's type.
// For now we only track 64-bit integers and pointers to another Type.
type Type struct {
    K    Kind
    Elem *Type // non-nil only when K==Ptr
}

func Int() Type { return Type{K: Int64} }

func PointerTo(elem Type) Type { return Type{K: Ptr, Elem: &elem} }

// Size returns the size in bytes for this type on our target.
func (t Type) Size() int {
    switch t.K {
    case Int64:
        return 8
    case Ptr:
        // 64-bit pointers
        return 8
    case Byte:
        return 1
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
