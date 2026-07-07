package deploy

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	cpuMilliRe = regexp.MustCompile(`^([1-9]\d{0,4})m$`)
	cpuCoreRe  = regexp.MustCompile(`^([1-9]\d{0,1})$`)
	memMiRe    = regexp.MustCompile(`^([1-9]\d{0,4})Mi$`)
	memGiRe    = regexp.MustCompile(`^([1-9]\d{0,1})Gi$`)
)

const (
	maxCPUMilli = 32000
	maxCPUCores = 32
	maxMemMi    = 32768
	maxMemGi    = 64
)

// ValidateServiceResources — custom mode: format K8s + request ≤ limit.
func ValidateServiceResources(mode, cpuReq, memReq, cpuLim, memLim string) error {
	mode = NormalizeResourcesMode(mode)
	if mode != ResourcesCustom {
		return nil
	}
	cpuReq = strings.TrimSpace(cpuReq)
	memReq = strings.TrimSpace(memReq)
	cpuLim = strings.TrimSpace(cpuLim)
	memLim = strings.TrimSpace(memLim)
	if cpuReq == "" && memReq == "" && cpuLim == "" && memLim == "" {
		return fmt.Errorf("tùy chỉnh: nhập ít nhất một giá trị CPU hoặc RAM")
	}
	if cpuReq != "" {
		if err := validateCPUQuantity(cpuReq); err != nil {
			return fmt.Errorf("CPU request: %w", err)
		}
	}
	if cpuLim != "" {
		if err := validateCPUQuantity(cpuLim); err != nil {
			return fmt.Errorf("CPU limit: %w", err)
		}
	}
	if memReq != "" {
		if err := validateMemoryQuantity(memReq); err != nil {
			return fmt.Errorf("RAM request: %w", err)
		}
	}
	if memLim != "" {
		if err := validateMemoryQuantity(memLim); err != nil {
			return fmt.Errorf("RAM limit: %w", err)
		}
	}
	if cpuReq != "" && cpuLim != "" {
		reqMilli, err := cpuToMilli(cpuReq)
		if err != nil {
			return err
		}
		limMilli, err := cpuToMilli(cpuLim)
		if err != nil {
			return err
		}
		if reqMilli > limMilli {
			return fmt.Errorf("CPU request (%s) không được lớn hơn CPU limit (%s)", cpuReq, cpuLim)
		}
	}
	if memReq != "" && memLim != "" {
		reqMi, err := memoryToMi(memReq)
		if err != nil {
			return err
		}
		limMi, err := memoryToMi(memLim)
		if err != nil {
			return err
		}
		if reqMi > limMi {
			return fmt.Errorf("RAM request (%s) không được lớn hơn RAM limit (%s)", memReq, memLim)
		}
	}
	return nil
}

func validateCPUQuantity(s string) error {
	if _, err := cpuToMilli(s); err != nil {
		return fmt.Errorf("dùng số + đơn vị m (vd. 100m) hoặc lõi (vd. 1)")
	}
	return nil
}

func validateMemoryQuantity(s string) error {
	if _, err := memoryToMi(s); err != nil {
		return fmt.Errorf("dùng Mi hoặc Gi (vd. 128Mi, 1Gi)")
	}
	return nil
}

func cpuToMilli(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("rỗng")
	}
	if m := cpuMilliRe.FindStringSubmatch(s); len(m) == 2 {
		n, _ := strconv.ParseInt(m[1], 10, 64)
		if n < 1 || n > maxCPUMilli {
			return 0, fmt.Errorf("CPU m: 1–%d", maxCPUMilli)
		}
		return n, nil
	}
	if m := cpuCoreRe.FindStringSubmatch(s); len(m) == 2 {
		n, _ := strconv.ParseInt(m[1], 10, 64)
		if n < 1 || n > maxCPUCores {
			return 0, fmt.Errorf("CPU lõi: 1–%d", maxCPUCores)
		}
		return n * 1000, nil
	}
	return 0, fmt.Errorf("định dạng không hợp lệ")
}

func memoryToMi(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("rỗng")
	}
	if m := memMiRe.FindStringSubmatch(s); len(m) == 2 {
		n, _ := strconv.ParseInt(m[1], 10, 64)
		if n < 32 || n > maxMemMi {
			return 0, fmt.Errorf("RAM Mi: 32–%d", maxMemMi)
		}
		return n, nil
	}
	if m := memGiRe.FindStringSubmatch(s); len(m) == 2 {
		n, _ := strconv.ParseInt(m[1], 10, 64)
		if n < 1 || n > maxMemGi {
			return 0, fmt.Errorf("RAM Gi: 1–%d", maxMemGi)
		}
		return n * 1024, nil
	}
	return 0, fmt.Errorf("định dạng không hợp lệ")
}
