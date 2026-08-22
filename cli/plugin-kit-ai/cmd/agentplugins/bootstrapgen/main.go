// Command bootstrapgen deterministically generates or verifies the production
// Directory bootstrap source from exact signed publication artifacts.
package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/777genius/plugin-kit-ai/cli/cmd/agentplugins/internal/bootstrapio"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/directoryv1"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "bootstrapgen:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	flags := flag.NewFlagSet("bootstrapgen", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	snapshotPath := flags.String("snapshot", "", "exact signed snapshot JSON")
	envelopePath := flags.String("envelope", "", "exact detached envelope JSON")
	trustPath := flags.String("trust", "", "reviewed trusted-keys JSON")
	outputPath := flags.String("output", "", "write generated source to this path (default stdout)")
	checkPath := flags.String("check", "", "require this generated source to reproduce exactly")
	releaseAtText := flags.String("release-at", "", "require publication usable at this RFC3339 release time")
	expectedKeyID := flags.String("expected-key-id", "", "require the envelope to bind this compiled production key ID")
	expectedPublicKey := flags.String("expected-public-key", "", "require this compiled production Ed25519 public key")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 || (*outputPath != "" && *checkPath != "") {
		return fmt.Errorf("unexpected arguments or conflicting -output/-check")
	}
	bundle, trust, err := bootstrapio.LoadVerifiedBundle(*snapshotPath, *envelopePath, *trustPath)
	if err != nil {
		return err
	}
	if bundle.Snapshot.Sequence == 0 {
		return fmt.Errorf("signed Directory bootstrap sequence is zero")
	}
	if (*expectedKeyID == "") != (*expectedPublicKey == "") {
		return fmt.Errorf("-expected-key-id and -expected-public-key must be supplied together")
	}
	if *expectedKeyID != "" {
		decoded, err := base64.StdEncoding.Strict().DecodeString(*expectedPublicKey)
		if err != nil || len(decoded) != ed25519.PublicKeySize {
			return fmt.Errorf("-expected-public-key is not an Ed25519 public key")
		}
		if bundle.Envelope.KeyID != *expectedKeyID {
			return fmt.Errorf("signed envelope key %q does not match compiled production key %q", bundle.Envelope.KeyID, *expectedKeyID)
		}
		matched := false
		for _, key := range trust.Keys {
			if key.ID == *expectedKeyID && bytes.Equal(key.PublicKey, decoded) {
				matched = true
			}
		}
		if !matched {
			return fmt.Errorf("trust input does not contain the exact compiled production key binding")
		}
	}
	if *releaseAtText != "" {
		releaseAt, err := time.Parse(time.RFC3339, *releaseAtText)
		if err != nil {
			return fmt.Errorf("parse -release-at: %w", err)
		}
		generatedAt, err := time.Parse(time.RFC3339, bundle.Snapshot.GeneratedAt)
		if err != nil {
			return fmt.Errorf("parse signed generated_at: %w", err)
		}
		expiresAt, err := time.Parse(time.RFC3339, bundle.Snapshot.ExpiresAt)
		if err != nil {
			return fmt.Errorf("parse signed expires_at: %w", err)
		}
		if releaseAt.Before(generatedAt) || !releaseAt.Before(expiresAt) {
			return fmt.Errorf("signed Directory bootstrap is not valid for release at %s (valid [%s, %s))", releaseAt.UTC().Format(time.RFC3339), generatedAt, expiresAt)
		}
	}
	source, err := directoryv1.GenerateReleaseBootstrapSource("main", "generatedProductionDirectoryBootstrap", bundle.SnapshotBytes, bundle.EnvelopeBytes, trust)
	if err != nil {
		return fmt.Errorf("generate verified Directory bootstrap: %w", err)
	}
	if *checkPath != "" {
		checked, err := os.ReadFile(*checkPath)
		if err != nil {
			return fmt.Errorf("read checked bootstrap source: %w", err)
		}
		if !bytes.Equal(source, checked) {
			return fmt.Errorf("%s is not reproducibly generated from the exact verified inputs", *checkPath)
		}
		return nil
	}
	if *outputPath != "" {
		if err := os.WriteFile(*outputPath, source, 0o644); err != nil {
			return fmt.Errorf("write generated bootstrap source: %w", err)
		}
		return nil
	}
	_, err = os.Stdout.Write(source)
	return err
}
