package worker

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
)

// TestAFrameReadsBackAsItWasWritten is the ordinary round trip, present so that
// the interesting tests below can be about the properties rather than about
// whether the encoding works at all.
func TestAFrameReadsBackAsItWasWritten(t *testing.T) {
	var buf bytes.Buffer
	want := Response{
		Certificates: []CertificatePayload{
			{DER: []byte{0x30, 0x82, 0x01}, Label: "Pošta CA", ID: []byte{0x01, 0x02}},
		},
		Chain: [][]byte{{0xAA}, {0xBB, 0xCC}},
	}
	if err := WriteFrame(&buf, want); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}

	var got Response
	if err := ReadFrame(&buf, &got); err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if len(got.Certificates) != 1 || got.Certificates[0].Label != "Pošta CA" {
		t.Errorf("certificates came back as %+v", got.Certificates)
	}
	if !bytes.Equal(got.Certificates[0].DER, want.Certificates[0].DER) {
		t.Errorf("DER came back as %x", got.Certificates[0].DER)
	}
	if len(got.Chain) != 2 || !bytes.Equal(got.Chain[1], []byte{0xBB, 0xCC}) {
		t.Errorf("chain came back as %x", got.Chain)
	}
	if buf.Len() != 0 {
		t.Errorf("%d bytes left in the buffer after one frame", buf.Len())
	}
}

// TestReadFrameTakesExactlyItsOwnBytesAndNotOneMore is the test for SPEC
// §6.5.1 clause 2's "never buffered", and it is the reason the framing is
// length-prefixed at all.
//
// The scenario is the one that matters: a request, and immediately behind it in
// the same pipe, a PIN. If the reader takes one byte more than the frame, the
// PIN — or part of it — is now in a buffer inside this process, where nothing
// overwrites it and no guard in pin_test.go can see it, because it is not a
// field, a parameter or a named result.
//
// The check is that the bytes after the frame are still *in the reader*,
// untouched, and can be read by the exact-length read the clause describes.
func TestReadFrameTakesExactlyItsOwnBytesAndNotOneMore(t *testing.T) {
	var pipe bytes.Buffer
	if err := WriteFrame(&pipe, Request{Op: OpList}); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}

	// Standing in for the PIN: bytes written immediately behind the request,
	// which must not be consumed by reading the request. Not a real PIN and
	// not shaped like one — what is under test is the reader's appetite.
	const following = "0123456789"
	pipe.WriteString(following)

	var req Request
	if err := ReadFrame(&pipe, &req); err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if req.Op != OpList {
		t.Fatalf("the request came back as %q", req.Op)
	}

	if pipe.Len() != len(following) {
		t.Fatalf("reading one frame consumed %d of the %d bytes behind it.\n\n"+
			"SPEC §6.5.1 clause 2 bounds the PIN to one write, read immediately, "+
			"never buffered. A reader with an appetite past the frame pulls the "+
			"next thing in the pipe into this process's heap, where nothing "+
			"overwrites it and no guard in pin_test.go can see it — it is not a "+
			"field, a parameter or a named result, it is somebody else's byte slice.",
			len(following)-pipe.Len(), len(following))
	}

	rest := make([]byte, len(following))
	if _, err := io.ReadFull(&pipe, rest); err != nil {
		t.Fatalf("the bytes behind the frame could not be read: %v", err)
	}
	if string(rest) != following {
		t.Errorf("the bytes behind the frame came back as %q", rest)
	}
}

// TestAFrameThatEndsEarlyIsAWorkerThatDied is the shape a crash takes on the
// parent's side of the pipe.
//
// D-272's module kills its process mid-call. What the parent sees is not an
// error message: it is the pipe ending in the middle of an answer. That has to
// arrive as an ordinary error the supervisor can turn into a Failure, never as
// a panic and never as a successfully-decoded zero value — the last being the
// dangerous one, because an empty certificate list reads exactly like a card
// with nothing on it.
func TestAFrameThatEndsEarlyIsAWorkerThatDied(t *testing.T) {
	var full bytes.Buffer
	if err := WriteFrame(&full, Response{Certificates: []CertificatePayload{{DER: []byte{1, 2, 3, 4, 5, 6, 7, 8}}}}); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	truncated := full.Bytes()[:full.Len()-4]

	var got Response
	err := ReadFrame(bytes.NewReader(truncated), &got)
	if err == nil {
		t.Fatal("a truncated frame decoded successfully, which is a dead worker " +
			"reported as a token with nothing on it")
	}
	if !errors.Is(err, ErrShortFrame) {
		t.Errorf("a truncated frame produced %v, want it to wrap %v", err, ErrShortFrame)
	}
}

// TestAnEndBetweenFramesIsNotAnError distinguishes the worker shutting down
// cleanly from the worker dying, which are the same bytes-on-the-wire
// difference of four: none versus some-but-not-enough.
func TestAnEndBetweenFramesIsNotAnError(t *testing.T) {
	var got Response
	err := ReadFrame(bytes.NewReader(nil), &got)
	if !errors.Is(err, io.EOF) {
		t.Errorf("an empty reader produced %v, want io.EOF", err)
	}
	if errors.Is(err, ErrShortFrame) {
		t.Error("a clean end between frames was reported as a worker that died " +
			"mid-answer; the supervisor would respawn for nothing")
	}
}

// TestALengthPrefixCannotAskForAnArbitraryAllocation is the check on the one
// number in this protocol that a hostile or corrupt peer controls.
//
// The parent reads frames from a process that has had a vendor module running
// in its address space. A four-byte length is up to 4GB, and the reader would
// allocate it before discovering there is nothing to fill it with.
func TestALengthPrefixCannotAskForAnArbitraryAllocation(t *testing.T) {
	for _, claim := range []uint32{maxFrame + 1, 1 << 30, 0xFFFFFFFF} {
		var header [4]byte
		binary.BigEndian.PutUint32(header[:], claim)

		var got Response
		err := ReadFrame(bytes.NewReader(header[:]), &got)
		if !errors.Is(err, ErrFrameTooLarge) {
			t.Errorf("a frame claiming %d bytes produced %v, want it to wrap %v",
				claim, err, ErrFrameTooLarge)
		}
	}
}

// TestWriteFrameRefusesToSendMoreThanItWouldAccept keeps the two halves of the
// bound the same. A worker that can send what the parent will not read is a
// worker whose large answers fail at the far end, where the reason is a
// protocol error rather than the real one.
func TestWriteFrameRefusesToSendMoreThanItWouldAccept(t *testing.T) {
	oversized := Response{Chain: [][]byte{make([]byte, maxFrame)}}
	err := WriteFrame(io.Discard, oversized)
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Errorf("writing an oversized frame produced %v, want it to wrap %v", err, ErrFrameTooLarge)
	}
}

// TestOneWriteCarriesTheWholeFrame checks that a frame is not torn across two
// Writes. A reader reassembles either way; what this protects is the case where
// the process dies between the two, which would leave a header on the wire with
// no payload behind it and is indistinguishable from a much longer answer that
// has not arrived yet.
func TestOneWriteCarriesTheWholeFrame(t *testing.T) {
	c := &countingWriter{}
	if err := WriteFrame(c, Request{Op: OpEnumerate}); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	if c.writes != 1 {
		t.Errorf("one frame took %d Write calls, want 1.\n\n"+
			"A header written separately from its payload can be the last thing a "+
			"dying worker emits, and the parent cannot tell that from an answer "+
			"still on its way.", c.writes)
	}
}

type countingWriter struct{ writes int }

func (c *countingWriter) Write(p []byte) (int, error) { c.writes++; return len(p), nil }

// TestTheOperationNamesAreStableOnTheWire guards the strings rather than the
// constants. The parent and the worker are the same binary today, so a rename
// would be invisible in testing and would matter the first time a half-upgraded
// installation has one of each — which F10's update channel makes possible.
func TestTheOperationNamesAreStableOnTheWire(t *testing.T) {
	for op, want := range map[Op]string{
		OpEnumerate: "enumerate",
		OpList:      "list",
		OpChainFor:  "chainfor",
		OpShutdown:  "shutdown",
	} {
		if string(op) != want {
			t.Errorf("an operation's wire name changed to %q, want %q.\n\n"+
				"The parent and the worker are the same binary in this tree, so "+
				"nothing here would notice. An installation mid-upgrade can have "+
				"one of each.", string(op), want)
		}
		if strings.ContainsAny(string(op), ` "\`) {
			t.Errorf("operation %q contains a character that will need quoting", string(op))
		}
	}
}
