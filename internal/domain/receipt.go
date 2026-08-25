package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

type FrozenManifest struct {
	SubmissionID   string             `json:"submissionId"`
	FrozenVersion  uint64             `json:"frozenVersion"`
	RulesetVersion string             `json:"rulesetVersion"`
	Artifacts      []ArtifactRevision `json:"artifacts"`
	Digest         string             `json:"digest"`
}

type AcceptanceReceipt struct {
	ReceiptID             string    `json:"receiptId"`
	Sequence              uint64    `json:"sequence"`
	SubmissionID          string    `json:"submissionId"`
	FrozenVersion         uint64    `json:"frozenVersion"`
	ManifestDigest        string    `json:"manifestDigest"`
	RulesetVersion        string    `json:"rulesetVersion"`
	ApprovedBy            string    `json:"approvedBy"`
	IssuedAt              time.Time `json:"issuedAt"`
	PreviousReceiptDigest string    `json:"previousReceiptDigest"`
	ReceiptDigest         string    `json:"receiptDigest"`
}

func ReceiptDigest(r AcceptanceReceipt) string {
	payload := struct {
		Sequence       uint64 `json:"sequence"`
		SubmissionID   string `json:"submissionId"`
		FrozenVersion  uint64 `json:"frozenVersion"`
		ManifestDigest string `json:"manifestDigest"`
		RulesetVersion string `json:"rulesetVersion"`
		ApprovedBy     string `json:"approvedBy"`
		IssuedAt       string `json:"issuedAt"`
		Previous       string `json:"previous"`
	}{r.Sequence, r.SubmissionID, r.FrozenVersion, r.ManifestDigest, r.RulesetVersion, r.ApprovedBy, r.IssuedAt.UTC().Format(time.RFC3339Nano), r.PreviousReceiptDigest}
	b, _ := json.Marshal(payload)
	return DigestBytes(b)
}

func NewReceipt(sequence uint64, previous, approver string, manifest FrozenManifest, now time.Time) AcceptanceReceipt {
	r := AcceptanceReceipt{ReceiptID: fmt.Sprintf("receipt-%012d", sequence), Sequence: sequence, SubmissionID: manifest.SubmissionID, FrozenVersion: manifest.FrozenVersion, ManifestDigest: manifest.Digest, RulesetVersion: manifest.RulesetVersion, ApprovedBy: approver, IssuedAt: now.UTC(), PreviousReceiptDigest: previous}
	r.ReceiptDigest = ReceiptDigest(r)
	return r
}
