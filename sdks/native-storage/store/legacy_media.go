package nativestore

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type legacyRoot struct{ declared, canonical string }

// Only the native OS broker can set these owned application roots. External
// Downloads/original gallery files are not inferred to be application-owned.
func (s *Store) SetLegacyRoots(roots []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.authorizedLocked() {
		return ErrLocked
	}
	var checked []legacyRoot
	for _, root := range roots {
		if !filepath.IsAbs(root) {
			return errors.New("invalid native legacy root")
		}
		clean := filepath.Clean(root)
		if clean == string(filepath.Separator) {
			return errors.New("invalid native legacy root")
		}
		canonical, err := filepath.EvalSymlinks(clean)
		if os.IsNotExist(err) {
			continue // Native callers may name historical directories not present.
		}
		if err != nil {
			return errors.New("invalid native legacy root")
		}
		parent, parentErr := filepath.EvalSymlinks(filepath.Dir(clean))
		if parentErr != nil || canonical != filepath.Join(parent, filepath.Base(clean)) {
			return errors.New("legacy root leaf must not be a symlink")
		}
		info, err := os.Stat(canonical)
		if err != nil || !info.IsDir() {
			return errors.New("invalid native legacy root")
		}
		checked = append(checked, legacyRoot{clean, canonical})
	}
	s.legacyRoots = checked
	return nil
}
func containedBy(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !filepath.IsAbs(relative) && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
func (s *Store) legacyDeclaredPath(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	path = filepath.Clean(path)
	s.mu.Lock()
	roots := append([]legacyRoot{}, s.legacyRoots...)
	s.mu.Unlock()
	for _, root := range roots {
		if containedBy(path, root.declared) {
			return true
		}
	}
	return false
}
func (s *Store) legacyOwned(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	path = filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	s.mu.Lock()
	roots := append([]legacyRoot{}, s.legacyRoots...)
	quarantine := s.quarantineRoot
	s.mu.Unlock()
	// A broad legacy Library root must not include this or another account's
	// protected vault. Only the explicitly declared root alias may resolve links.
	vault := s.root
	if filepath.Base(filepath.Dir(vault)) == "accounts" {
		vault = filepath.Dir(filepath.Dir(vault))
	}
	if canonical, evalErr := filepath.EvalSymlinks(vault); evalErr == nil {
		vault = canonical
	}
	if containedBy(resolved, vault) || (quarantine != "" && containedBy(resolved, quarantine)) {
		return false
	}
	for _, root := range roots {
		relative, relErr := filepath.Rel(root.declared, path)
		if relErr == nil && containedBy(path, root.declared) && resolved == filepath.Join(root.canonical, relative) {
			return true
		}
	}
	return false
}

// MigrateLegacyMedia walks bounded SQLCipher pages after the legacy DB export.
// Only the current sender's app-owned source paths are candidates: a received
// message cannot trick this importer into reading a local application file.
// The encrypted mapping survives a crash after unlink, before all shared
// message references have been rewritten.
func (s *Store) MigrateLegacyMedia() error {
	rows, err := s.db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'chat_logs_%'`)
	if err != nil {
		return err
	}
	var tables []string
	for rows.Next() {
		var table string
		if err = rows.Scan(&table); err != nil {
			_ = rows.Close()
			return err
		}
		tables = append(tables, table)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for _, table := range tables {
		q, err := quoteIdentifier(table)
		if err != nil {
			return err
		}
		after := ""
		for {
			rows, err = s.db.Query(`SELECT client_msg_id,content,ex FROM `+q+` WHERE client_msg_id>? AND send_id=? AND status<4 AND content_type IN(102,103,104,105) ORDER BY client_msg_id LIMIT 100`, after, s.account)
			if err != nil {
				return err
			}
			type message struct{ id, content, extra string }
			var batch []message
			for rows.Next() {
				var m message
				if err = rows.Scan(&m.id, &m.content, &m.extra); err != nil {
					_ = rows.Close()
					return err
				}
				batch = append(batch, m)
			}
			err = rows.Err()
			_ = rows.Close()
			if err != nil {
				return err
			}
			if len(batch) == 0 {
				break
			}
			for _, m := range batch {
				after = m.id
				var body map[string]json.RawMessage
				if json.Unmarshal([]byte(m.content), &body) != nil {
					continue
				}
				var extra map[string]json.RawMessage
				_ = json.Unmarshal([]byte(m.extra), &extra)
				changed := false
				for _, field := range []string{"sourcePath", "soundPath", "videoPath", "snapshotPath", "filePath"} {
					var source string
					if json.Unmarshal(body[field], &source) != nil || source == "" {
						continue
					}
					if _, _, registered := s.outgoingIdentity(source); registered == nil {
						continue
					}
					var encrypted, name string
					// Portable native backups retain basenames/IDs while the OS sandbox path
					// changes. Resolve only a known encrypted outgoing row owned by this account.
					if filepath.Base(filepath.Dir(source)) == "outgoing" {
						candidate := filepath.Join(s.root, "outgoing", filepath.Base(source))
						if _, n, registered := s.outgoingIdentity(candidate); registered == nil {
							body[field], _ = json.Marshal(candidate)
							changed = true
							name = n
							if err = s.BindOutgoing(m.id, candidate); err != nil {
								return err
							}
							continue
						}
					}

					mapped := s.db.QueryRow(`SELECT protected_path,source_name FROM puty_legacy_media WHERE source=?`, source).Scan(&encrypted, &name) == nil
					if mapped {
						encrypted = filepath.Join(s.root, "outgoing", filepath.Base(encrypted))
						if _, _, err = s.outgoingIdentity(encrypted); err != nil {
							return err
						}
					}
					if !mapped {
						var input io.ReadCloser
						if s.legacyOwned(source) {
							info, statErr := os.Lstat(source)
							if statErr != nil || !info.Mode().IsRegular() {
								continue
							}
							name = info.Name()
							input, err = os.Open(source)
						} else if s.legacyDeclaredPath(source) {
							// Another account may have sealed this unknown source earlier.
							// Only this actor's own persisted sent-message reference reaches here.
							var metadata legacyMetadata
							input, metadata, err = s.openQuarantinedSource(source, "file", "")
							if os.IsNotExist(err) {
								continue
							}
							name = filepath.Base(metadata.Source)
						} else {
							continue
						}
						if err != nil {
							return err
						}
						var original string
						if json.Unmarshal(body["fileName"], &original) == nil && original != "" {
							name = original
						}
						if extra != nil && json.Unmarshal(extra["putySourceName"], &original) == nil && original != "" {
							name = original
						}
						encrypted, err = s.StageOutgoing(input, name)
						_ = input.Close()
						if err != nil {
							return err
						}
						if _, err = s.db.Exec(`INSERT INTO puty_legacy_media VALUES(?,?,?)`, source, encrypted, name); err != nil {
							return err
						}
					}
					if err = s.BindOutgoing(m.id, encrypted); err != nil {
						return err
					}
					body[field], _ = json.Marshal(encrypted)
					changed = true
					if field != "snapshotPath" && field != "filePath" && (m.extra == "" || extra != nil) {
						if extra == nil {
							extra = map[string]json.RawMessage{}
						}
						extra["putySourceName"], _ = json.Marshal(name)
					}
					if s.legacyOwned(source) {
						if err = os.Remove(source); err != nil && !os.IsNotExist(err) {
							return err
						}
						if err = syncDirectory(filepath.Dir(source)); err != nil {
							return err
						}
					}
				}
				if changed {
					encoded, _ := json.Marshal(body)
					extraBytes := []byte(m.extra)
					if extra != nil {
						extraBytes, _ = json.Marshal(extra)
					}
					if _, err = s.db.Exec(`UPDATE `+q+` SET content=?,ex=? WHERE client_msg_id=?`, string(encoded), string(extraBytes), m.id); err != nil {
						return err
					}
				}
			}
		}
	}
	return s.checkAuthorized()
}
