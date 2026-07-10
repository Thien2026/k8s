package handler

import "testing"

func TestParseRedisInfoMap(t *testing.T) {
	raw := "# Server\r\nredis_version:7.2.4\r\nuptime_in_seconds:120\r\ndb0:keys=5,expires=1,avg_ttl=1000\r\n"
	m := parseRedisInfoMap(raw)
	if m["redis_version"] != "7.2.4" {
		t.Fatalf("version: %q", m["redis_version"])
	}
	if redisKeyspaceTotalKeys(m) != 5 {
		t.Fatalf("keys total")
	}
}

func TestNormalizeRedisScanPattern(t *testing.T) {
	p, err := normalizeRedisScanPattern("")
	if err != nil || p != "*" {
		t.Fatal(p, err)
	}
	_, err = normalizeRedisScanPattern(string(make([]byte, 121)))
	if err == nil {
		t.Fatal("expected error for long pattern")
	}
}
