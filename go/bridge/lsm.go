package bridge

// #cgo CFLAGS: -I../../storage/lsm/include
// #cgo LDFLAGS: -L../../storage/lsm/build -llsm -lstdc++ -lsnappy
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
