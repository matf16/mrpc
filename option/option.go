package option

import "github.com/matf16/mrpc/codec"

const MagicNumber = 0x5a6b7c8d

type Option struct {
	MagicNumber int
	CodecType   codec.Type
}

var DefaultOption = Option{
	MagicNumber: MagicNumber,
	CodecType:   codec.GobType,
}
