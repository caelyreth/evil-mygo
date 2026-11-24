//go:build !darwin

package text

// glyphPath gives no outline: only Core Text's is read yet.
func glyphPath(*Font, uint32) []PathSeg { return nil }
