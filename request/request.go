package request

import (
	"github.com/matf16/mrpc/codec"
	"reflect"
)

type Request struct {
	H      *codec.Header
	Argv   reflect.Value
	Replyv reflect.Value
}
