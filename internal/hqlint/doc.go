// Package hqlint holds the structural lint for the hq/ headquarters layout.
//
// It is a test-only package: every check runs under `go test ./...` (and so
// under `make test` and the commit gate), and there is no runtime surface. The
// checks enforce the invariants promised in hq/00-GENESIS/how-we-work.md and
// constitution.md — the five areas and their READMEs exist, research topics
// carry a legal non-terminal state, journey episodes are contiguously numbered,
// indexed, and each records its Reversal condition, the speckit constitution
// symlink resolves into GENESIS, and relative markdown links inside hq/ resolve.
package hqlint
