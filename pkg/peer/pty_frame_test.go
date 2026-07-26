package peer

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestPTYDataFrameRoundTripPreservesBytes(t *testing.T) {
	payload := []byte{0, 0xff, '\r', '\n', 0x1b, '[', 'A'}
	encoded, err := EncodePTYDataFrame(PTYDataInput, 42, payload)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodePTYDataFrame(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Direction != PTYDataInput || decoded.Sequence != 42 || !bytes.Equal(decoded.Payload, payload) {
		t.Fatalf("decoded frame = %#v", decoded)
	}
}

func TestPTYDataFrameRejectsMalformedHeaderAndLength(t *testing.T) {
	valid, _ := EncodePTYDataFrame(PTYDataOutput, 1, []byte("ok"))
	tests := [][]byte{
		valid[:10],
		append([]byte("NOPE"), valid[4:]...),
		append([]byte(nil), valid...),
		append([]byte(nil), valid...),
	}
	tests[2][4] = 2
	binary.BigEndian.PutUint32(tests[3][16:20], 99)
	for _, data := range tests {
		if _, err := DecodePTYDataFrame(data); err == nil {
			t.Fatalf("accepted malformed frame %x", data)
		}
	}
}

func TestPTYControlFrameValidation(t *testing.T) {
	encoded, err := EncodePTYControlFrame(PTYControlFrame{
		Version: PTYFrameVersion, Type: "resize", Sequence: 3, Cols: 120, Rows: 40,
	})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodePTYControlFrame(encoded)
	if err != nil || decoded.Cols != 120 || decoded.Rows != 40 {
		t.Fatalf("decoded=%#v err=%v", decoded, err)
	}
	for _, malformed := range [][]byte{
		[]byte(`{"version":1,"type":"resize","sequence":1,"cols":0,"rows":20}`),
		[]byte(`{"version":1,"type":"resize","sequence":1,"cols":80,"rows":20,"extra":true}`),
		[]byte(`{"version":1,"type":"close","sequence":0}`),
		[]byte(`not-json`),
	} {
		if _, err := DecodePTYControlFrame(malformed); err == nil {
			t.Fatalf("accepted malformed control %s", malformed)
		}
	}
}

func TestPTYSequenceDuplicateAndGap(t *testing.T) {
	var sequence PTYSequence
	if duplicate, err := sequence.Accept(1); duplicate || err != nil {
		t.Fatalf("first: duplicate=%v err=%v", duplicate, err)
	}
	if duplicate, err := sequence.Accept(1); !duplicate || err != nil {
		t.Fatalf("duplicate: duplicate=%v err=%v", duplicate, err)
	}
	if _, err := sequence.Accept(3); err == nil {
		t.Fatal("accepted sequence gap")
	}
	if duplicate, err := sequence.Accept(2); duplicate || err != nil {
		t.Fatalf("recovery: duplicate=%v err=%v", duplicate, err)
	}
}
