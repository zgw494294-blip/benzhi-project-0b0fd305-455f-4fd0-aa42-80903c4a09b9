package application

import "geopack/internal/domain"

type CreateSubmission struct {
	IdempotencyKey        string
	ProjectName           string
	AreaBBox              domain.BBox
	RequiredCRS           string
	MaxGroundResolutionCM float64
}
type RegisterArtifact struct {
	SubmissionID    string
	ExpectedVersion uint64
	IdempotencyKey  string
	Artifact        domain.ArtifactInput
	Actor           string
}
type RegisterArtifactBatch struct {
	SubmissionID    string
	ExpectedVersion uint64
	IdempotencyKey  string
	Artifacts       []domain.ArtifactInput
	Actor           string
}
type StartValidation struct {
	SubmissionID    string
	ExpectedVersion uint64
	IdempotencyKey  string
	Actor           string
}
type UpdateDiscrepancy struct {
	SubmissionID    string
	DiscrepancyID   string
	ExpectedVersion uint64
	IdempotencyKey  string
	Assignee        string
	Reason          string
	RemediationNote string
	EvidenceDigest  string
	Actor           string
}
type SubmitRemediation struct {
	SubmissionID    string
	DiscrepancyID   string
	ExpectedVersion uint64
	IdempotencyKey  string
	Artifact        domain.ArtifactInput
	RemediationNote string
	EvidenceDigest  string
	Actor           string
}
type FreezeSubmission struct {
	SubmissionID    string
	ExpectedVersion uint64
	IdempotencyKey  string
	ApprovedBy      string
	PreflightToken  string
}
type BatchAcknowledgeDiscrepancies struct {
	SubmissionID    string
	ExpectedVersion uint64
	IdempotencyKey  string
	Items           []domain.DiscrepancyAssignment
	Actor           string
}

type MutationResult struct {
	Submission         *domain.Submission               `json:"submission"`
	Artifact           *domain.ArtifactRevision         `json:"artifact,omitempty"`
	Artifacts          []domain.ArtifactRevision        `json:"artifacts,omitempty"`
	Validation         *domain.ValidationRun            `json:"validation,omitempty"`
	Discrepancy        *domain.Discrepancy              `json:"discrepancy,omitempty"`
	Discrepancies      []domain.Discrepancy             `json:"discrepancies,omitempty"`
	Receipt            *domain.AcceptanceReceipt        `json:"receipt,omitempty"`
	ManifestDigest     string                           `json:"manifestDigest,omitempty"`
	DiscrepancySummary map[domain.DiscrepancyStatus]int `json:"discrepancySummary,omitempty"`
}
type ReceiptView struct {
	Receipt          domain.AcceptanceReceipt `json:"receipt"`
	Manifest         domain.FrozenManifest    `json:"manifest"`
	ArtifactChecks   []ArtifactIntegrityCheck `json:"artifactChecks"`
	ManifestCheck    IntegrityCheck           `json:"manifestCheck"`
	ReceiptCheck     IntegrityCheck           `json:"receiptCheck"`
	AuditCheck       IntegrityCheck           `json:"auditCheck"`
	ManifestVerified bool                     `json:"manifestVerified"`
	ReceiptVerified  bool                     `json:"receiptVerified"`
	OverallVerified  bool                     `json:"overallVerified"`
	AuditTrail       any                      `json:"auditTrail"`
}

type IntegrityCheck struct {
	Verified bool   `json:"verified"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Message  string `json:"message,omitempty"`
}

type ArtifactIntegrityCheck struct {
	Role              domain.ArtifactRole `json:"role"`
	ArtifactID        string              `json:"artifactId"`
	Revision          uint32              `json:"revision"`
	ObjectKey         string              `json:"objectKey"`
	Exists            bool                `json:"exists"`
	ExpectedSizeBytes int64               `json:"expectedSizeBytes"`
	ActualSizeBytes   int64               `json:"actualSizeBytes,omitempty"`
	SizeMatches       bool                `json:"sizeMatches"`
	ExpectedSHA256    string              `json:"expectedSha256"`
	ActualSHA256      string              `json:"actualSha256,omitempty"`
	DigestMatches     bool                `json:"digestMatches"`
}
