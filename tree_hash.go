package ssz

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
)

const hashLengthBytes = 32
const sszChunkSize = 128

var useCache bool

type hashError struct {
	msg string
	typ reflect.Type
}

func (err *hashError) Error() string {
	return fmt.Sprintf("hash error: %s for input type %v", err.msg, err.typ)
}

func newHashError(msg string, typ reflect.Type) *hashError {
	return &hashError{msg, typ}
}

// HashTreeRoot determines the root hash using SSZ's merkleization.
func HashTreeRoot(val interface{}) ([32]byte, error) {
	if val == nil {
		return [32]byte{}, newHashError("untyped nil is not supported", nil)
	}
	rval := reflect.ValueOf(val)
	sszUtils, err := cachedSSZUtils(rval.Type())
	if err != nil {
		return [32]byte{}, newHashError(fmt.Sprint(err), rval.Type())
	}
	output, err := sszUtils.hasher(rval)
	if err != nil {
		return [32]byte{}, newHashError(fmt.Sprint(err), rval.Type())
	}
	return ToBytes32(output), nil
}

func makeHasher(typ reflect.Type) (hasher, error) {
	kind := typ.Kind()
	switch {
	case kind == reflect.Bool ||
		kind == reflect.Uint8 ||
		kind == reflect.Uint16 ||
		kind == reflect.Uint32 ||
		kind == reflect.Uint64 ||
		kind == reflect.Array && typ.Elem().Kind() == reflect.Bool ||
		kind == reflect.Array && typ.Elem().Kind() == reflect.Uint8 ||
		kind == reflect.Array && typ.Elem().Kind() == reflect.Uint16 ||
		kind == reflect.Array && typ.Elem().Kind() == reflect.Uint32 ||
		kind == reflect.Array && typ.Elem().Kind() == reflect.Uint64:
		return makeBasicTypeHasher(typ)
	case kind == reflect.Slice && typ.Elem().Kind() == reflect.Uint8 ||
		kind == reflect.Array && typ.Elem().Kind() == reflect.Uint8:
		return makeByteArrayHasher(typ)
	case kind == reflect.Slice && typ.Elem().Kind() == reflect.Bool ||
		kind == reflect.Slice && typ.Elem().Kind() == reflect.Uint8 ||
		kind == reflect.Slice && typ.Elem().Kind() == reflect.Uint16 ||
		kind == reflect.Slice && typ.Elem().Kind() == reflect.Uint32 ||
		kind == reflect.Slice && typ.Elem().Kind() == reflect.Uint64:
		if useCache {
			return makeBasicSliceHasherCache(typ)
		}
		return makeBasicSliceHasher(typ)
	case kind == reflect.Struct:
		if useCache {
			return makeStructHasherCache(typ)
		}
		return makeStructHasher(typ)
	case kind == reflect.Ptr:
		return makePtrHasher(typ)
	default:
		return nil, fmt.Errorf("type %v is not hashable", typ)
	}
}

func makeBasicTypeHasher(typ reflect.Type) (hasher, error) {
	utils, err := cachedSSZUtilsNoAcquireLock(typ)
	if err != nil {
		return nil, err
	}
	hasher := func(val reflect.Value) ([32]byte, error) {
		buf := &encbuf{}
		if err = utils.encoder(val, buf); err != nil {
			return [32]byte{}, err
		}
		writer := new(bytes.Buffer)
		if err = buf.toWriter(writer); err != nil {
			return [32]byte{}, err
		}
		serialized := writer.Bytes()
		chunks, err := pack([][]byte{serialized})
		if err != nil {
			return [32]byte{}, err
		}
		return merkleize(chunks), nil
	}
	return hasher, nil
}

func makeBasicSliceHasher(typ reflect.Type) (hasher, error) {
	elemSSZUtils, err := cachedSSZUtilsNoAcquireLock(typ.Elem())
	if err != nil {
		return nil, fmt.Errorf("failed to get ssz utils: %v", err)
	}
	hasher := func(val reflect.Value) ([32]byte, error) {
		var serializedValues [][]byte
		for i := 0; i < val.Len(); i++ {
			buf := &encbuf{}
			if err = elemSSZUtils.encoder(val.Index(i), buf); err != nil {
				return [32]byte{}, err
			}
			writer := new(bytes.Buffer)
			if err = buf.toWriter(writer); err != nil {
				return [32]byte{}, err
			}
			serializedValues = append(serializedValues, writer.Bytes())
		}
		chunks, err := pack(serializedValues)
		if err != nil {
			return [32]byte{}, err
		}
		b := make([]byte, 32)
		binary.LittleEndian.PutUint64(b, uint64(val.Len()))
		return mixInLength(merkleize(chunks), b), nil
	}
	return hasher, nil
}

func makeStructHasher(typ reflect.Type) (hasher, error) {
	fields, err := structFields(typ)
	if err != nil {
		return nil, err
	}
	hasher := func(val reflect.Value) ([]byte, error) {
		concatElemHash := make([]byte, 0)
		for _, f := range fields {
			elemHash, err := f.sszUtils.hasher(val.Field(f.index))
			if err != nil {
				return nil, fmt.Errorf("failed to hash field of struct: %v", err)
			}
			concatElemHash = append(concatElemHash, elemHash...)
		}
		result := Hash(concatElemHash)
		return result[:], nil
	}
	return hasher, nil
}

func makePtrHasher(typ reflect.Type) (hasher, error) {
	elemSSZUtils, err := cachedSSZUtilsNoAcquireLock(typ.Elem())
	if err != nil {
		return nil, err
	}

	// TODO(1461): The tree-hash of nil pointer isn't defined in the spec.
	// After considered the use case in Prysm, we've decided that:
	// - We assume we will only tree-hash pointer of array, slice or struct.
	// - The tree-hash for nil pointer shall be 0x00000000.

	hasher := func(val reflect.Value) ([]byte, error) {
		if val.IsNil() {
			return hashedEncoding(val)
		}
		return elemSSZUtils.hasher(val.Elem())
	}
	return hasher, nil
}

func getEncoding(val reflect.Value) ([]byte, error) {
	utils, err := cachedSSZUtilsNoAcquireLock(val.Type())
	if err != nil {
		return nil, err
	}
	buf := &encbuf{}
	if err = utils.encoder(val, buf); err != nil {
		return nil, err
	}
	writer := new(bytes.Buffer)
	if err = buf.toWriter(writer); err != nil {
		return nil, err
	}
	return writer.Bytes(), nil
}

func hashedEncoding(val reflect.Value) ([]byte, error) {
	encoding, err := getEncoding(val)
	if err != nil {
		return nil, err
	}
	output := Hash(encoding)
	return output[:], nil
}