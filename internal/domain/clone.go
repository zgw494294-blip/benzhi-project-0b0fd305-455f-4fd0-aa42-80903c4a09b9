package domain

import "encoding/json"

func CloneSubmission(s *Submission) *Submission {
	if s == nil {
		return nil
	}
	b, _ := json.Marshal(s)
	var copy Submission
	_ = json.Unmarshal(b, &copy)
	return &copy
}

func FindDiscrepancy(s *Submission, id string) (*Discrepancy, error) {
	for i := range s.Discrepancies {
		if s.Discrepancies[i].DiscrepancyID == id {
			return &s.Discrepancies[i], nil
		}
	}
	return nil, NewError("discrepancy_not_found", "差异项不存在")
}
