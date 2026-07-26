package update

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/fulcio/certificate"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

const (
	provenanceRootVersion = "sigstore-public-good-2026-04-07"
	provenanceIssuer      = "https://token.actions.githubusercontent.com"
	provenanceRepository  = "LosFurina/tmuxatlas"
	provenanceWorkflow    = ".github/workflows/goreleaser.yml"
	provenanceRootSHA256  = "4364d7724c04cc912ce2a6c45ed2610e8d8d1c4dc857fb500292738d4d9c8d2c"
)

// Sigstore public-good trusted root, pinned on 2026-04-07. Rotation is an
// explicit source change. Compression keeps the otherwise verbose TUF JSON
// out of the installed filesystem while remaining available offline.
const compressedProvenanceRoot = "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"

func provenanceIdentity(tag string) (verify.CertificateIdentity, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return verify.CertificateIdentity{}, fmt.Errorf("release tag is empty")
	}
	ref := "refs/tags/" + tag
	workflowURI := fmt.Sprintf("https://github.com/%s/%s@%s", provenanceRepository, provenanceWorkflow, ref)
	san, err := verify.NewSANMatcher(workflowURI, "")
	if err != nil {
		return verify.CertificateIdentity{}, err
	}
	issuer, err := verify.NewIssuerMatcher(provenanceIssuer, "")
	if err != nil {
		return verify.CertificateIdentity{}, err
	}
	return verify.NewCertificateIdentity(san, issuer, certificate.Extensions{
		GithubWorkflowRepository: provenanceRepository,
		GithubWorkflowRef:        ref,
		BuildSignerURI:           workflowURI,
		SourceRepositoryURI:      "https://github.com/" + provenanceRepository,
		SourceRepositoryRef:      ref,
	})
}

func decodePinnedRoot() ([]byte, error) {
	compressed, err := base64.StdEncoding.DecodeString(compressedProvenanceRoot)
	if err != nil {
		return nil, fmt.Errorf("decode pinned trusted root %s: %w", provenanceRootVersion, err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("open pinned trusted root %s: %w", provenanceRootVersion, err)
	}
	raw, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read pinned trusted root %s: %w", provenanceRootVersion, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close pinned trusted root %s: %w", provenanceRootVersion, closeErr)
	}
	return raw, nil
}

func pinnedTrustedRoot() (*root.TrustedRoot, error) {
	raw, err := decodePinnedRoot()
	if err != nil {
		return nil, err
	}
	trusted, err := root.NewTrustedRootFromJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("parse pinned trusted root %s: %w", provenanceRootVersion, err)
	}
	return trusted, nil
}

func verifyChecksumBundle(checksumsPath, bundlePath, tag string) error {
	checksums, err := os.ReadFile(checksumsPath)
	if err != nil {
		return fmt.Errorf("read checksums for provenance verification: %w", err)
	}
	signedBundle, err := bundle.LoadJSONFromPath(bundlePath)
	if err != nil {
		return fmt.Errorf("load Sigstore checksum bundle: %w", err)
	}
	trusted, err := pinnedTrustedRoot()
	if err != nil {
		return err
	}
	identity, err := provenanceIdentity(tag)
	if err != nil {
		return err
	}
	verifier, err := verify.NewVerifier(trusted, verify.WithTransparencyLog(1), verify.WithIntegratedTimestamps(1))
	if err != nil {
		return fmt.Errorf("create Sigstore verifier: %w", err)
	}
	if _, err := verifier.Verify(signedBundle, verify.NewPolicy(
		verify.WithArtifact(bytes.NewReader(checksums)),
		verify.WithCertificateIdentity(identity),
	)); err != nil {
		return fmt.Errorf("verify checksums provenance with root %s: %w", provenanceRootVersion, err)
	}
	return nil
}
