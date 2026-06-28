package deploy

import (
	"github.com/Thien2026/k8s/services/portal-api/internal/platformcontract"
)

// ServiceDefsFromContract chuyển .platform/services.yaml → ServiceDef.
func ServiceDefsFromContract(f platformcontract.ServicesFile) []ServiceDef {
	if f.Layout != LayoutMulti {
		return nil
	}
	out := make([]ServiceDef, 0, len(f.Services))
	for i, s := range f.Services {
		out = append(out, NormalizeServiceDef(ServiceDef{
			Name:           s.Name,
			DisplayName:    s.DisplayName,
			BuildContext:   s.BuildContext,
			BuildMode:      s.BuildMode,
			DockerfilePath: s.DockerfilePath,
			ContainerPort:  s.ContainerPort,
			HealthPath:     s.HealthPath,
			IngressPath:    s.IngressPath,
			ExposeIngress:  platformcontract.ServiceSpecExpose(s),
			SortOrder:      i,
		}))
	}
	return out
}

// ServicesContractSameAsDB so sánh contract với cấu hình Console.
func ServicesContractSameAsDB(f platformcontract.ServicesFile, db []ServiceDef) bool {
	if f.Layout != LayoutMulti {
		return len(db) == 0
	}
	want := ServiceDefsFromContract(f)
	if len(want) != len(db) {
		return false
	}
	for i := range want {
		a, b := want[i], db[i]
		if a.Name != b.Name ||
			a.BuildContext != b.BuildContext ||
			a.DockerfilePath != b.DockerfilePath ||
			a.IngressPath != b.IngressPath ||
			a.ExposeIngress != b.ExposeIngress {
			return false
		}
	}
	return true
}
