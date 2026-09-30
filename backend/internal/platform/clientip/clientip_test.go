package clientip

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func request(remote string, headers ...string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = remote
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Add(headers[i], headers[i+1])
	}
	return r
}

func mustResolver(t *testing.T, raw string) *Resolver {
	t.Helper()
	trusted, err := ParseTrusted(raw)
	if err != nil {
		t.Fatalf("ParseTrusted(%q): %v", raw, err)
	}
	return NewResolver(trusted)
}

func TestIP(t *testing.T) {
	tests := []struct {
		name    string
		trusted string
		req     *http.Request
		want    string
	}{
		{"default trusts nothing, header ignored", "", request("203.0.113.9:5555", Header, "198.51.100.7"), "203.0.113.9"},
		{"forwarded-for is never used", "10.0.0.0/8", request("10.1.2.3:80", "X-Forwarded-For", "198.51.100.7"), "10.1.2.3"},
		{"untrusted peer spoofing the header is ignored", "10.0.0.0/8", request("203.0.113.9:5555", Header, "198.51.100.7"), "203.0.113.9"},
		{"trusted peer header is honored", "10.0.0.0/8", request("10.1.2.3:80", Header, "198.51.100.7"), "198.51.100.7"},
		{"trusted bare IP", "172.18.0.5", request("172.18.0.5:80", Header, "198.51.100.7"), "198.51.100.7"},
		{"neighbor of a trusted bare IP is not trusted", "172.18.0.5", request("172.18.0.6:80", Header, "198.51.100.7"), "172.18.0.6"},
		{"trusted peer without header", "10.0.0.0/8", request("10.1.2.3:80"), "10.1.2.3"},
		{"malformed header falls back to the peer", "10.0.0.0/8", request("10.1.2.3:80", Header, "not-an-ip"), "10.1.2.3"},
		{"comma list is ambiguous", "10.0.0.0/8", request("10.1.2.3:80", Header, "198.51.100.7, 10.0.0.1"), "10.1.2.3"},
		{"repeated header lines are ambiguous", "10.0.0.0/8", request("10.1.2.3:80", Header, "198.51.100.7", Header, "192.0.2.1"), "10.1.2.3"},
		{"empty header", "10.0.0.0/8", request("10.1.2.3:80", Header, ""), "10.1.2.3"},
		{"port in header is malformed", "10.0.0.0/8", request("10.1.2.3:80", Header, "198.51.100.7:443"), "10.1.2.3"},
		{"header whitespace is trimmed", "10.0.0.0/8", request("10.1.2.3:80", Header, "  198.51.100.7 "), "198.51.100.7"},
		{"IPv6 trusted peer, IPv6 client", "fd00::/8", request("[fd00::5]:80", Header, "2001:db8::1"), "2001:db8::1"},
		{"IPv6 client is canonicalized", "fd00::/8", request("[fd00::5]:80", Header, "2001:0DB8:0:0:0:0:0:1"), "2001:db8::1"},
		{"IPv6 untrusted peer spoofing", "fd00::/8", request("[2001:db8::99]:80", Header, "2001:db8::1"), "2001:db8::99"},
		{"IPv6 peer is reported canonically", "", request("[2001:DB8::1]:80"), "2001:db8::1"},
		{"IPv6 zone is dropped", "fd00::/8", request("[fd00::5%eth0]:80", Header, "fe80::1%eth0"), "fe80::1"},
		{"IPv4-mapped peer matches an IPv4 prefix", "10.0.0.0/8", request("[::ffff:10.1.2.3]:80", Header, "198.51.100.7"), "198.51.100.7"},
		{"IPv4-mapped client header is unmapped", "10.0.0.0/8", request("10.1.2.3:80", Header, "::ffff:198.51.100.7"), "198.51.100.7"},
		{"IPv4-mapped prefix in the list", "::ffff:10.0.0.0/104", request("10.1.2.3:80", Header, "198.51.100.7"), "198.51.100.7"},
		{"unparsable RemoteAddr is returned as is", "10.0.0.0/8", request("garbage", Header, "198.51.100.7"), "garbage"},
		{"RemoteAddr without port", "", request("203.0.113.9"), "203.0.113.9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mustResolver(t, tt.trusted).IP(tt.req); got != tt.want {
				t.Fatalf("IP = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseTrusted(t *testing.T) {
	valid := []string{"", "  ", "10.0.0.0/8", "10.0.0.0/8, 172.16.0.0/12,fd00::/8", "127.0.0.1", "::1", "10.1.2.3/8", ",10.0.0.0/8,"}
	for _, raw := range valid {
		if _, err := ParseTrusted(raw); err != nil {
			t.Errorf("ParseTrusted(%q) unexpected error: %v", raw, err)
		}
	}
	invalid := []string{"nonsense", "10.0.0.0/33", "10.0.0.0/8, x", "0.0.0.0/0", "::/0", "10.0.0.0/"}
	for _, raw := range invalid {
		if _, err := ParseTrusted(raw); err == nil {
			t.Errorf("ParseTrusted(%q) should fail", raw)
		}
	}
	if got, _ := ParseTrusted(""); len(got) != 0 {
		t.Errorf("empty list should trust nothing, got %v", got)
	}
}

func TestMiddlewareAndFromRequest(t *testing.T) {
	r := mustResolver(t, "10.0.0.0/8")
	var seen string
	h := r.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		seen = FromRequest(req)
	}))
	h.ServeHTTP(httptest.NewRecorder(), request("10.1.2.3:80", Header, "198.51.100.7"))
	if seen != "198.51.100.7" {
		t.Fatalf("middleware ip = %q, want the trusted header value", seen)
	}

	// Without the middleware nothing is trusted, even a header from a would-be proxy.
	if got := FromRequest(request("10.1.2.3:80", Header, "198.51.100.7")); got != "10.1.2.3" {
		t.Fatalf("FromRequest without middleware = %q, want the peer", got)
	}
}
