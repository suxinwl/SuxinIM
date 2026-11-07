package imbridge

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
)

// IsDirectoryAvatarResource is an exact allowlist over current HRM directory
// projections. It never reads the filesystem or exposes general HRM uploads.
func (s *SQLStore) IsDirectoryAvatarResource(ctx context.Context, resource string) (bool, error) {
	if len(resource) == 0 || len(resource) > 4096 {
		return false, nil
	}
	requested, err := url.ParseRequestURI(resource)
	if err != nil || requested.Scheme != "" || requested.Host != "" || requested.User != nil || requested.Fragment != "" || !strings.HasPrefix(requested.Path, "/resource/uploads/") {
		return false, nil
	}
	for _, segment := range strings.Split(requested.Path, "/") {
		if segment == "." || segment == ".." {
			return false, nil
		}
	}
	if strings.ContainsAny(requested.Path, "\\\x00\r\n") || resource != requested.EscapedPath()+avatarQuery(requested) {
		return false, nil
	}
	origin, err := url.Parse(s.PublicURL)
	if err != nil || origin.User != nil || origin.Host == "" || origin.Scheme != "https" && origin.Scheme != "http" {
		return false, nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT projection_json FROM biz_openim_employee WHERE enabled=1 AND desired_version=applied_version`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return false, err
		}
		var projection Projection
		if json.Unmarshal([]byte(raw), &projection) != nil {
			continue
		}
		avatar, e := url.Parse(projection.Profile.FaceURL)
		if e != nil || avatar.User != nil || avatar.Fragment != "" || !strings.EqualFold(avatar.Scheme, origin.Scheme) || !strings.EqualFold(avatar.Host, origin.Host) {
			continue
		}
		if avatar.EscapedPath()+avatarQuery(avatar) == resource {
			return true, nil
		}
	}
	return false, rows.Err()
}

func avatarQuery(u *url.URL) string {
	if u.RawQuery != "" || u.ForceQuery {
		return "?" + u.RawQuery
	}
	return ""
}
