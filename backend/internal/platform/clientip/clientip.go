// Package clientip resolves the client IP of a request behind a reverse proxy.
//
// The peer address (RemoteAddr) is the only value a client cannot forge, so it is
// the default. When the direct peer belongs to a configured list of trusted
// proxies (for example the nginx container), the proxy is believed and the
// address it puts in the X-Real-IP header is used instead. With an empty list
// nothing is trusted and forwarding headers are ignored.
package clientip

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// Header is the request header a trusted proxy sets to the client address. The
// proxy must overwrite it (never append to or pass through a client value).
const Header = "X-Real-IP"

// Resolver decides which client IP a request is attributed to.
type Resolver struct {
	trusted []netip.Prefix
}

// NewResolver builds a Resolver that trusts the Header of peers inside trusted.
// A nil or empty list trusts nothing.
func NewResolver(trusted []netip.Prefix) *Resolver {
	return &Resolver{trusted: trusted}
}

// ParseTrusted parses a comma separated list of CIDRs and bare IPs (a bare IP
// is a single-address prefix). An empty string is an empty list. Prefixes that
// cover the whole address space are rejected: trusting every peer would let any
// client choose its own IP.
func ParseTrusted(raw string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		p, err := parsePrefix(item)
		if err != nil {
			return nil, fmt.Errorf("%q is not a CIDR or IP address", item)
		}
		if p.Bits() == 0 {
			return nil, fmt.Errorf("%q would trust every peer", item)
		}
		out = append(out, p)
	}
	return out, nil
}

func parsePrefix(s string) (netip.Prefix, error) {
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return netip.Prefix{}, err
		}
		return normalizePrefix(p), nil
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	a = normalize(a)
	return netip.PrefixFrom(a, a.BitLen()), nil
}

// normalizePrefix turns an IPv4-mapped IPv6 prefix (::ffff:a.b.c.d/n, n >= 96)
// into the plain IPv4 prefix so it matches unmapped peers.
func normalizePrefix(p netip.Prefix) netip.Prefix {
	a := p.Addr()
	if a.Is4In6() && p.Bits() >= 96 {
		return netip.PrefixFrom(a.Unmap(), p.Bits()-96).Masked()
	}
	return p.Masked()
}

// normalize drops the IPv6 zone and unmaps IPv4-mapped IPv6 addresses so one
// client always has one canonical spelling (and one rate-limit bucket).
func normalize(a netip.Addr) netip.Addr { return a.WithZone("").Unmap() }

// IP returns the client IP of r as a canonical string. It falls back to the raw
// RemoteAddr when that cannot be parsed, as the previous behavior did.
func (r *Resolver) IP(req *http.Request) string {
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		host = req.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return req.RemoteAddr
	}
	peer = normalize(peer)
	if !r.isTrusted(peer) {
		return peer.String()
	}
	// Exactly one header line, holding exactly one address. Anything else (absent,
	// repeated, a comma list, garbage) is ambiguous: keep the proxy's own address.
	values := req.Header.Values(Header)
	if len(values) != 1 {
		return peer.String()
	}
	client, err := netip.ParseAddr(strings.TrimSpace(values[0]))
	if err != nil {
		return peer.String()
	}
	return normalize(client).String()
}

func (r *Resolver) isTrusted(peer netip.Addr) bool {
	for _, p := range r.trusted {
		if p.Contains(peer) {
			return true
		}
	}
	return false
}

type ctxKey struct{}

// Middleware resolves the client IP once and stores it in the request context.
func (r *Resolver) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := context.WithValue(req.Context(), ctxKey{}, r.IP(req))
		next.ServeHTTP(w, req.WithContext(ctx))
	})
}

// FromContext returns the client IP stored by Middleware.
func FromContext(ctx context.Context) (string, bool) {
	ip, ok := ctx.Value(ctxKey{}).(string)
	return ip, ok
}

// FromRequest returns the IP stored by Middleware and, without it, the peer
// address (no header is ever trusted by default).
func FromRequest(req *http.Request) string {
	if ip, ok := FromContext(req.Context()); ok {
		return ip
	}
	return NewResolver(nil).IP(req)
}
