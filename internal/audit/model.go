package audit

import "time"

const SchemaVersion = 1

type Event struct {
	Type             string         `json:"type"`
	SubmissionID     string         `json:"submissionId"`
	AggregateVersion uint64         `json:"aggregateVersion"`
	Actor            string         `json:"actor,omitempty"`
	Details          map[string]any `json:"details,omitempty"`
	OccurredAt       time.Time      `json:"occurredAt"`
}

type Frame struct {
	SchemaVersion  int    `json:"schemaVersion"`
	Sequence       uint64 `json:"sequence"`
	PreviousDigest string `json:"previousDigest"`
	Payload        Event  `json:"payload"`
	Checksum       string `json:"checksum"`
	Digest         string `json:"digest"`
}

type Verification struct {
	Frames       int    `json:"frames"`
	LastSequence uint64 `json:"lastSequence"`
	LastDigest   string `json:"lastDigest"`
}
