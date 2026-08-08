package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

func NewInstallationID() (string, error) {
	return newInstallationID(rand.Reader)
}

func newInstallationID(reader io.Reader) (string, error) {
	var value [16]byte
	if _, err := io.ReadFull(reader, value[:]); err != nil {
		return "", fmt.Errorf("generate installation id: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}

func ComputeSourceBindingID(source SourceIdentity) string {
	canonical := strings.Join([]string{
		strings.TrimSpace(source.CanonicalSource),
		strings.TrimSpace(source.Repository),
		strings.TrimSpace(source.PackageSubpath),
	}, "\x00")
	sum := sha256.Sum256([]byte(canonical))
	return "src_" + hex.EncodeToString(sum[:12])
}

func ComputeClientBindingID(installationID, clientID, scope, targetLocator string) string {
	value := strings.Join([]string{installationID, clientID, scope, targetLocator}, "\x00")
	sum := sha256.Sum256([]byte(value))
	return "client_" + hex.EncodeToString(sum[:12])
}

func ComputePhysicalArtifactID(declaredName, installationID string) string {
	sum := sha256.Sum256([]byte(installationID))
	suffix := hex.EncodeToString(sum[:6])
	name := strings.TrimSpace(declaredName)
	maximumNameBytes := 64 - 1 - len(suffix)
	if len(name) > maximumNameBytes {
		name = strings.TrimRight(name[:maximumNameBytes], ".-")
	}
	return name + "-" + suffix
}
