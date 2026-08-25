package audit

import "fmt"

func (l *Log) VerifyTrace(submissionID string) ([]Frame, error) {
	l.mu.Lock()
	previous := ""
	for index, frame := range l.frames {
		if err := verifyFrame(frame, uint64(index+1), previous); err != nil {
			l.mu.Unlock()
			return nil, err
		}
		previous = frame.Digest
	}
	l.mu.Unlock()
	frames := l.EventsFor(submissionID)
	if len(frames) == 0 {
		return nil, fmt.Errorf("批次没有审计轨迹")
	}
	created := false
	issued := false
	lastVersion := uint64(0)
	for _, frame := range frames {
		if frame.Payload.Type == "submission.created" {
			created = true
		}
		if frame.Payload.Type == "receipt.issued" {
			issued = true
		}
		if frame.Payload.AggregateVersion < lastVersion {
			return nil, fmt.Errorf("聚合版本在审计轨迹中倒退")
		}
		lastVersion = frame.Payload.AggregateVersion
	}
	if !created {
		return nil, fmt.Errorf("审计轨迹缺少创建事件")
	}
	if issued {
		foundFreeze := false
		for _, f := range frames {
			if f.Payload.Type == "submission.frozen" {
				foundFreeze = true
			}
		}
		if !foundFreeze {
			return nil, fmt.Errorf("签发前缺少冻结事件")
		}
	}
	return frames, nil
}

func (l *Log) VerifyReceiptLink(sequence uint64, expectedPrevious, digest string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	count := uint64(0)
	previous := ""
	for _, frame := range l.frames {
		if frame.Payload.Type != "receipt.issued" {
			continue
		}
		count++
		current, _ := frame.Payload.Details["receiptDigest"].(string)
		if count == sequence {
			if previous != expectedPrevious {
				return fmt.Errorf("前序凭据摘要引用不一致")
			}
			if current != digest {
				return fmt.Errorf("审计凭据摘要与凭据不一致")
			}
			return nil
		}
		previous = current
	}
	return fmt.Errorf("审计轨迹中缺少凭据序号 %d", sequence)
}
