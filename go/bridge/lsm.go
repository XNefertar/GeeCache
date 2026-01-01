package bridge

// #cgo CFLAGS: -I../../cpp/lsm/include
// #cgo LDFLAGS: -L../../cpp/lsm/build -llsm -lstdc++
// #include "lsm.h"
// #include <stdlib.h>
import "C"

import (
	"errors"
	"geecache"
	"unsafe"
)

// LSMStore 实现了 geecache.CentralCache 接口
type LSMStore struct {
	db *C.lsm_db_t
}

func NewLSMStore(path string) (*LSMStore, error) {
	opts := C.lsm_options_create()
	defer C.lsm_options_destroy(opts)
	C.lsm_options_set_create_if_missing(opts, 1)

	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	var cErr *C.char
	db := C.lsm_db_open(opts, cPath, &cErr)

	if cErr != nil {
		defer C.lsm_free(unsafe.Pointer(cErr))
		return nil, errors.New(C.GoString(cErr))
	}

	return &LSMStore{db: db}, nil
}

func (s *LSMStore) Get(key string) ([]byte, error) {
	cKey := C.CBytes([]byte(key))
	defer C.free(unsafe.Pointer(cKey))

	var cErr *C.char
	var cValueLen C.size_t

	cValue := C.lsm_get(
		s.db,
		(*C.char)(cKey), C.size_t(len(key)),
		&cValueLen,
		&cErr,
	)

	if cErr != nil {
		defer C.lsm_free(unsafe.Pointer(cErr))
		return nil, errors.New(C.GoString(cErr))
	}
	if cValue == nil {
		return nil, nil // Not found
	}

	defer C.lsm_free(unsafe.Pointer(cValue))
	return C.GoBytes(unsafe.Pointer(cValue), C.int(cValueLen)), nil
}

func (s *LSMStore) Set(key string, value []byte) error {
	cKey := C.CBytes([]byte(key))
	defer C.free(unsafe.Pointer(cKey))
	cValue := C.CBytes(value)
	defer C.free(unsafe.Pointer(cValue))

	var cErr *C.char
	C.lsm_put(
		s.db,
		(*C.char)(cKey), C.size_t(len(key)),
		(*C.char)(cValue), C.size_t(len(value)),
		&cErr,
	)

	if cErr != nil {
		defer C.lsm_free(unsafe.Pointer(cErr))
		return errors.New(C.GoString(cErr))
	}
	return nil
}

func (s *LSMStore) Delete(key string) error {
	cKey := C.CBytes([]byte(key))
	defer C.free(unsafe.Pointer(cKey))

	var cErr *C.char
	C.lsm_delete(s.db, (*C.char)(cKey), C.size_t(len(key)), &cErr)

	if cErr != nil {
		defer C.lsm_free(unsafe.Pointer(cErr))
		return errors.New(C.GoString(cErr))
	}
	return nil
}

func (s *LSMStore) Close() {
	C.lsm_db_close(s.db)
}

// 确保 LSMStore 实现了 CentralCache 接口
var _ geecache.CentralCache = (*LSMStore)(nil)

type BatchEntry struct {
	Key   string
	Value []byte
}

func (s *LSMStore) BatchPut(entries []BatchEntry) error {
	if len(entries) == 0 {
		return nil
	}

	// 1. Calculate size
	structSize := unsafe.Sizeof(C.lsm_batch_entry_t{})
	dataSize := 0
	for _, e := range entries {
		dataSize += len(e.Key) + len(e.Value)
	}

	totalSize := C.size_t(int(structSize)*len(entries) + dataSize)

	// 2. Allocate C memory
	block := C.malloc(totalSize)
	defer C.free(block)

	// 3. Fill data
	// The block starts with the array of structs.
	// The data follows immediately after.
	cEntries := (*[1 << 30]C.lsm_batch_entry_t)(block)[:len(entries):len(entries)]

	// Pointer to the data area
	dataPtr := uintptr(block) + uintptr(structSize)*uintptr(len(entries))

	for i, e := range entries {
		// Set Key
		cEntries[i].key = (*C.char)(unsafe.Pointer(dataPtr))
		cEntries[i].key_len = C.size_t(len(e.Key))

		// Copy Key data
		if len(e.Key) > 0 {
			target := unsafe.Slice((*byte)(unsafe.Pointer(dataPtr)), len(e.Key))
			copy(target, e.Key)
			dataPtr += uintptr(len(e.Key))
		}

		// Set Value
		cEntries[i].value = (*C.char)(unsafe.Pointer(dataPtr))
		cEntries[i].val_len = C.size_t(len(e.Value))

		// Copy Value data
		if len(e.Value) > 0 {
			target := unsafe.Slice((*byte)(unsafe.Pointer(dataPtr)), len(e.Value))
			copy(target, e.Value)
			dataPtr += uintptr(len(e.Value))
		}
	}

	// 4. Call
	var cErr *C.char
	C.lsm_batch_put(s.db, (*C.lsm_batch_entry_t)(block), C.size_t(len(entries)), &cErr)

	if cErr != nil {
		defer C.lsm_free(unsafe.Pointer(cErr))
		return errors.New(C.GoString(cErr))
	}
	return nil
}

type BatchGetEntry struct {
	Key   string
	Value []byte
	Found bool
	Error error
}

func (s *LSMStore) BatchGet(keys []string) ([]BatchGetEntry, error) {
	count := len(keys)
	if count == 0 {
		return nil, nil
	}

	// 1. Calculate size for keys
	structSize := unsafe.Sizeof(C.lsm_batch_get_entry_t{})
	keysDataSize := 0
	for _, k := range keys {
		keysDataSize += len(k)
	}

	totalSize := C.size_t(int(structSize)*count + keysDataSize)

	// 2. Allocate C memory
	block := C.malloc(totalSize)
	defer C.free(block)

	cEntries := (*[1 << 30]C.lsm_batch_get_entry_t)(block)[:count:count]
	dataPtr := uintptr(block) + uintptr(structSize)*uintptr(count)

	for i, k := range keys {
		cEntries[i].key = (*C.char)(unsafe.Pointer(dataPtr))
		cEntries[i].key_len = C.size_t(len(k))

		if len(k) > 0 {
			target := unsafe.Slice((*byte)(unsafe.Pointer(dataPtr)), len(k))
			copy(target, k)
			dataPtr += uintptr(len(k))
		}

		// Initialize output fields
		cEntries[i].value = nil
		cEntries[i].val_len = 0
		cEntries[i].found = 0
		cEntries[i].error = nil
	}

	// 3. Call C++
	C.lsm_batch_get(s.db, (*C.lsm_batch_get_entry_t)(block), C.size_t(count))

	// 4. Process results
	results := make([]BatchGetEntry, count)
	for i := 0; i < count; i++ {
		results[i].Key = keys[i]
		if cEntries[i].error != nil {
			results[i].Error = errors.New(C.GoString(cEntries[i].error))
			C.free(unsafe.Pointer(cEntries[i].error))
		} else if cEntries[i].found != 0 {
			results[i].Found = true
			if cEntries[i].val_len > 0 {
				results[i].Value = C.GoBytes(unsafe.Pointer(cEntries[i].value), C.int(cEntries[i].val_len))
				C.free(unsafe.Pointer(cEntries[i].value))
			}
		} else {
			results[i].Found = false
		}
	}

	return results, nil
}
