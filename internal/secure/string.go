package secure

import (
	"crypto/rand"
	"runtime"
	"unsafe"
)

// String is a secure string type that zeroes memory on cleanup.
// Use this for sensitive data like passwords and API keys.
type String struct {
	data []byte
}

// NewString creates a new secure string from a regular string.
// The original string should be discarded after calling this.
func NewString(s string) *String {
	data := make([]byte, len(s))
	copy(data, s)
	ss := &String{data: data}
	runtime.SetFinalizer(ss, (*String).Destroy)
	return ss
}

// String returns the string value. Use sparingly and clear the result when done.
func (s *String) String() string {
	if s == nil || s.data == nil {
		return ""
	}
	return string(s.data)
}

// Bytes returns a copy of the underlying bytes.
func (s *String) Bytes() []byte {
	if s == nil || s.data == nil {
		return nil
	}
	result := make([]byte, len(s.data))
	copy(result, s.data)
	return result
}

// Destroy securely wipes the string from memory.
func (s *String) Destroy() {
	if s == nil || s.data == nil {
		return
	}
	// Overwrite with random data first, then zeros.
	rand.Read(s.data)
	for i := range s.data {
		s.data[i] = 0
	}
	s.data = nil
	runtime.SetFinalizer(s, nil)
}

// IsEmpty returns true if the string is empty or destroyed.
func (s *String) IsEmpty() bool {
	return s == nil || s.data == nil || len(s.data) == 0
}

// zeroBytes securely zeroes a byte slice.
func zeroBytes(b []byte) {
	if len(b) == 0 {
		return
	}
	// Use unsafe pointer to prevent compiler optimizations.
	ptr := unsafe.Pointer(&b[0])
	for i := range b {
		*(*byte)(unsafe.Pointer(uintptr(ptr) + uintptr(i))) = 0
	}
}
