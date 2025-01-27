package ssz

import "crypto/sha256"

// ToBytes32 is a convenience method for converting a byte slice to a fix
// sized 32 byte array. This method will truncate the input if it is larger
// than 32 bytes.
func ToBytes32(x []byte) [32]byte {
	var y [32]byte
	copy(y[:], x)
	return y
}

// Hash defines a function that returns the sha256 hash of the data passed in.
func Hash(data []byte) [32]byte {
	var hash [32]byte

	h := sha256.New()
	// The hash interface never returns an error, for that reason
	// we are not handling the error below. For reference, it is
	// stated here https://golang.org/pkg/hash/#Hash
	// #nosec G104
	h.Write(data)
	h.Sum(hash[:0])

	return hash
}
