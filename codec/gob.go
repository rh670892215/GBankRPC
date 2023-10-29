package codec

import (
	"bufio"
	"encoding/gob"
	"io"
)

type GobCodec struct {
	conn io.ReadWriteCloser
	buf  *bufio.Writer
	dec  *gob.Decoder
	enc  *gob.Encoder
}

func NewGobCodec(conn io.ReadWriteCloser) Codec {
	// 使用缓冲区 buf 提升性能
	buf := bufio.NewWriter(conn)
	return &GobCodec{
		conn: conn,
		buf:  buf,
		dec:  gob.NewDecoder(conn),
		// 结果先写入缓冲区
		enc: gob.NewEncoder(buf),
	}
}

func (g *GobCodec) Write(h *Header, body interface{}) error {
	var err error
	defer func() {
		// 将缓冲区中的数据统一写入连接
		g.buf.Flush()
		if err != nil {
			g.conn.Close()
		}
	}()

	if err = g.enc.Encode(h); err != nil {
		return err
	}

	if err = g.enc.Encode(body); err != nil {
		return err
	}

	return nil
}

func (g *GobCodec) ReadHeader(h *Header) error {
	return g.dec.Decode(h)
}

func (g *GobCodec) ReadBody(reply interface{}) error {
	return g.dec.Decode(reply)
}

func (g *GobCodec) Close() error {
	return g.conn.Close()
}
