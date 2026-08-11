// Package audit owns Rostrum's independent, append-only operational ledger.
// It deliberately lives beside the workspace store rather than inside the
// mutable State aggregate, so a restore or bad aggregate rewrite cannot erase
// the history of governed mutations that preceded it.
package audit

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

const defaultSegmentBytes int64 = 10 * 1024 * 1024

// Event is the secret-free input an application mutation supplies to the
// independent ledger. Detail must contain concise operational facts rather
// than request bodies, credentials, free-form reviewer comments, or uploads.
type Event struct {
	At      time.Time
	Kind    string
	ActorID string
	Subject string
	Rule    string
	Trace   []string
	Detail  map[string]string
}

// Record is the durable JSON Lines wire format. Hash binds every field,
// including PrevHash, into a tamper-evident chain across rotated segments.
type Record struct {
	Seq      uint64            `json:"seq"`
	At       time.Time         `json:"at"`
	Kind     string            `json:"kind"`
	ActorID  string            `json:"actorId"`
	Subject  string            `json:"subject"`
	Rule     string            `json:"rule,omitempty"`
	Trace    []string          `json:"trace,omitempty"`
	Detail   map[string]string `json:"detail,omitempty"`
	PrevHash string            `json:"prevHash"`
	Hash     string            `json:"hash"`
}

// Log serializes fsync-backed appends to one active JSON Lines segment. Once
// it reaches segmentBytes it is renamed, retained, and a fresh active segment
// is opened; verification always walks both old and active segments.
type Log struct {
	mu           sync.Mutex
	path         string
	segmentBytes int64
	file         *os.File
	seq          uint64
	previousHash string
}

// Open verifies existing segments before allowing another append. A corrupt
// or hand-edited ledger fails closed at startup rather than silently starting
// a new chain beside it.
func Open(path string) (*Log, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("audit log requires a path")
	}
	path = filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create audit log directory: %w", err)
	}
	records, err := readAll(path)
	if err != nil {
		return nil, err
	}
	if err := verify(records); err != nil {
		return nil, err
	}
	log := &Log{path: path, segmentBytes: defaultSegmentBytes}
	if len(records) > 0 {
		last := records[len(records)-1]
		log.seq = last.Seq
		log.previousHash = last.Hash
	}
	if err := log.openActive(); err != nil {
		return nil, err
	}
	return log, nil
}

// FromMeta translates the common store mutation metadata into a ledger event.
// It intentionally carries only audit-safe summaries and policy information.
func FromMeta(meta domain.AuditMeta) Event {
	detail := map[string]string{}
	if meta.Summary != "" {
		detail["summary"] = meta.Summary
	}
	if meta.Origin != "" {
		detail["origin"] = meta.Origin
	}
	trace := []string(nil)
	if value := strings.TrimSpace(meta.Trace); value != "" {
		trace = strings.Split(value, "; ")
	}
	return Event{
		At:      meta.At,
		Kind:    meta.Action,
		ActorID: meta.Actor,
		Subject: meta.EntityID,
		Rule:    meta.Rule,
		Trace:   trace,
		Detail:  detail,
	}
}

// Append writes and fsyncs exactly one JSON Lines record. It returns only
// after the record is durable on the local filesystem.
func (log *Log) Append(event Event) (Record, error) {
	if log == nil {
		return Record{}, fmt.Errorf("audit log is not configured")
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	if log.file == nil {
		return Record{}, fmt.Errorf("audit log is closed")
	}
	if err := log.rotateLocked(); err != nil {
		return Record{}, err
	}
	at := event.At.UTC()
	if at.IsZero() {
		at = time.Now().UTC()
	}
	record := Record{
		Seq:      log.seq + 1,
		At:       at,
		Kind:     auditText(event.Kind, 120),
		ActorID:  auditText(event.ActorID, 160),
		Subject:  auditText(event.Subject, 160),
		Rule:     auditText(event.Rule, 160),
		Trace:    cloneStrings(event.Trace, 160, 500),
		Detail:   cloneDetail(event.Detail),
		PrevHash: log.previousHash,
	}
	if record.Kind == "" {
		record.Kind = "state.updated"
	}
	computed, err := record.computedHash()
	if err != nil {
		return Record{}, err
	}
	record.Hash = computed
	encoded, err := json.Marshal(record)
	if err != nil {
		return Record{}, fmt.Errorf("encode audit record: %w", err)
	}
	if _, err := log.file.Write(append(encoded, '\n')); err != nil {
		return Record{}, fmt.Errorf("append audit record: %w", err)
	}
	if err := log.file.Sync(); err != nil {
		return Record{}, fmt.Errorf("sync audit record: %w", err)
	}
	log.seq = record.Seq
	log.previousHash = record.Hash
	return record, nil
}

// Records returns the complete verified ledger, including rotated segments.
func (log *Log) Records() ([]Record, error) {
	if log == nil {
		return nil, fmt.Errorf("audit log is not configured")
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	records, err := readAll(log.path)
	if err != nil {
		return nil, err
	}
	if err := verify(records); err != nil {
		return nil, err
	}
	return cloneRecords(records), nil
}

// Verify exposes a cheap operator and test health check without returning the
// record contents themselves.
func (log *Log) Verify() error {
	_, err := log.Records()
	return err
}

func (log *Log) Close() error {
	if log == nil {
		return nil
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	if log.file == nil {
		return nil
	}
	err := log.file.Close()
	log.file = nil
	return err
}

func (log *Log) openActive() error {
	file, err := os.OpenFile(log.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	log.file = file
	return nil
}

func (log *Log) rotateLocked() error {
	info, err := log.file.Stat()
	if err != nil {
		return fmt.Errorf("stat audit log: %w", err)
	}
	if info.Size() < log.segmentBytes {
		return nil
	}
	if err := log.file.Close(); err != nil {
		return fmt.Errorf("close audit segment: %w", err)
	}
	log.file = nil
	rotated := segmentPath(log.path, time.Now().UTC())
	if err := os.Rename(log.path, rotated); err != nil {
		return fmt.Errorf("rotate audit segment: %w", err)
	}
	if err := log.openActive(); err != nil {
		return err
	}
	return nil
}

func readAll(path string) ([]Record, error) {
	paths, err := segmentPaths(path)
	if err != nil {
		return nil, err
	}
	records := make([]Record, 0)
	for _, candidate := range paths {
		file, err := os.Open(candidate)
		if err != nil {
			return nil, fmt.Errorf("open audit segment %s: %w", candidate, err)
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
		line := 0
		for scanner.Scan() {
			line++
			if strings.TrimSpace(scanner.Text()) == "" {
				continue
			}
			var record Record
			if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
				_ = file.Close()
				return nil, fmt.Errorf("decode audit segment %s line %d: %w", candidate, line, err)
			}
			records = append(records, record)
		}
		if err := scanner.Err(); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("read audit segment %s: %w", candidate, err)
		}
		if err := file.Close(); err != nil {
			return nil, fmt.Errorf("close audit segment %s: %w", candidate, err)
		}
	}
	return records, nil
}

func segmentPaths(path string) ([]string, error) {
	directory := filepath.Dir(path)
	base := filepath.Base(path)
	extension := filepath.Ext(base)
	stem := strings.TrimSuffix(base, extension)
	segments, err := filepath.Glob(filepath.Join(directory, stem+"-*"+extension))
	if err != nil {
		return nil, fmt.Errorf("find audit segments: %w", err)
	}
	sort.Strings(segments)
	if _, err := os.Stat(path); err == nil {
		segments = append(segments, path)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat audit log: %w", err)
	}
	return segments, nil
}

func segmentPath(path string, at time.Time) string {
	directory := filepath.Dir(path)
	base := filepath.Base(path)
	extension := filepath.Ext(base)
	stem := strings.TrimSuffix(base, extension)
	return filepath.Join(directory, stem+"-"+at.UTC().Format("20060102T150405.000000000Z")+extension)
}

func verify(records []Record) error {
	previous := ""
	for index, record := range records {
		if record.Seq != uint64(index+1) {
			return fmt.Errorf("audit record %d has sequence %d, want %d", index, record.Seq, index+1)
		}
		if record.At.IsZero() || strings.TrimSpace(record.Kind) == "" {
			return fmt.Errorf("audit record %d is incomplete", index)
		}
		if record.PrevHash != previous {
			return fmt.Errorf("audit record %d has an invalid previous hash", index)
		}
		computed, err := record.computedHash()
		if err != nil {
			return err
		}
		if record.Hash != computed {
			return fmt.Errorf("audit record %d has an invalid hash", index)
		}
		previous = record.Hash
	}
	return nil
}

func (record Record) computedHash() (string, error) {
	canonical := struct {
		Seq      uint64            `json:"seq"`
		At       time.Time         `json:"at"`
		Kind     string            `json:"kind"`
		ActorID  string            `json:"actorId"`
		Subject  string            `json:"subject"`
		Rule     string            `json:"rule,omitempty"`
		Trace    []string          `json:"trace,omitempty"`
		Detail   map[string]string `json:"detail,omitempty"`
		PrevHash string            `json:"prevHash"`
	}{
		Seq: record.Seq, At: record.At.UTC(), Kind: record.Kind, ActorID: record.ActorID,
		Subject: record.Subject, Rule: record.Rule, Trace: record.Trace, Detail: record.Detail, PrevHash: record.PrevHash,
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("encode audit hash payload: %w", err)
	}
	sum := sha256.Sum256(append([]byte(record.PrevHash+"\x1f"), encoded...))
	return hex.EncodeToString(sum[:]), nil
}

func auditText(value string, limit int) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, value)
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

func cloneStrings(values []string, itemLimit, totalLimit int) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	remaining := totalLimit
	for _, value := range values {
		value = auditText(value, itemLimit)
		if value == "" || remaining <= 0 {
			continue
		}
		runes := []rune(value)
		if len(runes) > remaining {
			value = string(runes[:remaining])
		}
		result = append(result, value)
		remaining -= len([]rune(value))
	}
	return result
}

func cloneDetail(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		key = auditText(key, 80)
		value = auditText(value, 500)
		if key != "" && value != "" {
			result[key] = value
		}
	}
	return result
}

func cloneRecords(records []Record) []Record {
	result := make([]Record, len(records))
	for index, record := range records {
		record.Trace = append([]string(nil), record.Trace...)
		record.Detail = cloneDetail(record.Detail)
		result[index] = record
	}
	return result
}
