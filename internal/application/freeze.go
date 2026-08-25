package application

import (
	"context"
	"geopack/internal/domain"
)

func (s *Service) Freeze(ctx context.Context, cmd FreezeSubmission) (MutationResult, error) {
	return s.execute(ctx, cmd.SubmissionID, "freeze", cmd.IdempotencyKey, cmd, func(sub *domain.Submission) (MutationResult, error) {
		if cmd.PreflightToken != "" && !sub.ValidatePreflightToken(cmd.PreflightToken) {
			return MutationResult{}, NewError("preflight_expired", "冻结预检已过期")
		}
		if err := s.checkVersion(sub, cmd.ExpectedVersion); err != nil {
			return MutationResult{}, err
		}
		if report := sub.FreezePreflight(cmd.ApprovedBy, cmd.ExpectedVersion); !report.Ready {
			return MutationResult{}, NewError("not_approvable", "冻结条件不满足")
		}
		manifest, err := sub.Freeze(cmd.ApprovedBy, s.clock())
		if err != nil {
			return MutationResult{}, err
		}
		if err = s.event("submission.frozen", sub, cmd.ApprovedBy, map[string]any{"manifestDigest": manifest.Digest, "frozenVersion": manifest.FrozenVersion}); err != nil {
			return MutationResult{}, err
		}
		sequence, previous := s.audit.NextReceiptSequence()
		receipt := domain.NewReceipt(sequence, previous, cmd.ApprovedBy, manifest, s.clock())
		if err = sub.AttachReceipt(receipt, s.clock()); err != nil {
			return MutationResult{}, err
		}
		if err = s.event("receipt.issued", sub, cmd.ApprovedBy, map[string]any{"receiptId": receipt.ReceiptID, "sequence": receipt.Sequence, "previousReceiptDigest": receipt.PreviousReceiptDigest, "receiptDigest": receipt.ReceiptDigest}); err != nil {
			return MutationResult{}, err
		}
		return MutationResult{Receipt: &receipt}, nil
	})
}

func (s *Service) FreezePreview(ctx context.Context, id string, expectedVersion uint64, approver string) (domain.FreezePreflight, error) {
	sub, err := s.Get(ctx, id)
	if err != nil {
		return domain.FreezePreflight{}, err
	}
	return sub.FreezePreflight(approver, expectedVersion), nil
}

func (s *Service) Receipt(ctx context.Context, id string, role *domain.ArtifactRole) (ReceiptView, error) {
	sub, err := s.Get(ctx, id)
	if err != nil {
		return ReceiptView{}, err
	}
	if sub.Receipt == nil || sub.FrozenManifest == nil {
		return ReceiptView{}, NewError("receipt_not_found", "批次尚未签发接收凭据")
	}
	if role != nil && !role.Valid() {
		return ReceiptView{}, NewError("invalid_artifact_role", "未知成果角色 %q", *role)
	}
	actualManifest := domain.ManifestDigestFor(sub.FrozenManifest.Artifacts)
	manifestOK := actualManifest == sub.FrozenManifest.Digest && sub.Receipt.ManifestDigest == sub.FrozenManifest.Digest
	actualReceipt := domain.ReceiptDigest(*sub.Receipt)
	receiptOK := actualReceipt == sub.Receipt.ReceiptDigest
	allChecks := make([]ArtifactIntegrityCheck, 0, len(sub.FrozenManifest.Artifacts))
	objectsOK := true
	for _, artifact := range sub.FrozenManifest.Artifacts {
		check, checkErr := s.store.VerifyObject(ctx, artifact.ObjectKey, artifact.SizeBytes, artifact.SHA256)
		if checkErr != nil {
			return ReceiptView{}, checkErr
		}
		item := ArtifactIntegrityCheck{Role: artifact.Role, ArtifactID: artifact.ArtifactID, Revision: artifact.Revision, ObjectKey: artifact.ObjectKey, Exists: check.Exists, ExpectedSizeBytes: check.ExpectedSize, ActualSizeBytes: check.ActualSize, SizeMatches: check.SizeMatches, ExpectedSHA256: check.ExpectedSHA256, ActualSHA256: check.ActualSHA256, DigestMatches: check.DigestMatches}
		allChecks = append(allChecks, item)
		objectsOK = objectsOK && item.Exists && item.SizeMatches && item.DigestMatches
	}
	frames, traceErr := s.audit.VerifyTrace(id)
	linkErr := s.audit.VerifyReceiptLink(sub.Receipt.Sequence, sub.Receipt.PreviousReceiptDigest, sub.Receipt.ReceiptDigest)
	auditOK := traceErr == nil && linkErr == nil
	auditMessage := ""
	if traceErr != nil {
		auditMessage = traceErr.Error()
	} else if linkErr != nil {
		auditMessage = linkErr.Error()
	}
	details := allChecks
	if role != nil {
		details = []ArtifactIntegrityCheck{}
		for _, item := range allChecks {
			if item.Role == *role {
				details = append(details, item)
			}
		}
	}
	return ReceiptView{Receipt: *sub.Receipt, Manifest: *sub.FrozenManifest, ArtifactChecks: details,
		ManifestCheck: IntegrityCheck{Verified: manifestOK, Expected: sub.FrozenManifest.Digest, Actual: actualManifest},
		ReceiptCheck:  IntegrityCheck{Verified: receiptOK, Expected: sub.Receipt.ReceiptDigest, Actual: actualReceipt},
		AuditCheck:    IntegrityCheck{Verified: auditOK, Message: auditMessage}, ManifestVerified: manifestOK, ReceiptVerified: receiptOK, OverallVerified: objectsOK && manifestOK && receiptOK && auditOK, AuditTrail: frames}, nil
}
