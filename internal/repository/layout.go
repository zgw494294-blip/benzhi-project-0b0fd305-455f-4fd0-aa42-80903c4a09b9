package repository

import (
	"fmt"
	"path/filepath"
)

type layout struct{ root string }

func (l layout) objectsDir() string             { return filepath.Join(l.root, "objects") }
func (l layout) submissionsDir() string         { return filepath.Join(l.root, "submissions") }
func (l layout) idempotencyDir() string         { return filepath.Join(l.root, "idempotency") }
func (l layout) submissionDir(id string) string { return filepath.Join(l.submissionsDir(), id) }
func (l layout) generation(id string, version uint64) string {
	return filepath.Join(l.submissionDir(id), fmt.Sprintf("snapshot-%020d.json", version))
}
func (l layout) index(id string) string { return filepath.Join(l.submissionDir(id), "CURRENT") }
func (l layout) object(digest string) string {
	return filepath.Join(l.objectsDir(), digest[:2], digest)
}
func (l layout) idempotency(keyDigest string) string {
	return filepath.Join(l.idempotencyDir(), keyDigest[:2], keyDigest+".json")
}
