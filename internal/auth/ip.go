package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
)

// ClientIP extracts the client IP honoring X-Forwarded-For when the request
// comes from a trusted proxy. trusted is a list of CIDRs.
func ClientIP(r *http.Request, trusted []string) string {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	remote := net.ParseIP(host)
	if remote == nil {
		remote = net.ParseIP(strings.TrimSpace(r.RemoteAddr))
	}
	xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if xff == "" {
		if remote != nil {
			return remote.String()
		}
		return ""
	}
	// Consider first hop in XFF chain only when remote is trusted
	if remote != nil && isTrusted(remote, trusted) {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if p := net.ParseIP(ip); p != nil {
				return p.String()
			}
		}
	}
	if remote != nil {
		return remote.String()
	}
	return ""
}

func isTrusted(ip net.IP, cidrs []string) bool {
	for _, c := range cidrs {
		_, netw, err := net.ParseCIDR(strings.TrimSpace(c))
		if err != nil || netw == nil {
			continue
		}
		if netw.Contains(ip) {
			return true
		}
	}
	return false
}

// HashIP returns hex(HMAC-SHA256(key, ip)). Returns empty string when ip or key is empty.
func HashIP(ip, key string) string {
	ip = strings.TrimSpace(ip)
	key = strings.TrimSpace(key)
	if ip == "" || key == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(ip))
	return hex.EncodeToString(mac.Sum(nil))
}
