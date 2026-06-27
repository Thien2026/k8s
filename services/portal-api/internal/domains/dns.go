package domains

import "strings"

// DNSHint hướng dẫn cấu hình DNS cho custom domain.
type DNSHint struct {
	Mode        string `json:"mode"` // auto | cname | a
	Message     string `json:"message,omitempty"`
	RecordType  string `json:"record_type,omitempty"`
	RecordName  string `json:"record_name,omitempty"`
	RecordValue string `json:"record_value,omitempty"`
	AltType     string `json:"alt_type,omitempty"`
	AltValue    string `json:"alt_value,omitempty"`
	Note        string `json:"note,omitempty"`
}

func DNSInstructions(kind, hostname string, p Platform) DNSHint {
	if kind == "auto" {
		return DNSHint{
			Mode:    "auto",
			Message: "Hostname *.sslip.io (hoặc subdomain platform) tự trỏ về IP cluster — không cần cấu hình DNS.",
		}
	}
	host := strings.TrimSpace(strings.ToLower(hostname))
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return DNSHint{Mode: "custom", Message: "Hostname không hợp lệ"}
	}

	cnameTarget := p.IngressCNAMETarget()
	if cnameTarget == "" && p.PublicIP != "" {
		cnameTarget = p.PublicIP
	}

	// Apex: example.com → A record
	if len(parts) == 2 {
		return DNSHint{
			Mode:        "a",
			RecordType:  "A",
			RecordName:  "@",
			RecordValue: p.PublicIP,
			Note:        "Apex domain: dùng A record. Cloudflare: tạm DNS only (xám) khi xin Let's Encrypt.",
		}
	}

	recordName := parts[0]
	if recordName == "www" && len(parts) == 3 {
		recordName = "www"
	}

	hint := DNSHint{
		Mode:       "cname",
		RecordType: "CNAME",
		RecordName: recordName,
		Note:       "Sau khi DNS propagate (5–30 phút), cert TLS sẽ tự cấp qua Let's Encrypt.",
	}
	if cnameTarget != "" && !strings.Contains(cnameTarget, ".") {
		hint.RecordValue = cnameTarget
		hint.AltType = "A"
		hint.AltValue = p.PublicIP
	} else if cnameTarget != "" {
		hint.RecordValue = cnameTarget
		if p.PublicIP != "" {
			hint.AltType = "A"
			hint.AltValue = p.PublicIP
		}
	} else {
		hint.Mode = "a"
		hint.RecordType = "A"
		hint.RecordName = recordName
		hint.RecordValue = p.PublicIP
	}
	return hint
}
