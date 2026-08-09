package main

import "strings"

func shouldFailRequest(protocol, payload string) bool {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	payload = strings.ToLower(strings.TrimSpace(payload))

	if payload == "" {
		return false
	}

	return strings.Contains(payload, "fail:all") || strings.Contains(payload, "fail:"+protocol)
}
