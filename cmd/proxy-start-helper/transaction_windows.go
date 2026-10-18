//go:build windows

package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// managedMarker is deliberately self-describing. A marker from a different
// helper session cannot authorize overwriting a live engine configuration.
// The configuration itself remains the source of truth; the marker only
// proves that this helper wrote the current digest for the fixed service.
type managedMarker struct {
	Version    int    `json:"version"`
	SessionID  string `json:"sessionId"`
	EnginePath string `json:"enginePath"`
	Service    string `json:"service"`
	ConfigHash string `json:"configSha256"`
}

type configSnapshot struct {
	config       []byte
	configExists bool
	marker       []byte
	markerExists bool
	serviceRun   bool
}

func newSessionID() (string, error) {
	var value [24]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func digestBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func (h *helper) markerFor(data []byte) ([]byte, error) {
	engine, err := filepath.Abs(h.engine)
	if err != nil {
		return nil, err
	}
	marker := managedMarker{
		Version: managedMarkerVersion, SessionID: h.sessionID,
		EnginePath: filepath.Clean(engine), Service: expectedServiceName,
		ConfigHash: digestBytes(data),
	}
	encoded, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func (h *helper) readSnapshot() (configSnapshot, error) {
	var snapshot configSnapshot
	config, err := os.ReadFile(h.configPath)
	if err == nil {
		snapshot.configExists = true
		snapshot.config = append([]byte(nil), config...)
	} else if !errors.Is(err, os.ErrNotExist) {
		return snapshot, err
	}
	marker, err := os.ReadFile(h.markerPath)
	if err == nil {
		snapshot.markerExists = true
		snapshot.marker = append([]byte(nil), marker...)
	} else if !errors.Is(err, os.ErrNotExist) {
		return snapshot, err
	}
	running, err := h.queryService()
	if err != nil {
		return snapshot, err
	}
	snapshot.serviceRun = running
	logPath := filepath.Join(filepath.Dir(h.engine), "logs", "logfile.txt")
	if info, statErr := os.Stat(logPath); statErr == nil {
		h.logOffset = info.Size()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return snapshot, statErr
	} else {
		h.logOffset = 0
	}
	return snapshot, nil
}

func (h *helper) beginTransaction(snapshot configSnapshot, desired []byte) error {
	if h.transactionActive {
		return nil
	}
	if snapshot.configExists && !bytesEqual(snapshot.config, desired) {
		if !h.markerBelongsToSession(snapshot.marker, snapshot.config) {
			return fmt.Errorf("%s: existing app-config.json is not managed by this Proxy Start session", errConfigurationConflict)
		}
	}
	if snapshot.markerExists && !snapshot.configExists {
		return fmt.Errorf("%s: managed marker has no matching engine configuration", errConfigurationConflict)
	}
	// Capture the baseline in memory exactly once. Never create a public .bak
	// containing proxy credentials on behalf of the launcher.
	h.originalConfigSeen = snapshot.configExists
	h.originalConfig = append([]byte(nil), snapshot.config...)
	h.originalMarkerSeen = snapshot.markerExists
	h.originalMarker = append([]byte(nil), snapshot.marker...)
	h.serviceWasRunning = snapshot.serviceRun
	h.transactionActive = true
	return nil
}

func (h *helper) markerBelongsToSession(data, config []byte) bool {
	if len(data) == 0 || len(config) == 0 {
		return false
	}
	var marker managedMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return false
	}
	engine, err := filepath.Abs(h.engine)
	if err != nil {
		return false
	}
	return marker.Version == managedMarkerVersion && marker.SessionID == h.sessionID &&
		strings.EqualFold(marker.Service, expectedServiceName) &&
		strings.EqualFold(filepath.Clean(marker.EnginePath), filepath.Clean(engine)) &&
		strings.EqualFold(marker.ConfigHash, digestBytes(config))
}

func (h *helper) restoreSnapshot(snapshot configSnapshot) error {
	var joined error
	if snapshot.configExists {
		if err := writeAtomicProtected(h.configPath, snapshot.config); err != nil {
			joined = errors.Join(joined, fmt.Errorf("restore engine configuration: %w", err))
		}
	} else if err := os.Remove(h.configPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		joined = errors.Join(joined, fmt.Errorf("remove temporary engine configuration: %w", err))
	}
	if snapshot.markerExists {
		if err := writeAtomicProtected(h.markerPath, snapshot.marker); err != nil {
			joined = errors.Join(joined, fmt.Errorf("restore managed marker: %w", err))
		}
	} else if err := os.Remove(h.markerPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		joined = errors.Join(joined, fmt.Errorf("remove temporary managed marker: %w", err))
	}
	return joined
}

func (h *helper) rollbackApply(ctx context.Context, snapshot configSnapshot) error {
	var joined error
	if err := h.restoreSnapshot(snapshot); err != nil {
		joined = errors.Join(joined, err)
	}
	if snapshot.serviceRun {
		if err := h.ensureServiceRunning(ctx); err != nil {
			joined = errors.Join(joined, fmt.Errorf("restore running service: %w", err))
		}
	} else {
		if err := h.ensureServiceStopped(ctx); err != nil {
			joined = errors.Join(joined, fmt.Errorf("restore stopped service: %w", err))
		}
	}
	return joined
}

func (h *helper) forgetTransaction() {
	h.transactionActive = false
	h.serviceOwned = false
	h.serviceWasRunning = false
	h.originalConfig = nil
	h.originalMarker = nil
	h.originalConfigSeen = false
	h.originalMarkerSeen = false
}

func bytesEqual(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
