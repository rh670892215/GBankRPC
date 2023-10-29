package main

import (
	"GbankRPC"
	"GbankRPC/codec"
	"encoding/json"
	"fmt"
	"log"
	"net"
)

func serverTestMain() {
	addr := make(chan string)
	// server
	go func() {
		ls, err := net.Listen("tcp", ":0")
		if err != nil {
			log.Println("net listen error", err)
			return
		}

		s := GbankRPC.NewServer()
		addr <- ls.Addr().String()
		s.Accept(ls)
	}()

	// client
	conn, err := net.Dial("tcp", <-addr)
	if err != nil {
		log.Println("net dial error", err)
		return
	}

	opt := GbankRPC.Option{
		MagicNumber: GbankRPC.MagicNumber,
		CodecType:   codec.GobCodecType,
	}
	if err := json.NewEncoder(conn).Encode(opt); err != nil {
		log.Println("json encode error", err)
		return
	}

	cc := codec.NewGobCodec(conn)

	for i := 0; i < 5; i++ {
		h := &codec.Header{
			ServiceMethod: "Foo.Sum",
			Seq:           uint64(i),
		}
		cc.Write(h, fmt.Sprintf("request header seq is %d", h.Seq))
		var reply string
		cc.ReadHeader(h)
		cc.ReadBody(&reply)
		log.Println("reply is", reply)
	}
}
