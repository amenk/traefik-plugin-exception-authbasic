package traefik_plugin_exception_basicauth

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
)

// Config is the configuration for this plugin
type Config struct {
	AllowIPList []string `json:"allowIPList,omitempty"`
	User        string   `json:"user"`
	Password    string   `json:"password"`
	PreventUser bool     `json:"preventUser"`
	IPHeaders   []string `json:"ipHeaders"`
}

// CreateConfig creates a new configuration for this plugin
func CreateConfig() *Config {
	return &Config{}
}

// ExceptBasicAuth represents the basic properties of this plugin
type ExceptBasicAuth struct {
	next          http.Handler
	name          string
	config        *Config
	allowedIPs    []*net.IP
	allowedIPNets []*net.IPNet
}

// New creates a new instance of this plugin
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	var allowedIPs []*net.IP
	var allowedIPNets []*net.IPNet

	for _, allowedIP := range config.AllowIPList {
		ip, ipNet, err := parseIP(allowedIP)

		if err != nil {
			log.Printf("Failed to parse ip %s: %v", allowedIP, err)
		} else if ip != nil {
			allowedIPs = append(allowedIPs, ip)
		} else if ipNet != nil {
			allowedIPNets = append(allowedIPNets, ipNet)
		}
	}

	return &ExceptBasicAuth{
		name:          name,
		next:          next,
		config:        config,
		allowedIPNets: allowedIPNets,
		allowedIPs:    allowedIPs,
	}, nil
}

func (e *ExceptBasicAuth) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	extractedIPs := e.extractIP(req)

	if len(extractedIPs) > 0 && e.isAnyIPAllowed(extractedIPs) {
		req.SetBasicAuth(e.config.User, e.config.Password)
	} else if e.config.PreventUser && req.Header.Get("Authorization") != "" {
		user, _, ok := req.BasicAuth()
		if ok && user == e.config.User {
			rw.WriteHeader(http.StatusUnauthorized)
			return
		}
	}

	e.next.ServeHTTP(rw, req)
}

func (e *ExceptBasicAuth) extractIP(req *http.Request) []string {
	var possibleIPs []string

	for _, header := range e.config.IPHeaders {
		headerVal := req.Header.Get(header)

		if headerVal != "" {
			for _, possibleIP := range strings.Split(headerVal, ",") {
				parsedIP := net.ParseIP(strings.TrimSpace(possibleIP))
				if parsedIP != nil {
					possibleIPs = append(possibleIPs, strings.TrimSpace(possibleIP))
				}
			}
		}
	}

	if len(possibleIPs) < 1 {
		ip, _, err := net.SplitHostPort(req.RemoteAddr)
		if err == nil {
			possibleIPs = append(possibleIPs, ip)
		}
	}

	return possibleIPs
}

func (e *ExceptBasicAuth) isAnyIPAllowed(ips []string) bool {
	for _, ip := range ips {
		parsedIP := net.ParseIP(ip)
		if parsedIP != nil && e.isIPAllowed(parsedIP) {
			return true
		}
	}

	return false
}

func (e *ExceptBasicAuth) isIPAllowed(ip net.IP) bool {
	for _, allowedIP := range e.allowedIPs {
		if allowedIP.Equal(ip) {
			return true
		}
	}

	for _, allowedIPnet := range e.allowedIPNets {
		if allowedIPnet.Contains(ip) {
			return true
		}
	}

	return false
}

func parseIP(allowedIP string) (*net.IP, *net.IPNet, error) {
	if strings.Contains(allowedIP, "/") {
		_, ipNet, err := net.ParseCIDR(allowedIP)
		if err != nil {
			return nil, nil, fmt.Errorf("unable to parse %s as cidr, skipping", allowedIP)
		}
		return nil, ipNet, err
	}
	parsedIP := net.ParseIP(allowedIP)
	if parsedIP == nil {
		return nil, nil, fmt.Errorf("unable to parse ip %s, skipping", allowedIP)
	}
	return &parsedIP, nil, nil
}
