package zotero

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

const DefaultBaseURL = "http://127.0.0.1:23119/api"

func NormalizeBaseURL(raw string) (string, error) {
	parsed, err := ParseAndValidateBaseURL(raw)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func ParseAndValidateBaseURL(raw string) (*url.URL, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		value = DefaultBaseURL
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("Zotero Local API 地址无效")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("Zotero Local API 只允许 http 或 https")
	}
	host := parsed.Hostname()
	if !isLoopbackHost(host) {
		return nil, fmt.Errorf("Zotero Local API 只允许连接本机 127.0.0.1 或 localhost")
	}
	if parsed.Port() != "" {
		port, err := strconv.Atoi(parsed.Port())
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("Zotero Local API 端口无效")
		}
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if parsed.Path == "" {
		parsed.Path = "/api"
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed, nil
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
