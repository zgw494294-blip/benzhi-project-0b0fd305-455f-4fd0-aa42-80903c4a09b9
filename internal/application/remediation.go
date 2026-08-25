package application

import (
	"context"
	"geopack/internal/domain"
)

func (s *Service) BatchAcknowledge(ctx context.Context, cmd BatchAcknowledgeDiscrepancies) (MutationResult, error) {
	return s.execute(ctx, cmd.SubmissionID, "discrepancy-batch", cmd.IdempotencyKey, cmd, func(sub *domain.Submission) (MutationResult, error) {
		if err := s.checkVersion(sub, cmd.ExpectedVersion); err != nil {
			return MutationResult{}, err
		}
		updated, err := sub.AcknowledgeBatch(cmd.Items, s.clock())
		if err != nil {
			return MutationResult{}, err
		}
		details := make([]map[string]any, 0, len(updated))
		for _, item := range updated {
			details = append(details, map[string]any{"discrepancyId": item.DiscrepancyID, "assignee": item.Assignee, "reasonDigest": domain.DigestBytes([]byte(item.Reason))})
		}
		if err = s.event("discrepancies.batch_acknowledged", sub, cmd.Actor, map[string]any{"items": details}); err != nil {
			return MutationResult{}, err
		}
		return MutationResult{Discrepancies: updated, DiscrepancySummary: discrepancySummary(sub.Discrepancies)}, nil
	})
}

func (s *Service) PreviewRemediation(ctx context.Context, id, discrepancyID string, expectedVersion uint64, proposed domain.ProposedArtifactMetadata) (domain.RemediationPreview, error) {
	sub, err := s.Get(ctx, id)
	if err != nil {
		return domain.RemediationPreview{}, err
	}
	if err = s.checkVersion(sub, expectedVersion); err != nil {
		return domain.RemediationPreview{}, err
	}
	return sub.PreviewRemediation(discrepancyID, proposed)
}

func (s *Service) UpdateDiscrepancy(ctx context.Context, cmd UpdateDiscrepancy) (MutationResult, error) {
	return s.execute(ctx, cmd.SubmissionID, "discrepancy", cmd.IdempotencyKey, cmd, func(sub *domain.Submission) (MutationResult, error) {
		if err := s.checkVersion(sub, cmd.ExpectedVersion); err != nil {
			return MutationResult{}, err
		}
		d, err := domain.FindDiscrepancy(sub, cmd.DiscrepancyID)
		if err != nil {
			return MutationResult{}, err
		}
		if err = d.Acknowledge(cmd.Assignee, cmd.Reason); err != nil {
			return MutationResult{}, err
		}
		if cmd.RemediationNote != "" || cmd.EvidenceDigest != "" {
			if err = d.Remediate(cmd.RemediationNote, cmd.EvidenceDigest, "manual-evidence"); err != nil {
				return MutationResult{}, err
			}
		}
		sub.Version++
		sub.UpdatedAt = s.clock().UTC()
		copy := *d
		if err = s.event("discrepancy.updated", sub, cmd.Actor, map[string]any{"discrepancyId": d.DiscrepancyID, "status": d.Status}); err != nil {
			return MutationResult{}, err
		}
		return MutationResult{Discrepancy: &copy}, nil
	})
}

func (s *Service) Remediate(ctx context.Context, cmd SubmitRemediation) (MutationResult, error) {
	return s.execute(ctx, cmd.SubmissionID, "remediate", cmd.IdempotencyKey, cmd, func(sub *domain.Submission) (MutationResult, error) {
		if err := s.checkVersion(sub, cmd.ExpectedVersion); err != nil {
			return MutationResult{}, err
		}
		d, err := domain.FindDiscrepancy(sub, cmd.DiscrepancyID)
		if err != nil {
			return MutationResult{}, err
		}
		if d.Status == domain.DiscrepancyOpen {
			if err = d.Acknowledge(cmd.Actor, "成果修订整改"); err != nil {
				return MutationResult{}, err
			}
		}
		if d.Role != "" && d.Role != cmd.Artifact.Role {
			return MutationResult{}, domain.NewError("incompatible_artifact_role", "拟提交角色与差异关联角色不相容")
		}
		key, err := s.store.PutObject(ctx, cmd.Artifact.Content, cmd.Artifact.SHA256, cmd.Artifact.SizeBytes)
		if err != nil {
			return MutationResult{}, err
		}
		a, err := sub.RegisterArtifact(cmd.Artifact, key, s.clock())
		if err != nil {
			return MutationResult{}, err
		}
		if err = d.Remediate(cmd.RemediationNote, cmd.EvidenceDigest, a.Key()); err != nil {
			return MutationResult{}, err
		}
		run := domain.RunValidation(sub, s.clock(), domain.AffectedChecks(a.Role))
		sub.ApplyValidation(run, s.clock())
		if run.Outcome == domain.OutcomePassed {
			for i := range sub.Discrepancies {
				if sub.Discrepancies[i].Status == domain.DiscrepancyRemediated {
					sub.Discrepancies[i].Close(s.clock())
				}
			}
			sub.Status = domain.StatusApprovable
		}
		if err = s.event("remediation.validated", sub, cmd.Actor, map[string]any{"discrepancyId": d.DiscrepancyID, "revision": a.Key(), "outcome": run.Outcome}); err != nil {
			return MutationResult{}, err
		}
		return MutationResult{Artifact: &a, Validation: &run}, nil
	})
}
