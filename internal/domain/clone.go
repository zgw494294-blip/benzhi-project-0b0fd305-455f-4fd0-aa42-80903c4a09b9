package domain

func CloneSubmission(s *Submission) *Submission {
	if s == nil {
		return nil
	}
	cloned := *s
	cloned.ValidationRuns = append([]ValidationRun(nil), s.ValidationRuns...)
	cloned.Discrepancies = append([]Discrepancy(nil), s.Discrepancies...)
	if s.FrozenManifest != nil {
		manifest := *s.FrozenManifest
		cloned.FrozenManifest = &manifest
	}
	if s.Receipt != nil {
		receipt := *s.Receipt
		cloned.Receipt = &receipt
	}
	return &cloned
}

func FindDiscrepancy(s *Submission, id string) (*Discrepancy, error) {
	for i := range s.Discrepancies {
		if s.Discrepancies[i].DiscrepancyID == id {
			return &s.Discrepancies[i], nil
		}
	}
	return nil, NewError("discrepancy_not_found", "差异项不存在")
}
