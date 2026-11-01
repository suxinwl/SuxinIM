package enterprise

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"golang.org/x/mod/semver"
)

type Application struct {
	ID         string `json:"id"`
	Platform   string `json:"platform"`
	Version    string `json:"version"`
	URL        string `json:"url"`
	Text       string `json:"text"`
	Force      bool   `json:"force"`
	Latest     bool   `json:"latest"`
	Hot        bool   `json:"hot"`
	CreateTime int64  `json:"createTime"`
}
type Artifact struct {
	Platform  string `json:"platform"`
	Version   string `json:"version"`
	Build     int64  `json:"build"`
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	Text      string `json:"text"`
	Force     bool   `json:"force"`
	Published bool   `json:"published"`
}
type Catalog interface {
	Latest(context.Context, string) (Application, error)
	Add(context.Context, Actor, Application) error
}
type GRPCCatalog struct{ Auth *GRPCAuth }

func (c *GRPCCatalog) Latest(ctx context.Context, platform string) (Application, error) {
	in := fieldString(nil, 2, platform)
	var out []byte
	err := c.Auth.chat.Invoke(ctx, "/openim.admin.admin/LatestApplicationVersion", &in, &out)
	if err != nil {
		return Application{}, err
	}
	f, err := wireFields(out)
	if err != nil {
		return Application{}, err
	}
	if len(f[1]) == 0 {
		return Application{}, ErrNotFound
	}
	v, err := wireFields(f[1][0])
	if err != nil {
		return Application{}, err
	}
	return Application{ID: strField(v, 1), Platform: strField(v, 2), Version: strField(v, 3), URL: strField(v, 4), Text: strField(v, 5), Force: intField(v, 6) != 0, Latest: intField(v, 7) != 0, Hot: intField(v, 8) != 0, CreateTime: intField(v, 9)}, nil
}
func (c *GRPCCatalog) Add(ctx context.Context, a Actor, v Application) error {
	in := fieldString(nil, 1, v.Platform)
	in = fieldString(in, 2, v.Version)
	in = fieldString(in, 3, v.URL)
	in = fieldString(in, 4, v.Text)
	if v.Force {
		in = fieldInt(in, 5, 1)
	}
	in = fieldInt(in, 6, 1)
	var out []byte
	return c.Auth.chat.Invoke(actorMetadata(ctx, a), "/openim.admin.admin/AddApplicationVersion", &in, &out)
}
func normalVersion(s string) (string, error) {
	v := "v" + strings.TrimPrefix(s, "v")
	if !semver.IsValid(v) || semver.Canonical(v) == "" {
		return "", bad("version must be semantic version MAJOR.MINOR.PATCH")
	}
	return v, nil
}
func artifactKey(platform, version string) string {
	return "artifact:" + platform + ":" + strings.TrimPrefix(version, "v")
}
func validPlatform(p string) bool {
	return p == "android" || p == "windows" || p == "ios" || p == "mac" || p == "linux"
}
func (s *Server) latest(ctx context.Context, q request) (any, error) {
	p := strings.ToLower(q.str("platform"))
	if !validPlatform(p) {
		return nil, bad("unsupported platform")
	}
	current, err := normalVersion(q.str("version"))
	if err != nil {
		return nil, err
	}
	if s.Catalog == nil {
		return map[string]any{"available": false}, nil
	}
	up, err := s.Catalog.Latest(ctx, p)
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			slog.Warn("update catalog unavailable", "error", err)
		}
		return map[string]any{"available": false}, nil
	}
	a, _, err := load[Artifact](ctx, s.Store, artifactKey(p, up.Version))
	if errors.Is(err, ErrNotFound) {
		return map[string]any{"available": false}, nil
	}
	if err != nil {
		return nil, err
	}
	if !a.Published || up.Hot || a.URL != up.URL || a.Version != up.Version {
		return map[string]any{"available": false}, nil
	}
	latest, err := normalVersion(a.Version)
	if err != nil {
		return nil, err
	}
	cmp := semver.Compare(latest, current)
	if cmp < 0 || cmp == 0 && a.Build <= q.num("build") {
		return map[string]any{"available": false}, nil
	}
	return map[string]any{"available": true, "version": a.Version, "build": a.Build, "url": a.URL, "sha256": a.SHA256, "size": a.Size, "text": up.Text, "force": up.Force, "platform": p}, nil
}
func (s *Server) publish(ctx context.Context, a Actor, q request) (any, error) {
	if !a.Admin {
		return nil, forbidden("administrator required")
	}
	var v Artifact
	if err := json.Unmarshal(mustJSON(q), &v); err != nil {
		return nil, bad("invalid artifact metadata")
	}
	v.Platform = strings.ToLower(v.Platform)
	if !validPlatform(v.Platform) {
		return nil, bad("unsupported platform")
	}
	version, err := normalVersion(v.Version)
	if err != nil {
		return nil, err
	}
	v.Version = strings.TrimPrefix(v.Version, "v")
	if v.Build <= 0 || v.Size <= 0 {
		return nil, bad("build and size must be positive")
	}
	hash, err := hex.DecodeString(v.SHA256)
	if err != nil || len(hash) != 32 {
		return nil, bad("sha256 must contain 64 hexadecimal characters")
	}
	v.SHA256 = strings.ToLower(v.SHA256)
	u, err := url.Parse(v.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return nil, bad("download URL must use HTTPS without embedded credentials")
	}
	if s.Catalog == nil {
		return nil, fmt.Errorf("Chat application catalog unavailable")
	}
	// The artifact record is staged first. An interrupted publication cannot make
	// a force update visible without a checksum/size record.
	key := artifactKey(v.Platform, v.Version)
	old, d, err := load[Artifact](ctx, s.Store, key)
	if err == nil {
		if old.URL != v.URL || old.SHA256 != v.SHA256 || old.Size != v.Size || old.Build != v.Build {
			return nil, conflict("published artifacts are immutable; use a new semantic version")
		}
		if old.Published {
			return map[string]any{"artifact": old}, nil
		}
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	} else {
		d = document(key, "artifact", v.Platform, v)
	}
	latest, e := s.Catalog.Latest(ctx, v.Platform)
	if e != nil && !errors.Is(e, ErrNotFound) {
		return nil, e
	}
	already := e == nil && latest.Version == v.Version && latest.URL == v.URL
	if e == nil && !already {
		lv, err := normalVersion(latest.Version)
		if err != nil {
			return nil, err
		}
		if semver.Compare(version, lv) <= 0 {
			return nil, conflict("new version must be greater than the current release")
		}
		prev, _, err := load[Artifact](ctx, s.Store, artifactKey(v.Platform, latest.Version))
		if err == nil && v.Build <= prev.Build {
			return nil, conflict("build must increase across releases")
		}
	}
	if d.Version == 0 {
		v.Published = false
		d.Data, _ = json.Marshal(v)
		if err = s.Store.Save(ctx, d, 0); err != nil {
			return nil, err
		}
	}
	if !already {
		if err = s.Catalog.Add(ctx, a, Application{Platform: v.Platform, Version: v.Version, URL: v.URL, Text: v.Text, Force: v.Force, Latest: true, Hot: false}); err != nil {
			return nil, err
		}
	}
	latest, err = s.Catalog.Latest(ctx, v.Platform)
	if err != nil {
		return nil, err
	}
	if latest.Version != v.Version || latest.URL != v.URL || latest.Hot {
		return nil, conflict("Chat did not activate the staged version")
	}
	v, err = mutate[Artifact](ctx, s.Store, key, func(v *Artifact) error { v.Published = true; return nil })
	return map[string]any{"artifact": v, "chatVersionID": latest.ID}, err
}
