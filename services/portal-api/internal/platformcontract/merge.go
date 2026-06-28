package platformcontract

// MergeContracts — gộp contract repo (ưu tiên) với defaults platform.
func MergeContracts(repo *File, defaults File) *File {
	out := File{
		Version: ContractVersion,
		Vars:    map[string]VarSpec{},
	}
	for k, v := range defaults.Vars {
		out.Vars[k] = v
	}
	if repo != nil {
		if repo.Version > 0 {
			out.Version = repo.Version
		}
		for k, v := range repo.Vars {
			out.Vars[k] = v
		}
	}
	if len(out.Vars) == 0 {
		return nil
	}
	return &out
}
