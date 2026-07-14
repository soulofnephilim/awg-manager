package nwg

import (
	"net"
	"strings"
)

// endpointHostIsIPv6 reports whether the "host:port" / "[host]:port" endpoint
// carries an IPv6-literal host. Hostnames and IPv4 return false.
func endpointHostIsIPv6(endpoint string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(endpoint))
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.To4() == nil
}

// ndmsEndpointPlaceholder — endpoint-заглушка для NDMS на этапе create.
// Порт сохраняем из реального endpoint'а (информативно в NDMS UI), хост —
// loopback: NDMS RCI-импорт и batch-команды отвергают IPv6-endpoint
// («invalid endpoint format»), а endpoint на этапе create — временный:
// Start в любом случае переставляет его (127.0.0.1:proxy у kmod-пути,
// реальный у нативного ASC).
func ndmsEndpointPlaceholder(endpoint string) string {
	_, port, err := net.SplitHostPort(strings.TrimSpace(endpoint))
	if err != nil || port == "" {
		port = "51820"
	}
	return "127.0.0.1:" + port
}

// replaceConfEndpointLine переписывает строку `Endpoint = ...` в .conf,
// сгенерированном config.GenerateForExport (там ровно одна такая строка).
func replaceConfEndpointLine(conf, endpoint string) string {
	lines := strings.Split(conf, "\n")
	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "Endpoint") && strings.Contains(trimmed, "=") {
			lines[i] = "Endpoint = " + endpoint
		}
	}
	return strings.Join(lines, "\n")
}
