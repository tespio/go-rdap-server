package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"
)

const clientIPKey ctxKey = "client_ip"

// TrustedProxyClientIP returns middleware that resolves the real client IP in a
// proxy-safe way. It only trusts the X-Forwarded-For / X-Real-IP headers when
// the direct network peer is in the trusted proxy list; otherwise it ignores
// those headers entirely and uses the socket peer address.
//
// This prevents an arbitrary Internet client from spoofing its IP via headers
// to manipulate per-IP rate limiting or access logs.
func TrustedProxyClientIP(trustedCIDRs []string) func(http.Handler) http.Handler {
	trusted := parseCIDRs(trustedCIDRs)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := peerIP(r)
			if ipAllowed(clientIP, trusted) {
				if xff := forwardedFor(r); xff != "" {
					clientIP = xff
				} else if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
					clientIP = xrip
				}
			}
			r.RemoteAddr = net.JoinHostPort(clientIP, remotePort(r))
			ctx := context.WithValue(r.Context(), clientIPKey, clientIP)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetClientIP returns the resolved client IP from the request context, falling
// back to the socket peer if not present.
func GetClientIP(r *http.Request) string {
	if ip, ok := r.Context().Value(clientIPKey).(string); ok && ip != "" {
		return ip
	}
	return peerIP(r)
}

// peerIP extracts the host portion of r.RemoteAddr without trusting any headers.
func peerIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func remotePort(r *http.Request) string {
	_, port, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return ""
	}
	return port
}

// forwardedFor returns the first (client-most) address in X-Forwarded-For.
// Callers must only use this after confirming the direct peer is trusted.
func forwardedFor(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return ""
	}
	parts := strings.Split(xff, ",")
	first := strings.TrimSpace(parts[0])
	if first == "" {
		return ""
	}
	return first
}

func parseCIDRs(cidrs []string) []*net.IPNet {
	var out []*net.IPNet
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if ip := net.ParseIP(c); ip != nil {
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			out = append(out, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		if _, ipnet, err := net.ParseCIDR(c); err == nil {
			out = append(out, ipnet)
		}
	}
	return out
}

func ipAllowed(ip string, trusted []*net.IPNet) bool {
	if len(trusted) == 0 {
		return false
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, ipnet := range trusted {
		if ipnet.Contains(parsed) {
			return true
		}
	}
	return false
}
