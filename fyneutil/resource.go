package fyneutil

import (
	"unsafe"

	"fyne.io/fyne/v2"
)

type memoryResource struct {
	name string
	data string
}

func NewMemoryResource(name string, data []byte) fyne.Resource {
	return memoryResource{
		name: name,
		data: *(*string)(unsafe.Pointer(&data)),
	}
}

func (r memoryResource) Name() string {
	return r.name
}

func (r memoryResource) Content() []byte {
	return (*(*[]byte)(unsafe.Pointer(&r.data)))[:len(r.data):len(r.data)]
}
