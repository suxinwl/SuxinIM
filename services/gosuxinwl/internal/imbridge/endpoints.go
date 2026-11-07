package imbridge

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type Endpoints struct{ HRM, IM string }
type endpointsContextKey struct{}

func profileForEndpoint(ctx context.Context, profile Profile) Profile {
	pair, ok := ctx.Value(endpointsContextKey{}).(Endpoints)
	if !ok {
		return profile
	}
	avatar, err := url.Parse(profile.FaceURL)
	if err != nil {
		return profile
	}
	for _, known := range []Endpoints{PublicEndpoints(), LANEndpoints()} {
		origin, _ := url.Parse(known.HRM)
		if avatar.Scheme == origin.Scheme && strings.EqualFold(avatar.Host, origin.Host) {
			target, _ := url.Parse(pair.HRM)
			avatar.Scheme, avatar.Host = target.Scheme, target.Host
			profile.FaceURL = avatar.String()
			break
		}
	}
	return profile
}

func endpointEnv(key, fallback string) string {
	if value := strings.TrimRight(os.Getenv(key), "/"); value != "" {
		return value
	}
	return fallback
}

func PublicEndpoints() Endpoints {
	return Endpoints{endpointEnv("GOSUXIN_HRM_PUBLIC_URL", "https://hr.szputy.com:9880"), endpointEnv("GOSUXIN_IM_PUBLIC_URL", "https://im.szputy.com:9885")}
}
func LANEndpoints() Endpoints {
	return Endpoints{endpointEnv("GOSUXIN_HRM_LAN_URL", "https://192.168.10.11:9880"), endpointEnv("GOSUXIN_IM_LAN_URL", "https://192.168.10.11:9885")}
}
func validEndpoint(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == ""
}

// SelectEndpoints uses only an exact configured Host, never forwarded headers or
// a caller supplied redirect. Both the host proxy and plugin preserve this Host.
func SelectEndpoints(r *http.Request) (Endpoints, error) {
	for _, pair := range []Endpoints{LANEndpoints(), PublicEndpoints()} {
		if !validEndpoint(pair.HRM) || !validEndpoint(pair.IM) {
			return Endpoints{}, errors.New("通讯入口配置无效")
		}
		for _, raw := range []string{pair.HRM, pair.IM} {
			u, _ := url.Parse(raw)
			if strings.EqualFold(r.Host, u.Host) {
				return pair, nil
			}
		}
	}
	return Endpoints{}, errors.New("通讯入口不在可信主机白名单中")
}
func ticketURL(r *http.Request, ticket string) (string, error) {
	pair, err := SelectEndpoints(r)
	if err != nil {
		return "", err
	}
	return pair.IM + "/?ticket=" + url.QueryEscape(ticket) + "#/login", nil
}
