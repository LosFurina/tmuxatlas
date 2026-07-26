package update

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/fulcio/certificate"
)

func TestPinnedTrustedRoot(t *testing.T) {
	trusted, err := pinnedTrustedRoot()
	if err != nil {
		t.Fatal(err)
	}
	if len(trusted.FulcioCertificateAuthorities()) == 0 || len(trusted.RekorLogs()) == 0 {
		t.Fatal("pinned root lacks Fulcio or Rekor trust material")
	}
	raw, err := decodePinnedRoot()
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if got := hex.EncodeToString(sum[:]); got != provenanceRootSHA256 {
		t.Fatalf("trusted root digest = %s, want %s", got, provenanceRootSHA256)
	}
}

func TestProvenanceIdentityPolicy(t *testing.T) {
	identity, err := provenanceIdentity("v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	valid := certificate.Summary{
		SubjectAlternativeName: "https://github.com/LosFurina/tmuxatlas/.github/workflows/goreleaser.yml@refs/tags/v1.2.3",
		Extensions: certificate.Extensions{
			Issuer: provenanceIssuer, GithubWorkflowRepository: provenanceRepository,
			GithubWorkflowRef:   "refs/tags/v1.2.3",
			BuildSignerURI:      "https://github.com/LosFurina/tmuxatlas/.github/workflows/goreleaser.yml@refs/tags/v1.2.3",
			SourceRepositoryURI: "https://github.com/LosFurina/tmuxatlas",
			SourceRepositoryRef: "refs/tags/v1.2.3",
		},
	}
	if err := identity.Verify(valid); err != nil {
		t.Fatalf("valid identity rejected: %v", err)
	}
	tests := map[string]func(*certificate.Summary){
		"issuer": func(s *certificate.Summary) { s.Extensions.Issuer = "https://issuer.example" },
		"repository": func(s *certificate.Summary) {
			s.Extensions.GithubWorkflowRepository = "attacker/tmuxatlas"
		},
		"workflow": func(s *certificate.Summary) {
			s.SubjectAlternativeName = "https://github.com/LosFurina/tmuxatlas/.github/workflows/release.yml@refs/tags/v1.2.3"
		},
		"tag": func(s *certificate.Summary) { s.Extensions.SourceRepositoryRef = "refs/tags/v9.9.9" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			invalid := valid
			mutate(&invalid)
			if err := identity.Verify(invalid); err == nil {
				t.Fatal("invalid identity was accepted")
			}
		})
	}
}

func TestVerifyChecksumBundleFailsClosed(t *testing.T) {
	dir := t.TempDir()
	checksums := filepath.Join(dir, "checksums.txt")
	if err := os.WriteFile(checksums, []byte("checksums"), 0o600); err != nil {
		t.Fatal(err)
	}
	malformed := filepath.Join(dir, "malformed.sigstore.json")
	if err := os.WriteFile(malformed, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, bundlePath := range map[string]string{
		"missing": filepath.Join(dir, "missing.sigstore.json"), "malformed": malformed,
	} {
		t.Run(name, func(t *testing.T) {
			if err := verifyChecksumBundle(checksums, bundlePath, "v1.2.3"); err == nil {
				t.Fatal("invalid bundle was accepted")
			}
		})
	}
}
