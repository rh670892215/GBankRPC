package codec

import "io"

type Header struct {
	ServiceMethod string
	// 请求序号，区分不同请求
	Seq           uint64
	Err           string
}

type Codec interface {
	io.Closer
	ReadHeader(*Header) error
	ReadBody(interface{}) error
	Write(*Header, interface{}) error
}

type NewCodecFunc func(closer io.ReadWriteCloser) Codec

type Type string

const (
	JsonCodecType Type = "application/json"
	GobCodecType  Type = "application/gob"
)

var NewCodecFuncTable map[Type]NewCodecFunc

func init() {
	NewCodecFuncTable = make(map[Type]NewCodecFunc)
	NewCodecFuncTable[GobCodecType] = NewGobCodec
}
