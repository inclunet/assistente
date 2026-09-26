package commandbindings

// AppPageValue is a route-level application fact. It deliberately does not reuse
// SurfaceType: workspace tabs and focus remain independent dimensions.
type AppPageValue string

const (
	PageWorkspace AppPageValue = "workspace"
	PageSettings  AppPageValue = "settings"
	PageProfiles  AppPageValue = "profiles"
	PageHistory   AppPageValue = "history"
	PageHelp      AppPageValue = "help"
	PageAbout     AppPageValue = "about"
	PageUpdate    AppPageValue = "update"
	PageTaskLists AppPageValue = "tasklists"
	PageJobs      AppPageValue = "jobs"
	PageMemories  AppPageValue = "memories"
)

var appPages = [...]AppPageValue{
	PageWorkspace, PageSettings, PageProfiles, PageHistory, PageHelp,
	PageAbout, PageUpdate, PageTaskLists, PageJobs, PageMemories,
}

func AppPages() []string {
	pages := make([]string, len(appPages))
	for i, page := range appPages {
		pages[i] = string(page)
	}
	return pages
}

func IsAppPage(value string) bool {
	for _, page := range appPages {
		if value == string(page) {
			return true
		}
	}
	return false
}
