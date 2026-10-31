package runtime

import (
	"bufio"
	"encoding/binary"
	"io"
	"reflect"
	"unsafe"
)

// File is a Pascal file of T. The buffer variable is Buf.
// A defined Pascal file type embeds File so each file type stays distinct.
// Components are stored in a disk file. Reads and writes are buffered.
//
// The encoding is local to this package. encoding/gob cannot store the
// unexported fields of a Pascal record, whose names stay lower case.
type File[T any] struct {
	Buf   T
	name  string
	store *store
	br    *bufio.Reader
	eof   bool
}

// Bind records the external name of a program parameter.
func (f *File[T]) Bind(name string) { f.name = name }

// Rewrite empties the file and leaves eof true.
func (f *File[T]) Rewrite() {
	f.open(true)
	f.br = nil
	f.eof = true
}

// Reset positions the file at its first component and loads the buffer.
func (f *File[T]) Reset() {
	f.br = nil
	if f.store != nil && f.store.bw != nil {
		f.store.bw.Flush()
		f.store.bw = nil
	}
	if f.store == nil {
		if f.name == "" {
			f.eof = true
			return
		}
		f.open(false)
		if f.store.bw != nil {
			f.store.bw.Flush()
			f.store.bw = nil
		}
	}
	if _, err := f.store.f.Seek(0, io.SeekStart); err != nil {
		panic(err)
	}
	f.br = bufio.NewReader(f.store.f)
	f.load()
}

// Get advances to the next component.
func (f *File[T]) Get() {
	if f.eof {
		panic("get past eof")
	}
	if f.br == nil {
		panic("get past eof")
	}
	f.load()
}

func (f *File[T]) load() {
	var v T
	err := readVal(f.br, reflect.ValueOf(&v).Elem())
	if err == io.EOF {
		f.eof = true
		return
	}
	if err != nil {
		panic(err)
	}
	f.Buf = v
	f.eof = false
}

// Put appends the buffer. eof must be true.
func (f *File[T]) Put() {
	if !f.eof {
		panic("put when not eof")
	}
	if f.store == nil || f.store.bw == nil {
		f.Rewrite()
	}
	if err := writeVal(f.store.bw, reflect.ValueOf(&f.Buf).Elem()); err != nil {
		panic(err)
	}
	f.eof = true
}

func (f *File[T]) open(trunc bool) {
	f.store = openStore(f.store, f.name, trunc, "")
}

// Eof reports whether the file is positioned at the end.
func (f *File[T]) Eof() bool { return f.eof }

// Read returns the current component and advances.
func (f *File[T]) Read() T {
	v := f.Buf
	f.Get()
	return v
}

// Write assigns v to the buffer and appends it.
func (f *File[T]) Write(v T) {
	f.Buf = v
	f.Put()
}

// structField is a settable view of field i. reflect refuses to set an
// unexported field even from the package that declared it, so the field
// is addressed by its offset instead.
func structField(v reflect.Value, i int) reflect.Value {
	f := v.Type().Field(i)
	p := unsafe.Add(unsafe.Pointer(v.UnsafeAddr()), f.Offset)
	return reflect.NewAt(f.Type, p).Elem()
}

func writeVal(w io.Writer, v reflect.Value) error {
	switch v.Kind() {
	case reflect.Bool:
		b := byte(0)
		if v.Bool() {
			b = 1
		}
		_, err := w.Write([]byte{b})
		return err
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return binary.Write(w, binary.LittleEndian, v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return binary.Write(w, binary.LittleEndian, v.Uint())
	case reflect.Float32, reflect.Float64:
		return binary.Write(w, binary.LittleEndian, v.Float())
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if err := writeVal(w, v.Index(i)); err != nil {
				return err
			}
		}
		return nil
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if err := writeVal(w, structField(v, i)); err != nil {
				return err
			}
		}
		return nil
	case reflect.Pointer:
		// A pointer component is the identifying value, not a copy of the
		// variable it denotes. The same address is restored on read.
		var addr uint64
		if !v.IsNil() {
			addr = uint64(v.Pointer())
		}
		return binary.Write(w, binary.LittleEndian, addr)
	default:
		panic("cannot store " + v.Kind().String())
	}
}

func readVal(r io.Reader, v reflect.Value) error {
	switch v.Kind() {
	case reflect.Bool:
		var b [1]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return err
		}
		v.SetBool(b[0] != 0)
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var n int64
		if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
			return err
		}
		v.SetInt(n)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		var n uint64
		if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
			return err
		}
		v.SetUint(n)
		return nil
	case reflect.Float32, reflect.Float64:
		var n float64
		if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
			return err
		}
		v.SetFloat(n)
		return nil
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if err := readVal(r, v.Index(i)); err != nil {
				return err
			}
		}
		return nil
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if err := readVal(r, structField(v, i)); err != nil {
				return err
			}
		}
		return nil
	case reflect.Pointer:
		var addr uint64
		if err := binary.Read(r, binary.LittleEndian, &addr); err != nil {
			return err
		}
		if addr == 0 {
			v.SetZero()
			return nil
		}
		// The address was stored as an integer. Copy it into a pointer
		// word; a uintptr-to-Pointer conversion is not a valid restore.
		var p unsafe.Pointer
		*(*uintptr)(unsafe.Pointer(&p)) = uintptr(addr)
		v.Set(reflect.NewAt(v.Type().Elem(), p))
		return nil
	default:
		panic("cannot store " + v.Kind().String())
	}
}
