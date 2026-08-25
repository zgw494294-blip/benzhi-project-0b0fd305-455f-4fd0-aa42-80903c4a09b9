package audit

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

func sum(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }

func encodeFrame(frame Frame) ([]byte, error) {
	frame.Checksum = ""
	frame.Digest = ""
	core, err := json.Marshal(frame)
	if err != nil {
		return nil, err
	}
	frame.Checksum = sum(core)
	digestInput := append([]byte(frame.PreviousDigest), core...)
	frame.Digest = sum(digestInput)
	body, err := json.Marshal(frame)
	if err != nil {
		return nil, err
	}
	if len(body) > 16<<20 {
		return nil, fmt.Errorf("审计帧过大")
	}
	out := make([]byte, 4+len(body))
	binary.BigEndian.PutUint32(out[:4], uint32(len(body)))
	copy(out[4:], body)
	return out, nil
}

func verifyFrame(frame Frame, expected uint64, previous string) error {
	if frame.SchemaVersion != SchemaVersion {
		return fmt.Errorf("不支持的审计 schemaVersion %d", frame.SchemaVersion)
	}
	if frame.Sequence != expected {
		return fmt.Errorf("审计序号不连续: 得到 %d, 期望 %d", frame.Sequence, expected)
	}
	if frame.PreviousDigest != previous {
		return fmt.Errorf("审计摘要链断裂")
	}
	checksum, digest := frame.Checksum, frame.Digest
	frame.Checksum = ""
	frame.Digest = ""
	core, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	if sum(core) != checksum {
		return fmt.Errorf("审计帧校验和错误")
	}
	if sum(append([]byte(previous), core...)) != digest {
		return fmt.Errorf("审计帧摘要错误")
	}
	return nil
}
