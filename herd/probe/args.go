package probe

// Args are the akita arguments that probe a host's workspaces: all of
// them under root, or only the named projects.
func Args(root string, projects ...string) []string {
	args := make([]string, 0, 5+2*len(projects))
	args = append(args, "herd", "probe", "--json", "--root", root)

	for _, p := range projects {
		args = append(args, "--project", p)
	}

	return args
}

// RunArgs are the akita arguments that print a RunDetail.
func RunArgs(root, project, name string) []string {
	return []string{"herd", "probe", "run", "--root", root, project, name}
}

// CampaignArgs are the akita arguments that print a CampaignCells.
func CampaignArgs(root, project, name string) []string {
	return []string{"herd", "probe", "campaign", "--root", root, project, name}
}
