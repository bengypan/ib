package trade

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"reflect"
	"strings"
	"time"
)

const (
	version = 48
	gateway = "127.0.0.1:4001"
)

type Engine struct {
	client        long
	tick          long
	nextTick      chan long
	con           net.Conn
	reader        *bufio.Reader
	input         *bytes.Buffer
	output        *bytes.Buffer
	serverTime    time.Time
	clientVersion long
	serverVersion long
}

func uniqueId() chan long {
	ch := make(chan long)
	id := long(0)
	go func() {
		for {
			ch <- id
			id += 1
		}
	}()
	return ch
}

// Set up the engine 

func Make(client long) (*Engine, error) {
	con, err := net.Dial("tcp", gateway)
	if err != nil {
		return nil, err
	}

	reader := bufio.NewReader(con)
	input := bytes.NewBuffer(make([]byte, 0, 4096))
	output := bytes.NewBuffer(make([]byte, 0, 4096))
	tick := uniqueId()

	engine := Engine{
		client:   client,
		nextTick: tick,
		con:      con,
		reader:   reader,
		input:    input,
		output:   output,
	}

	// write client version
	cver := &clientVersion{version}
	if err := engine.write(cver); err != nil {
		return nil, err
	}

	// read server version
	sver := &serverVersion{}
	if err := engine.read(sver); err != nil {
		return nil, err
	}

	// read server time
	tm := &serverTime{}
	if err := engine.read(tm); err != nil {
		return nil, err
	}

	// write client id
	id := &clientId{client}
	if err := engine.write(id); err != nil {
		return nil, err
	}

	engine.serverVersion = sver.Version
	engine.serverTime = tm.Time

	return &engine, nil
}

type PacketError struct {
	Value interface{}
	Type  reflect.Type
}

func (e *PacketError) Error() string {
	return fmt.Sprintf("don't understand packet '%v' of type '%v'",
		e.Value, e.Type)
}

func failPacket(v interface{}) error {
	return &PacketError{
		Value: v,
		Type:  reflect.ValueOf(v).Type(),
	}
}

func dump(b *bytes.Buffer) {
	s := strings.Replace(b.String(), "\000", "-", -1)
	fmt.Printf("Buffer = '%s'\n", s)
}

func (engine *Engine) Send(tick long, v interface{}) error {
	type header struct {
		//Client  long
		Code    long
		Version long
		Tick    long
	}

	engine.output.Reset()

	code := msg2Code(v)
	if code == 0 {
		return failPacket(v)
	}

	// encode message type and client version
	ver := code2Version(code)
	hdr := &header{
		//Client:  engine.client,
		Code:    code,
		Version: ver,
		Tick:    tick,
	}
	fmt.Printf("Sending message '%v' with code %d and tick id %d\n", v, code, hdr.Tick)
	if err := Encode(engine.output, hdr); err != nil {
		return err
	}

	// encode the message itself
	if err := Encode(engine.output, v); err != nil {
		return err
	}

	//dump(engine.output)

	if _, err := engine.con.Write(engine.output.Bytes()); err != nil {
		return err
	}

	return nil
}

func (engine *Engine) Receive() (interface{}, error) {
	type header struct {
		Code    long
		Version long
	}

	engine.input.Reset()
	hdr := &header{}

	// decode header
	if err := Decode(engine.reader, hdr); err != nil {
		return nil, err
	}

	// decode message
	v := code2Msg(hdr.Code)
	if err := Decode(engine.reader, v); err != nil {
		return nil, err
	}

	return v, nil
}

func (engine *Engine) write(v interface{}) error {
	engine.output.Reset()

	if err := Encode(engine.output, v); err != nil {
		return err
	}

	if _, err := engine.con.Write(engine.output.Bytes()); err != nil {
		return err
	}

	return nil
}

func (engine *Engine) read(v interface{}) error {
	engine.input.Reset()

	if err := Decode(engine.reader, v); err != nil {
		return err
	}
	return nil
}

func (engine *Engine) NextTick() long {
	engine.tick = <-engine.nextTick
	return engine.tick
}

func (engine *Engine) Tick() long {
	return engine.tick
}