package audit

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

type Log struct {
	mu     sync.Mutex
	path   string
	frames []Frame
}

func Open(dir string) (*Log, error) {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, err
	}
	l := &Log{path: filepath.Join(dir, "audit.frames")}
	if err := l.recover(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Log) recover() error {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0640)
	if err != nil {
		return err
	}
	defer f.Close()
	offset := int64(0)
	previous := ""
	seq := uint64(1)
	for {
		var header [4]byte
		n, err := io.ReadFull(f, header[:])
		if errors.Is(err, io.EOF) {
			break
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return f.Truncate(offset)
		}
		if err != nil {
			return err
		}
		if n != 4 {
			return fmt.Errorf("审计帧头损坏")
		}
		length := binary.BigEndian.Uint32(header[:])
		if length == 0 || length > 16<<20 {
			return fmt.Errorf("审计帧长度非法")
		}
		body := make([]byte, length)
		_, err = io.ReadFull(f, body)
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			return f.Truncate(offset)
		}
		if err != nil {
			return err
		}
		var frame Frame
		if err = json.Unmarshal(body, &frame); err != nil {
			return fmt.Errorf("审计帧 JSON 损坏: %w", err)
		}
		if err = verifyFrame(frame, seq, previous); err != nil {
			return err
		}
		l.frames = append(l.frames, frame)
		previous = frame.Digest
		seq++
		offset += int64(4 + length)
	}
	return nil
}

func (l *Log) Append(event Event) (Frame, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.appendLocked(event)
}

// appendLocked encodes and persists a frame, then records it in the in-memory
// chain. Callers must already hold l.mu so that the previous-digest and
// sequence derivation stay consistent with the on-disk append.
func (l *Log) appendLocked(event Event) (Frame, error) {
	seq := uint64(len(l.frames) + 1)
	previous := ""
	if len(l.frames) > 0 {
		previous = l.frames[len(l.frames)-1].Digest
	}
	frame := Frame{SchemaVersion: SchemaVersion, Sequence: seq, PreviousDigest: previous, Payload: event}
	data, err := encodeFrame(frame)
	if err != nil {
		return Frame{}, err
	}
	var decoded Frame
	if err = json.Unmarshal(data[4:], &decoded); err != nil {
		return Frame{}, err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
	if err != nil {
		return Frame{}, err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return Frame{}, err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return Frame{}, err
	}
	if err = f.Close(); err != nil {
		return Frame{}, err
	}
	l.frames = append(l.frames, decoded)
	return decoded, nil
}

// IssueReceipt atomically allocates the next global receipt sequence number,
// reads the previously issued receipt's digest, invokes build to construct the
// receipt and the receipt.issued audit event, and appends that event while
// still holding the log lock. Performing allocation, previous-digest reads,
// receipt construction and publication within a single critical section
// guarantees that concurrent freezes of different submissions observe unique,
// consecutive receipt sequences and an unbroken receipt digest chain, so each
// later receipt references the previously issued receipt's digest.
func (l *Log) IssueReceipt(build func(sequence uint64, previousReceiptDigest string) (Event, string, error)) (Frame, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	count := uint64(0)
	previous := ""
	for _, f := range l.frames {
		if f.Payload.Type != "receipt.issued" {
			continue
		}
		count++
		if d, ok := f.Payload.Details["receiptDigest"].(string); ok {
			previous = d
		}
	}
	sequence := count + 1
	event, receiptDigest, err := build(sequence, previous)
	if err != nil {
		return Frame{}, err
	}
	if event.Details == nil {
		event.Details = map[string]any{}
	}
	event.Details["receiptDigest"] = receiptDigest
	event.Details["sequence"] = sequence
	event.Details["previousReceiptDigest"] = previous
	return l.appendLocked(event)
}

func (l *Log) EventsFor(submissionID string) []Frame {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := []Frame{}
	for _, f := range l.frames {
		if f.Payload.SubmissionID == submissionID {
			out = append(out, f)
		}
	}
	return out
}
func (l *Log) NextReceiptSequence() (uint64, string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	count := uint64(0)
	prev := ""
	for _, f := range l.frames {
		if f.Payload.Type == "receipt.issued" {
			count++
			if d, ok := f.Payload.Details["receiptDigest"].(string); ok {
				prev = d
			}
		}
	}
	return count + 1, prev
}
func (l *Log) Verify() Verification {
	l.mu.Lock()
	defer l.mu.Unlock()
	v := Verification{Frames: len(l.frames)}
	if len(l.frames) > 0 {
		v.LastSequence = l.frames[len(l.frames)-1].Sequence
		v.LastDigest = l.frames[len(l.frames)-1].Digest
	}
	return v
}
