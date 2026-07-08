package rancher

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

func parseK8sItems(body []byte, defaultKind string) ([]ResourceRow, int, error) {
	var envelope struct {
		Items []json.RawMessage `json:"items"`
		Data  []json.RawMessage `json:"data"`
		Count int               `json:"count"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, 0, err
	}

	rawItems := envelope.Items
	if len(rawItems) == 0 {
		rawItems = envelope.Data
	}

	rows := make([]ResourceRow, 0, len(rawItems))
	for _, raw := range rawItems {
		if row, ok := parseK8sItem(raw, defaultKind); ok {
			rows = append(rows, row)
		}
	}

	total := envelope.Count
	if total == 0 {
		total = len(rows)
	}
	return rows, total, nil
}

func parseK8sItem(raw json.RawMessage, defaultKind string) (ResourceRow, bool) {
	var obj struct {
		ID                  string `json:"id"`
		Kind                string `json:"kind"`
		Reason              string `json:"reason"`
		Note                string `json:"note"`
		Message             string `json:"message"`
		EventTime           string `json:"eventTime"`
		ReportingController string `json:"reportingController"`
		Metadata            struct {
			Name              string `json:"name"`
			Namespace         string `json:"namespace"`
			CreationTimestamp string `json:"creationTimestamp"`
		} `json:"metadata"`
		Regarding struct {
			Kind      string `json:"kind"`
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
		} `json:"regarding"`
		InvolvedObject struct {
			Kind      string `json:"kind"`
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
		} `json:"involvedObject"`
		LastTimestamp string          `json:"lastTimestamp"`
		Status        json.RawMessage `json:"status"`
		Spec          json.RawMessage `json:"spec"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ResourceRow{}, false
	}

	name := obj.Metadata.Name
	if name == "" {
		name = obj.ID
	}

	created := obj.EventTime
	if created == "" {
		created = obj.LastTimestamp
	}
	if created == "" {
		created = obj.Metadata.CreationTimestamp
	}

	kind := obj.Kind
	if kind == "" {
		kind = defaultKind
	}
	row := ResourceRow{
		Name:      name,
		Namespace: obj.Metadata.Namespace,
		Created:   created,
		Kind:      kind,
		Status:    extractStatus(kind, obj.Status),
	}

	if isEventObject(kind, obj.Reason, obj.Note, obj.EventTime, obj.Regarding.Name, obj.InvolvedObject.Name) {
		row.Status = eventTypeFromRaw(raw)
		row.Reason = obj.Reason
		row.Message = obj.Note
		if row.Message == "" {
			row.Message = obj.Message
		}
		ref := obj.Regarding
		if ref.Name == "" {
			ref = obj.InvolvedObject
		}
		if ref.Name != "" {
			row.Object = ref.Kind + "/" + ref.Name
			if row.Namespace == "" {
				row.Namespace = ref.Namespace
			}
		}
		return row, true
	}

	if kind == "Node" {
		enrichNodeRow(&row, obj.Status)
	}
	enrichWorkloadRow(&row, kind, obj.Status, obj.Spec)

	return row, name != ""
}

func enrichWorkloadRow(row *ResourceRow, kind string, status, spec json.RawMessage) {
	if kind == "" && len(status) > 0 {
		var probe struct {
			Phase string `json:"phase"`
		}
		if json.Unmarshal(status, &probe) == nil && probe.Phase != "" {
			kind = "Pod"
		}
	}
	switch kind {
	case "Pod":
		var st struct {
			PodIP      string `json:"podIP"`
			HostIP     string `json:"hostIP"`
			Phase      string `json:"phase"`
			Conditions []struct {
				Type   string `json:"type"`
				Status string `json:"status"`
			} `json:"conditions"`
			ContainerStatuses []struct {
				RestartCount int    `json:"restartCount"`
				Ready        bool   `json:"ready"`
				Image        string `json:"image"`
				State        struct {
					Waiting *struct {
						Reason string `json:"reason"`
					} `json:"waiting"`
					Terminated *struct {
						Reason string `json:"reason"`
					} `json:"terminated"`
				} `json:"state"`
				LastState struct {
					Terminated *struct {
						Reason string `json:"reason"`
					} `json:"terminated"`
				} `json:"lastState"`
			} `json:"containerStatuses"`
		}
		var sp struct {
			RestartPolicy string `json:"restartPolicy"`
			NodeName      string `json:"nodeName"`
			Containers    []struct {
				Image string `json:"image"`
			} `json:"containers"`
		}
		if json.Unmarshal(status, &st) == nil {
			row.PodIP = st.PodIP
			row.HostIP = st.HostIP
			if st.Phase != "" {
				row.Status = st.Phase
			}
			var imgs []string
			for _, c := range st.ContainerStatuses {
				row.Restarts += c.RestartCount
				if c.Image != "" {
					imgs = append(imgs, c.Image)
				}
				if c.State.Waiting != nil && c.State.Waiting.Reason != "" {
					row.Status = c.State.Waiting.Reason
				} else if c.State.Terminated != nil && c.State.Terminated.Reason == "Error" {
					row.Status = "Error"
				}
				if c.LastState.Terminated != nil && c.LastState.Terminated.Reason != "" {
					row.LastTerminationReason = c.LastState.Terminated.Reason
				} else if c.State.Terminated != nil && c.State.Terminated.Reason != "" {
					row.LastTerminationReason = c.State.Terminated.Reason
				}
			}
			if len(st.ContainerStatuses) > 0 {
				allReady := true
				for _, c := range st.ContainerStatuses {
					if !c.Ready {
						allReady = false
						break
					}
				}
				row.Ready = allReady
			} else {
				row.Ready = podReadyFromConditions(st.Conditions)
			}
			if len(imgs) > 0 {
				row.Images = strings.Join(imgs, ", ")
				if len(imgs) > 1 {
					row.Images = imgs[0] + " (+" + fmt.Sprintf("%d", len(imgs)-1) + ")"
				}
			}
		}
		if json.Unmarshal(spec, &sp) == nil {
			if sp.RestartPolicy != "" {
				row.RestartPolicy = sp.RestartPolicy
			}
			row.Node = sp.NodeName
			if row.Images == "" && len(sp.Containers) > 0 && sp.Containers[0].Image != "" {
				row.Images = sp.Containers[0].Image
			}
		}
	case "Deployment", "StatefulSet", "DaemonSet":
		var st struct {
			Replicas          *int `json:"replicas"`
			ReadyReplicas     int  `json:"readyReplicas"`
			AvailableReplicas int  `json:"availableReplicas"`
		}
		var sp struct {
			Replicas *int `json:"replicas"`
		}
		if json.Unmarshal(status, &st) == nil {
			want := st.Replicas
			if want == nil && json.Unmarshal(spec, &sp) == nil {
				want = sp.Replicas
			}
			if want != nil {
				row.Replicas = fmt.Sprintf("%d/%d ready", st.ReadyReplicas, *want)
			} else if st.ReadyReplicas > 0 {
				row.Replicas = fmt.Sprintf("%d ready", st.ReadyReplicas)
			}
			if st.AvailableReplicas > 0 && want != nil && st.AvailableReplicas != st.ReadyReplicas {
				row.Status = fmt.Sprintf("%d available", st.AvailableReplicas)
			} else if want != nil {
				if st.ReadyReplicas >= *want && *want > 0 {
					row.Status = "Available"
				} else if *want == 0 {
					row.Status = "Scaled to 0"
				} else {
					row.Status = fmt.Sprintf("%d/%d ready", st.ReadyReplicas, *want)
				}
			} else if st.ReadyReplicas > 0 {
				row.Status = fmt.Sprintf("%d ready", st.ReadyReplicas)
			}
		}
		var selWrap struct {
			Selector struct {
				MatchLabels map[string]string `json:"matchLabels"`
			} `json:"selector"`
		}
		if json.Unmarshal(spec, &selWrap) == nil && len(selWrap.Selector.MatchLabels) > 0 {
			parts := make([]string, 0, len(selWrap.Selector.MatchLabels))
			for k, v := range selWrap.Selector.MatchLabels {
				parts = append(parts, k+"="+v)
			}
			row.Selector = strings.Join(parts, ", ")
		}
	case "Job":
		var st struct {
			Succeeded int `json:"succeeded"`
			Failed    int `json:"failed"`
			Active    int `json:"active"`
		}
		var sp struct {
			Completions *int `json:"completions"`
		}
		json.Unmarshal(spec, &sp)
		if json.Unmarshal(status, &st) == nil {
			row.Status = fmt.Sprintf("active=%d succeeded=%d", st.Active, st.Succeeded)
			if sp.Completions != nil {
				row.Completions = fmt.Sprintf("%d/%d", st.Succeeded, *sp.Completions)
			}
		}
		if sp.Completions != nil && row.Completions == "" {
			row.Completions = fmt.Sprintf("0/%d", *sp.Completions)
		}
	case "HorizontalPodAutoscaler":
		var sp struct {
			MinReplicas *int `json:"minReplicas"`
			MaxReplicas int  `json:"maxReplicas"`
		}
		var st struct {
			CurrentReplicas int `json:"currentReplicas"`
		}
		if json.Unmarshal(spec, &sp) == nil {
			min := 1
			if sp.MinReplicas != nil {
				min = *sp.MinReplicas
			}
			cur := 0
			if json.Unmarshal(status, &st) == nil {
				cur = st.CurrentReplicas
			}
			row.Scale = fmt.Sprintf("%d–%d → %d", min, sp.MaxReplicas, cur)
		}
	case "CronJob":
		var sp struct {
			Schedule string `json:"schedule"`
			Suspend  *bool  `json:"suspend"`
		}
		var st struct {
			Active []any `json:"active"`
		}
		if json.Unmarshal(spec, &sp) == nil {
			row.Schedule = sp.Schedule
			if sp.Suspend != nil && *sp.Suspend {
				row.Suspend = "yes"
			} else {
				row.Suspend = "no"
			}
		}
		if json.Unmarshal(status, &st) == nil {
			if len(st.Active) > 0 {
				row.Status = fmt.Sprintf("active=%d", len(st.Active))
			}
		}
	case "Service":
		var sp struct {
			Type      string `json:"type"`
			ClusterIP string `json:"clusterIP"`
			Ports     []struct {
				Port       int    `json:"port"`
				TargetPort any    `json:"targetPort"`
				Protocol   string `json:"protocol"`
			} `json:"ports"`
		}
		if json.Unmarshal(spec, &sp) == nil {
			row.ServiceType = sp.Type
			row.ClusterIP = sp.ClusterIP
			var ports []string
			for _, p := range sp.Ports {
				ports = append(ports, fmt.Sprintf("%d/%s", p.Port, p.Protocol))
			}
			row.Ports = strings.Join(ports, ", ")
		}
	case "Ingress":
		var sp struct {
			IngressClassName *string `json:"ingressClassName"`
			Rules            []struct {
				Host string `json:"host"`
			} `json:"rules"`
		}
		if json.Unmarshal(spec, &sp) == nil {
			var hosts []string
			for _, r := range sp.Rules {
				if r.Host != "" {
					hosts = append(hosts, r.Host)
				}
			}
			row.Host = strings.Join(hosts, ", ")
			if sp.IngressClassName != nil {
				row.Status = *sp.IngressClassName
			}
		}
	case "PersistentVolumeClaim":
		var sp struct {
			StorageClassName *string  `json:"storageClassName"`
			AccessModes      []string `json:"accessModes"`
			Resources        struct {
				Requests map[string]string `json:"requests"`
			} `json:"resources"`
		}
		if json.Unmarshal(spec, &sp) == nil {
			if sp.StorageClassName != nil {
				row.StorageClass = *sp.StorageClassName
			}
			row.AccessModes = strings.Join(sp.AccessModes, ", ")
			if s, ok := sp.Resources.Requests["storage"]; ok {
				row.Capacity = s
			}
		}
	case "PersistentVolume":
		var sp struct {
			StorageClassName string            `json:"storageClassName"`
			AccessModes      []string          `json:"accessModes"`
			Capacity         map[string]string `json:"capacity"`
		}
		if json.Unmarshal(spec, &sp) == nil {
			row.StorageClass = sp.StorageClassName
			row.AccessModes = strings.Join(sp.AccessModes, ", ")
			if s, ok := sp.Capacity["storage"]; ok {
				row.Capacity = s
			}
		}
	case "Secret", "ConfigMap":
		// keys count handled via status phase only
	}
}

func isEventObject(kind, reason, note, eventTime, regarding, involved string) bool {
	if strings.EqualFold(kind, "Event") {
		return true
	}
	if eventTime != "" && reason != "" {
		return true
	}
	if reason != "" && (regarding != "" || involved != "") {
		return true
	}
	if note != "" && reason != "" {
		return true
	}
	return false
}

func eventTypeFromRaw(raw json.RawMessage) string {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return "Normal"
	}
	if t, ok := m["type"].(string); ok && (t == "Normal" || t == "Warning") {
		return t
	}
	return "Normal"
}

func enrichNodeRow(row *ResourceRow, status json.RawMessage) {
	var st struct {
		Addresses []struct {
			Type    string `json:"type"`
			Address string `json:"address"`
		} `json:"addresses"`
		Allocatable map[string]string `json:"allocatable"`
		Capacity    map[string]string `json:"capacity"`
	}
	if json.Unmarshal(status, &st) != nil {
		return
	}
	for _, a := range st.Addresses {
		if a.Type == "InternalIP" || a.Type == "ExternalIP" {
			row.NodeIP = a.Address
			break
		}
	}
	if p, ok := st.Capacity["pods"]; ok {
		fmt.Sscanf(p, "%d", &row.PodsMax)
	}
	row.CPUCores = parseCPU(st.Capacity["cpu"])
	row.MemGiB = parseMemGiB(st.Capacity["memory"])
}

func parseCPU(s string) float64 {
	if s == "" {
		return 0
	}
	var v float64
	if strings.HasSuffix(s, "n") {
		fmt.Sscanf(s, "%f", &v)
		return v / 1e9
	}
	if strings.HasSuffix(s, "m") {
		fmt.Sscanf(s, "%f", &v)
		return v / 1000
	}
	fmt.Sscanf(s, "%f", &v)
	return v
}

func parseMemGiB(s string) float64 {
	if s == "" {
		return 0
	}
	var v float64
	if strings.HasSuffix(s, "Ki") {
		fmt.Sscanf(s, "%fKi", &v)
		return v / (1024 * 1024)
	}
	if strings.HasSuffix(s, "Mi") {
		fmt.Sscanf(s, "%fMi", &v)
		return v / 1024
	}
	if strings.HasSuffix(s, "Gi") {
		fmt.Sscanf(s, "%fGi", &v)
		return v
	}
	fmt.Sscanf(s, "%f", &v)
	return v / (1024 * 1024 * 1024)
}

func podReadyFromConditions(conditions []struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}) bool {
	for _, c := range conditions {
		if c.Type == "Ready" && strings.EqualFold(strings.TrimSpace(c.Status), "True") {
			return true
		}
	}
	return false
}

func extractStatus(kind string, status json.RawMessage) string {
	if len(status) == 0 {
		return ""
	}
	var generic map[string]any
	if err := json.Unmarshal(status, &generic); err != nil {
		return ""
	}
	switch kind {
	case "Pod":
		if phase, ok := generic["phase"].(string); ok {
			return phase
		}
	case "Node":
		if conds, ok := generic["conditions"].([]any); ok {
			for _, c := range conds {
				m, _ := c.(map[string]any)
				if m["type"] == "Ready" {
					if s, ok := m["status"].(string); ok {
						return "Ready=" + s
					}
				}
			}
		}
	case "Deployment", "StatefulSet", "DaemonSet":
		if r, ok := generic["readyReplicas"].(float64); ok {
			d := generic["replicas"]
			return fmt.Sprintf("%v/%v ready", int(r), d)
		}
	case "Job":
		if s, ok := generic["succeeded"].(float64); ok {
			return fmt.Sprintf("succeeded=%d", int(s))
		}
	case "CronJob":
		if a, ok := generic["active"].([]any); ok {
			return fmt.Sprintf("active=%d", len(a))
		}
	case "PersistentVolume", "PersistentVolumeClaim", "Namespace":
		if phase, ok := generic["phase"].(string); ok {
			return phase
		}
	}
	if phase, ok := generic["phase"].(string); ok {
		return phase
	}
	if load, ok := generic["loadBalancer"].(map[string]any); ok && load != nil {
		return "LB"
	}
	return ""
}

func round1(f float64) float64 {
	return math.Round(f*10) / 10
}
