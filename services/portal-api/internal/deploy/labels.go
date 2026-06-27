package deploy

import "strings"

// Label dùng trên Pod template — list pod qua Rancher labelSelector.
const LabelImageTag = "platform.7mlabs.com/image-tag"

// ImageTagLabelValue rút gọn tag cho label K8s (max 63 ký tự).
func ImageTagLabelValue(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "latest"
	}
	if len(tag) > 63 {
		return tag[:63]
	}
	return tag
}
