package runtime

import "unsafe"

// Elem returns the element i of the array that starts at base.
// i is a zero-based offset. Conformant arrays are addressed this way
// because their length is not a Go array type.
func Elem[T any](base *T, i int) *T {
	if base == nil {
		panic("nil array")
	}
	return (*T)(unsafe.Add(unsafe.Pointer(base), i*int(unsafe.Sizeof(*base))))
}

// Move copies n elements from src to dst.
func Move[T any](src, dst *T, n int) {
	if n <= 0 || src == nil || dst == nil {
		return
	}
	copy(unsafe.Slice(dst, n), unsafe.Slice(src, n))
}
