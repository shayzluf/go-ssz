package ssz

var (
	// BytesPerChunk for an SSZ serialized object.
	BytesPerChunk = 32
	// BytesPerLengthOffset defines a constant for off-setting serialized chunks.
	BytesPerLengthOffset = 4
	// BitsPerByte as a useful constant.
	BitsPerByte = 8
	// MaxByteOffset allowed in serialization.
	MaxByteOffset = 1 << uint64(BytesPerLengthOffset*BitsPerByte)
)
